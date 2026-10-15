package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"github.com/caddyserver/caddy/v2"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func startFixtureProxy(t *testing.T, upstream *httptest.Server, trust bool) *http.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	raw, err := buildCaddyConfig(Config{addr, upstream.URL, "test-fixture-key"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	json.Unmarshal(raw, &cfg)
	server := cfg["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["forwarder"].(map[string]any)
	if trust {
		cert := filepath.Join(t.TempDir(), "fixture-ca.pem")
		os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600)
		transport := server["routes"].([]any)[0].(map[string]any)["handle"].([]any)[0].(map[string]any)["transport"].(map[string]any)
		transport["tls"].(map[string]any)["ca"] = map[string]any{"provider": "file", "pem_files": []string{cert}}
	}
	raw, _ = json.Marshal(cfg)
	if err = caddy.Load(raw, true); err != nil {
		t.Fatal("fixture proxy startup failed:", err)
	}
	t.Cleanup(func() {
		if err := caddy.Stop(); err != nil {
			t.Error(err)
		}
	})
	tr := &http2.Transport{AllowHTTP: true, DialTLSContext: func(ctx context.Context, _ string, _ string, _ *tls.Config) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}
func fixtureServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(handler)
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}
func TestHTTP2HeaderAndTrailerForwarding(t *testing.T) {
	observed := make(chan bool, 1)
	expectedAuthority := ""
	upstream := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) {
		observed <- r.ProtoMajor == 2 && r.Header.Get("X-Api-Key") == "test-fixture-key" && r.Header.Get("Authorization") == "" && r.Host == expectedAuthority
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
		w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "7")
		w.Header().Set("Grpc-Message", "fixture-permission-denied")
	})
	expectedAuthority = upstream.Listener.Addr().String()
	client := startFixtureProxy(t, upstream, true)
	req, _ := http.NewRequest("POST", "http://local.invalid/fixture.Service/Unary", nil)
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("X-Api-Key", "caller-must-not-override")
	req.Header.Set("Authorization", "caller-credential")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || len(data) != 5 || resp.Trailer.Get("Grpc-Status") != "7" || resp.Trailer.Get("Grpc-Message") != "fixture-permission-denied" {
		t.Fatal("gRPC response/trailers not preserved")
	}
	if !<-observed {
		t.Fatal("HTTP/2, authority or credential header forwarding incorrect")
	}
}
func TestHTTP1OnlyTLSRejectedBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	client := startFixtureProxy(t, upstream, true)
	req, err := http.NewRequest("POST", "http://local.invalid/fixture.Service/Unary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusBadGateway || calls.Load() != 0 {
		t.Fatalf("HTTP/1-only TLS: status=%d handler calls=%d; want status=502 handler calls=0", resp.StatusCode, calls.Load())
	}
}

func TestUntrustedTLSRejected(t *testing.T) {
	upstream := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("untrusted upstream handler should not be reached")
	})
	client := startFixtureProxy(t, upstream, false)
	req, _ := http.NewRequest("POST", "http://local.invalid/fixture.Service/Unary", nil)
	req.Header.Set("Content-Type", "application/grpc")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("untrusted TLS returned %d, expected502", resp.StatusCode)
	}
}
func TestStreamingAndCancellation(t *testing.T) {
	permitSecond := make(chan struct{})
	canceled := make(chan struct{})
	ready := make(chan struct{})
	upstream := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Write([]byte{0, 0, 0, 0, 0})
		w.(http.Flusher).Flush()
		close(ready)
		select {
		case <-permitSecond:
		case <-r.Context().Done():
			close(canceled)
			return
		}
		w.Write([]byte{0, 0, 0, 0, 0})
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	})
	client := startFixtureProxy(t, upstream, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://local.invalid/fixture.Service/Stream", nil)
	req.Header.Set("Content-Type", "application/grpc")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data := make([]byte, 5)
	if _, err = io.ReadFull(resp.Body, data); err != nil {
		t.Fatal("first frame did not arrive incrementally:", err)
	}
	<-ready
	close(permitSecond)
	if _, err = io.ReadFull(resp.Body, data); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("downstream cancellation not propagated")
	}
}
