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
  listen: "127.0.0.1:9999"
storage: {}
profiles:
  demo:
    provider: openai
    model: gpt-4o
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
	if got := cfg.Profiles["demo"].Endpoint; got != defaultOpenAIAPI {
		t.Fatalf("endpoint = %q, want %q", got, defaultOpenAIAPI)
	}
	if got := cfg.Serve.Listen; got != "127.0.0.1:9999" {
		t.Fatalf("listen = %q, want %q", got, "127.0.0.1:9999")
	}
}

func TestLoadRejectsAnthropicEndpointOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
storage: {}
profiles:
  demo:
    provider: anthropic
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

func TestLoadOpenAIRespProfile(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
storage: {}
profiles:
  demo:
    provider: openai_resp
    model: gpt-5.4
    api_key: ${OPENAI_API_KEY}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	profile := cfg.Profiles["demo"]
	if got := profile.APIKey; got != "secret" {
		t.Fatalf("API key = %q, want secret", got)
	}
	if got := profile.Endpoint; got != defaultOpenAIAPI {
		t.Fatalf("endpoint = %q, want %q", got, defaultOpenAIAPI)
	}
}

func TestLoadRejectsOpenAIRespEndpointOverride(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
storage: {}
profiles:
  demo:
    provider: openai_resp
    model: gpt-5.4
    endpoint: https://example.com/v1
    api_key: ${OPENAI_API_KEY}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded, want error")
	}
}

func TestLoadIgnoresUnknownBenchmarkField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
benchmark:
  case_file: "benchmarks/demo/case.yaml"
storage: {}
profiles:
  demo:
    provider: openai
    model: gpt-4o
    api_key: plain-text-key
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if _, ok := cfg.Profiles["demo"]; !ok {
		t.Fatal("expected demo profile to load")
	}
}

func TestLoadDefaultsServeListen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
storage: {}
profiles:
  demo:
    provider: openai
    model: gpt-4o
    api_key: plain-text-key
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := cfg.Serve.Listen; got != defaultListenAddress {
		t.Fatalf("listen = %q, want %q", got, defaultListenAddress)
	}
}

func TestLoadCloudflareProfile(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "account-123")
	t.Setenv("CLOUDFLARE_API_TOKEN", "token-456")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `version: 1
storage: {}
profiles:
  demo:
    provider: cloudflare
    model: "@cf/meta/llama-3.1-8b-instruct"
    account_id: ${CLOUDFLARE_ACCOUNT_ID}
    api_token: ${CLOUDFLARE_API_TOKEN}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	profile := cfg.Profiles["demo"]
	if got := profile.AccountID; got != "account-123" {
		t.Fatalf("account_id = %q, want %q", got, "account-123")
	}
	if got := profile.APIToken; got != "token-456" {
		t.Fatalf("api_token = %q, want %q", got, "token-456")
	}
	if got := profile.Endpoint; got != defaultCloudflareAPI {
		t.Fatalf("endpoint = %q, want %q", got, defaultCloudflareAPI)
	}
}
