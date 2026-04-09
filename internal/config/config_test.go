package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsAPIKeyAndDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
serve:
  interval: 6h
storage: {}
profiles:
  demo:
    driver: openai
    model: gpt-4o
    endpoint: https://api.openai.com/v1
    api_key: ${OPENAI_API_KEY}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.Profiles["demo"].APIKey; got != "secret" {
		t.Fatalf("API key = %q, want secret", got)
	}
	if cfg.Profiles["demo"].Timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want %v", cfg.Profiles["demo"].Timeout, defaultTimeout)
	}
	if cfg.Storage.TimelineDir == "" {
		t.Fatal("timeline dir should be defaulted")
	}
	if cfg.Benchmark.CaseFile == "" {
		t.Fatal("case file should be defaulted")
	}
}

func TestLoadRejectsAnthropicEndpointOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
serve:
  interval: 6h
storage: {}
profiles:
  demo:
    driver: anthropic
    model: claude
    endpoint: https://example.com
    api_key: ${ANTHROPIC_API_KEY}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded, want error")
	}
}
