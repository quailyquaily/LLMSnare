package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath      = "~/.config/llmsnare/config.yaml"
	defaultTimelineDir     = "~/.local/state/llmsnare/timeline"
	defaultListenAddress   = "127.0.0.1:8787"
	defaultTimeout         = 300 * time.Second
	defaultMaxOutputTokens = 4096
	defaultOpenAIAPI       = "https://api.openai.com/v1"
	defaultAnthropicAPI    = "https://api.anthropic.com"
	defaultGeminiAPI       = "https://generativelanguage.googleapis.com"
	defaultCloudflareAPI   = "https://api.cloudflare.com/client/v4"
)

var envPattern = regexp.MustCompile(`^\$\{([A-Z0-9_]+)\}$`)
var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Config struct {
	Version  int                `yaml:"version"`
	Serve    ServeConfig        `yaml:"serve"`
	Storage  StorageConfig      `yaml:"storage"`
	Profiles map[string]Profile `yaml:"profiles"`
}

type ServeConfig struct {
	Listen string `yaml:"listen"`
}

type StorageConfig struct {
	TimelineDir string `yaml:"timeline_dir"`
}

type Profile struct {
	Provider        string        `yaml:"provider"`
	Model           string        `yaml:"model"`
	Endpoint        string        `yaml:"endpoint"`
	APIKey          string        `yaml:"api_key"`
	AccountID       string        `yaml:"account_id"`
	APIToken        string        `yaml:"api_token"`
	Timeout         time.Duration `yaml:"-"`
	Temperature     float64       `yaml:"temperature"`
	MaxOutputTokens int           `yaml:"max_output_tokens"`

	TimeoutRaw string `yaml:"timeout"`
}

func DefaultConfigPath() (string, error) {
	return ExpandHome(defaultConfigPath)
}

func ExpandHome(value string) (string, error) {
	if !strings.HasPrefix(value, "~") {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("lookup home directory: %w", err)
	}
	if value == "~" {
		return home, nil
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, value[2:]), nil
	}
	return "", fmt.Errorf("unsupported home path %q", value)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.normalize(filepath.Dir(path)); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) normalize(baseDir string) error {
	if c.Version != 1 {
		return fmt.Errorf("config version must be 1")
	}
	if len(c.Profiles) == 0 {
		return fmt.Errorf("profiles must not be empty")
	}
	if c.Serve.Listen == "" {
		c.Serve.Listen = defaultListenAddress
	}

	var err error
	if c.Storage.TimelineDir == "" {
		c.Storage.TimelineDir = defaultTimelineDir
	}
	c.Storage.TimelineDir, err = resolvePath(baseDir, c.Storage.TimelineDir)
	if err != nil {
		return fmt.Errorf("resolve storage.timeline_dir: %w", err)
	}

	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !profileNamePattern.MatchString(name) {
			return fmt.Errorf("profile name %q is invalid; use letters, digits, dot, underscore, or dash", name)
		}

		profile := c.Profiles[name]
		if err := profile.normalize(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
		c.Profiles[name] = profile
	}

	return nil
}

func (p *Profile) normalize() error {
	p.Provider = strings.TrimSpace(p.Provider)
	switch p.Provider {
	case "openai", "anthropic", "gemini", "cloudflare":
	default:
		return fmt.Errorf("provider must be one of openai, anthropic, gemini, cloudflare")
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("model is required")
	}
	p.Endpoint = strings.TrimSpace(p.Endpoint)

	switch p.Provider {
	case "openai":
		if p.Endpoint == "" {
			p.Endpoint = defaultOpenAIAPI
		}
		resolvedKey, err := expandRequiredEnvRef("api_key", p.APIKey)
		if err != nil {
			return err
		}
		p.APIKey = resolvedKey
	case "anthropic":
		if p.Endpoint == "" {
			p.Endpoint = defaultAnthropicAPI
		}
		if strings.TrimRight(p.Endpoint, "/") != defaultAnthropicAPI {
			return fmt.Errorf("anthropic endpoint overrides are not supported by the configured uniai provider")
		}
		resolvedKey, err := expandRequiredEnvRef("api_key", p.APIKey)
		if err != nil {
			return err
		}
		p.APIKey = resolvedKey
	case "gemini":
		if p.Endpoint == "" {
			p.Endpoint = defaultGeminiAPI
		}
		resolvedKey, err := expandRequiredEnvRef("api_key", p.APIKey)
		if err != nil {
			return err
		}
		p.APIKey = resolvedKey
	case "cloudflare":
		if p.Endpoint == "" {
			p.Endpoint = defaultCloudflareAPI
		}
		resolvedAccountID, err := expandRequiredEnvRef("account_id", p.AccountID)
		if err != nil {
			return err
		}
		resolvedToken, err := expandRequiredEnvRef("api_token", p.APIToken)
		if err != nil {
			return err
		}
		p.AccountID = resolvedAccountID
		p.APIToken = resolvedToken
	}

	if p.TimeoutRaw == "" {
		p.Timeout = defaultTimeout
	} else {
		timeout, err := time.ParseDuration(p.TimeoutRaw)
		if err != nil {
			return fmt.Errorf("parse timeout: %w", err)
		}
		p.Timeout = timeout
	}
	if p.MaxOutputTokens == 0 {
		p.MaxOutputTokens = defaultMaxOutputTokens
	}
	return nil
}

func expandRequiredEnvRef(fieldName, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	matches := envPattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		return value, nil
	}
	resolved, ok := os.LookupEnv(matches[1])
	if !ok {
		return "", fmt.Errorf("%s environment variable %q is not set", fieldName, matches[1])
	}
	return resolved, nil
}

func TemplateYAML() string {
	return `version: 1

serve:
  listen: "127.0.0.1:8787"

storage:
  timeline_dir: "~/.local/state/llmsnare/timeline"

profiles:
  openai_gpt4o:
    provider: openai
    model: "gpt-4o"
    api_key: "${OPENAI_API_KEY}"
    timeout: 300s
    temperature: 0
    max_output_tokens: 4096
`
}

func resolvePath(baseDir, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "~") {
		return ExpandHome(value)
	}
	if filepath.IsAbs(value) {
		return value, nil
	}
	return filepath.Join(baseDir, value), nil
}
