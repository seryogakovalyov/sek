package config

import "testing"

func TestNormalizeHTTPAddrAddsMissingPortColon(t *testing.T) {
	var cfg Config
	cfg.MCP.HTTPAddr = "9000"

	cfg.Normalize()

	if cfg.MCP.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.MCP.HTTPAddr, ":9000")
	}
}

func TestNormalizeHTTPAddrLeavesHostPortUntouched(t *testing.T) {
	var cfg Config
	cfg.MCP.HTTPAddr = "127.0.0.1:9000"

	cfg.Normalize()

	if cfg.MCP.HTTPAddr != "127.0.0.1:9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.MCP.HTTPAddr, "127.0.0.1:9000")
	}
}
