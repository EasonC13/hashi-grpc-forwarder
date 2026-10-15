package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLicensesWithoutCredentials(t *testing.T) {
	var out, err bytes.Buffer
	if code := run([]string{"--licenses"}, &out, &err); code != 0 {
		t.Fatalf("licenses command failed with code%d", code)
	}
	if !strings.Contains(out.String(), "Apache License") || !strings.Contains(out.String(), "Caddy") {
		t.Fatal("embedded dependency licenses missing")
	}
}
func TestVersionWithoutCredentials(t *testing.T) {
	var out, err bytes.Buffer
	if code := run([]string{"--version"}, &out, &err); code != 0 || !strings.Contains(out.String(), "hashi-grpc-forwarder") {
		t.Fatal("version unavailable")
	}
}
