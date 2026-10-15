package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
)

func provisionFixtureTransport(t *testing.T, s *httptest.Server, serverName string) *strictH2Transport {
	t.Helper()
	cert := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	ca, err := json.Marshal(map[string]any{"provider": "file", "pem_files": []string{cert}})
	if err != nil {
		t.Fatal(err)
	}
	tr := &strictH2Transport{TLS: &reverseproxy.TLSConfig{CARaw: ca, ServerName: serverName}}
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	t.Cleanup(cancel)
	if err := tr.Provision(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tr.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	return tr
}

func TestStrictH2RejectsMissingALPN(t *testing.T) {
	var calls atomic.Int32
	negotiated := make(chan string, 1)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	s.TLS = &tls.Config{VerifyConnection: func(state tls.ConnectionState) error {
		negotiated <- state.NegotiatedProtocol
		return nil
	}}
	s.StartTLS()
	defer s.Close()
	// Unlike an HTTP/1-only ALPN offer, no ALPN permits the TLS handshake
	// to succeed. The transport must still reject before sending any request.
	s.TLS.NextProtos = nil
	tr := provisionFixtureTransport(t, s, "")
	req, _ := http.NewRequest("POST", s.URL, nil)
	if resp, err := tr.RoundTrip(req); err == nil {
		resp.Body.Close()
		t.Fatal("accepted TLS without h2 ALPN")
	}
	select {
	case protocol := <-negotiated:
		if protocol != "" {
			t.Fatalf("fixture negotiated %q, expected no ALPN", protocol)
		}
	case <-time.After(time.Second):
		t.Fatal("fixture TLS handshake did not complete")
	}
	if calls.Load() != 0 {
		t.Fatal("sent a request without h2 ALPN")
	}
}

func TestStrictH2VerifiesHostname(t *testing.T) {
	var calls atomic.Int32
	s := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	tr := provisionFixtureTransport(t, s, "wrong.invalid")
	req, _ := http.NewRequest("POST", s.URL, nil)
	if resp, err := tr.RoundTrip(req); err == nil {
		resp.Body.Close()
		t.Fatal("accepted certificate with wrong hostname")
	}
	if calls.Load() != 0 {
		t.Fatal("sent a request to a hostname-mismatched server")
	}
}

func TestStrictH2RejectsCleartext(t *testing.T) {
	tr := &strictH2Transport{}
	req, _ := http.NewRequest("POST", "http://127.0.0.1:1/", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("accepted cleartext upstream")
	}
}

func TestStrictH2RejectsUnsafeConfig(t *testing.T) {
	for name, tr := range map[string]*strictH2Transport{
		"missing TLS":                {},
		"unverified TLS":             {TLS: &reverseproxy.TLSConfig{InsecureSkipVerify: true}},
		"cleartext port":             {TLS: &reverseproxy.TLSConfig{ExceptPorts: []string{"443"}}},
		"HTTP1":                      {TLS: &reverseproxy.TLSConfig{}, Versions: []string{"1.1", "2"}},
		"negative dial timeout":      {TLS: &reverseproxy.TLSConfig{}, DialTimeout: -1},
		"negative handshake timeout": {TLS: &reverseproxy.TLSConfig{HandshakeTimeout: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
			defer cancel()
			if err := tr.Provision(ctx); err == nil {
				t.Fatal("accepted unsafe transport configuration")
			}
		})
	}
}

func TestStrictH2HandshakeTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			io.Copy(io.Discard, conn) // Read ClientHello but never answer it.
		}
	}()
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()
	tr := &strictH2Transport{TLS: &reverseproxy.TLSConfig{HandshakeTimeout: caddy.Duration(50 * time.Millisecond)}}
	if err := tr.Provision(ctx); err != nil {
		t.Fatal(err)
	}
	defer tr.Cleanup()
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()
	req, _ := http.NewRequestWithContext(reqCtx, "POST", "https://"+listener.Addr().String(), nil)
	start := time.Now()
	if resp, err := tr.RoundTrip(req); err == nil {
		resp.Body.Close()
		t.Fatal("accepted stalled TLS handshake")
	}
	if time.Since(start) > time.Second {
		t.Fatal("TLS handshake was not bounded by configured timeout")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed-out TLS connection was not closed")
	}
}
