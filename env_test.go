package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvFileLoadsWithoutOverridingEnvironment(t *testing.T) {
	t.Setenv("SUI_GRPC_API_KEY", "existing-fixture-key")
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte("SUI_GRPC_API_KEY=other-fixture-key\nFORWARDER_TEST_LOADED=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORWARDER_TEST_LOADED", "")
	os.Unsetenv("FORWARDER_TEST_LOADED")
	if err := loadEnvFile(p); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FORWARDER_TEST_LOADED") != "loaded" {
		t.Fatal("env file not loaded")
	}
	if os.Getenv("SUI_GRPC_API_KEY") != "existing-fixture-key" {
		t.Fatal("existing environment was overwritten")
	}
}
func TestMalformedEnvDoesNotExposeContents(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("not a valid ! key = secret-fixture"), 0600)
	if err := loadEnvFile(p); err == nil {
		t.Fatal("malformed env accepted")
	}
}
