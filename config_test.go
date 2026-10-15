package main

import (
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	c, err := readConfig(func(k string) string {
		if k == "SUI_GRPC_API_KEY" {
			return "test-fixture-key"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:29010" || c.Upstream != "https://sui-mainnet-grpc.giverep.com:443" || c.APIKey != "test-fixture-key" {
		t.Fatal("incorrect defaults")
	}
}
func TestUnsafeConfigurationRejected(t *testing.T) {
	cases := []map[string]string{
		{"FORWARDER_LISTEN": "0.0.0.0:29010"}, {"FORWARDER_LISTEN": "localhost:29010"}, {"FORWARDER_LISTEN": "127.0.0.1:0"},
		{"SUI_GRPC_UPSTREAM": "http://example.org"}, {"SUI_GRPC_UPSTREAM": "https://user:password@example.org"}, {"SUI_GRPC_UPSTREAM": "https://example.org/path"}, {"SUI_GRPC_UPSTREAM": "https://example.org/?key=secret"},
		{"SUI_GRPC_API_KEY": "bad\r\nheader"}, {"SUI_GRPC_API_KEY": "{env.SECRET}"}, {"SUI_GRPC_API_KEY": "has space"},
		{"SUI_GRPC_API_KEY": strings.Repeat("x", 4097)}, {"FORWARDER_ALLOW_NON_LOOPBACK": "1"},
		{"SUI_GRPC_UPSTREAM": "https://{env.HOST}"}, {"SUI_GRPC_UPSTREAM": "https://example.org:word"}, {"SUI_GRPC_UPSTREAM": "https://example.org:0"}, {"SUI_GRPC_UPSTREAM": "https://example.org:65536"}, {"SUI_GRPC_UPSTREAM": "https://example.org:"}, {"SUI_GRPC_UPSTREAM": "https://example.org?"}, {"SUI_GRPC_UPSTREAM": "https://example.org/#fragment"}, {"SUI_GRPC_UPSTREAM": "https://example.org/#"},
	}
	for i, values := range cases {
		if _, ok := values["SUI_GRPC_API_KEY"]; !ok {
			values["SUI_GRPC_API_KEY"] = "test-fixture-key"
		}
		_, err := readConfig(func(k string) string { return values[k] })
		if err == nil {
			t.Errorf("case %d accepted unsafe configuration", i)
		}
	}
}
func TestExplicitPrivateNetworkBind(t *testing.T) {
	values := map[string]string{"SUI_GRPC_API_KEY": "test-fixture-key", "FORWARDER_LISTEN": "0.0.0.0:29010", "FORWARDER_ALLOW_NON_LOOPBACK": "true"}
	if _, err := readConfig(func(k string) string { return values[k] }); err != nil {
		t.Fatal(err)
	}
}
func TestMissingKeyFails(t *testing.T) {
	_, err := readConfig(func(string) string { return "" })
	if err == nil {
		t.Fatal("missing API key must fail closed")
	}
}
