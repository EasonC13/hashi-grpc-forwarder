package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"golang.org/x/net/http2"
)

func init() { caddy.RegisterModule(strictH2Transport{}) }

// strictH2Transport never transmits an HTTP request before TLS negotiates h2.
type strictH2Transport struct {
	Versions    []string                `json:"versions,omitempty"`
	DialTimeout caddy.Duration          `json:"dial_timeout,omitempty"`
	TLS         *reverseproxy.TLSConfig `json:"tls,omitempty"`

	transport *http2.Transport
}

func (strictH2Transport) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.reverse_proxy.transport.hashi_h2",
		New: func() caddy.Module { return new(strictH2Transport) },
	}
}

func (h *strictH2Transport) Provision(ctx caddy.Context) error {
	if h.TLS == nil || h.TLS.InsecureSkipVerify || len(h.TLS.ExceptPorts) != 0 {
		return fmt.Errorf("hashi_h2 requires verified TLS on every upstream connection")
	}
	if len(h.Versions) != 0 && (len(h.Versions) != 1 || h.Versions[0] != "2") {
		return fmt.Errorf("hashi_h2 supports only HTTP/2")
	}
	dialTimeout := time.Duration(h.DialTimeout)
	if dialTimeout == 0 {
		dialTimeout = 10 * time.Second
	}
	handshakeTimeout := time.Duration(h.TLS.HandshakeTimeout)
	if handshakeTimeout == 0 {
		handshakeTimeout = 10 * time.Second
	}
	if dialTimeout < 0 || handshakeTimeout < 0 {
		return fmt.Errorf("hashi_h2 timeouts must be positive")
	}
	tlsConfig, err := h.TLS.MakeTLSClientConfig(ctx)
	if err != nil {
		return err
	}
	h.transport = &http2.Transport{
		TLSClientConfig: tlsConfig,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			cfg = cfg.Clone()
			cfg.NextProtos = []string{http2.NextProtoTLS}
			tlsConn := tls.Client(conn, cfg)
			handshakeCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
			defer cancel()
			if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
				conn.Close()
				return nil, err
			}
			if tlsConn.ConnectionState().NegotiatedProtocol != http2.NextProtoTLS {
				tlsConn.Close()
				return nil, fmt.Errorf("hashi_h2 upstream did not negotiate h2")
			}
			return tlsConn, nil
		},
	}
	return nil
}

func (h *strictH2Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("hashi_h2 requires an HTTPS upstream")
	}
	// Caddy clears the scheme before handing the request to its transport.
	req = req.Clone(req.Context())
	req.URL.Scheme = "https"
	return h.transport.RoundTrip(req)
}

func (h *strictH2Transport) Cleanup() error {
	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
	return nil
}

var (
	_ caddy.Provisioner  = (*strictH2Transport)(nil)
	_ caddy.CleanerUpper = (*strictH2Transport)(nil)
	_ http.RoundTripper  = (*strictH2Transport)(nil)
)
