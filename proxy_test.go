package main

import (
	"encoding/json"
	"testing"
)

func TestProxySecurityConfig(t *testing.T) {
	data, err := buildCaddyConfig(Config{Listen: "127.0.0.1:29010", Upstream: "https://example.org:443", APIKey: "test-fixture-key"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	admin, ok := cfg["admin"].(map[string]any)
	if !ok || admin["disabled"] != true {
		t.Fatal("admin must be disabled")
	}
	conf := admin["config"].(map[string]any)
	if conf["persist"] != false {
		t.Fatal("secret config must not be persisted")
	}
	logs := cfg["logging"].(map[string]any)["logs"].(map[string]any)["default"].(map[string]any)["writer"].(map[string]any)
	if logs["output"] != "discard" {
		t.Fatal("credential-bearing internal logs must be disabled")
	}
	server := cfg["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["forwarder"].(map[string]any)
	if server["automatic_https"].(map[string]any)["disable"] != true {
		t.Fatal("local listener must not obtain certificates")
	}
	route := server["routes"].([]any)[0].(map[string]any)["handle"].([]any)[0].(map[string]any)
	transport := route["transport"].(map[string]any)
	if transport["versions"].([]any)[0] != "2" {
		t.Fatal("upstream must use HTTP/2")
	}
	tls := transport["tls"].(map[string]any)
	if tls["insecure_skip_verify"] == true {
		t.Fatal("TLS verification cannot be disabled")
	}
}
