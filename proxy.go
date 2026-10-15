package main

import (
	"encoding/json"
	"net"
	"net/url"
)

func buildCaddyConfig(c Config) ([]byte, error) {
	u, err := url.Parse(c.Upstream)
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	proxy := map[string]any{
		"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": net.JoinHostPort(u.Hostname(), port)}},
		"headers":        map[string]any{"request": map[string]any{"set": map[string][]string{"X-Api-Key": {c.APIKey}, "Host": {u.Host}}, "delete": []string{"Authorization"}}},
		"transport":      map[string]any{"protocol": "hashi_h2", "versions": []string{"2"}, "dial_timeout": "10s", "tls": map[string]any{"server_name": u.Hostname(), "handshake_timeout": "10s"}},
		"flush_interval": -1,
	}
	cfg := map[string]any{
		"admin":   map[string]any{"disabled": true, "config": map[string]any{"persist": false}},
		"logging": map[string]any{"logs": map[string]any{"default": map[string]any{"writer": map[string]any{"output": "discard"}}}},
		"apps": map[string]any{"http": map[string]any{"grace_period": "5s", "servers": map[string]any{"forwarder": map[string]any{
			"listen": []string{c.Listen}, "protocols": []string{"h1", "h2c"}, "automatic_https": map[string]any{"disable": true},
			"routes": []any{map[string]any{"handle": []any{proxy}}},
		}}}},
	}
	return json.Marshal(cfg)
}
