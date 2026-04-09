package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"llmsnare/internal/benchcase"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath      = "~/.config/llmsnare/config.yaml"
	defaultTimelineDir     = "~/.local/state/llmsnare/timeline"
	defaultListenAddress   = "127.0.0.1:8787"
	defaultTimeout         = 90 * time.Second
	defaultMaxOutputTokens = 4096
	defaultAnthropicAPI    = "https://api.anthropic.com"
)

var envPattern = regexp.MustCompile(`^\$\{([A-Z0-9_]+)\}$`)
var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Config struct {
	Version   int                `yaml:"version"`
	Benchmark BenchmarkConfig    `yaml:"benchmark"`
	Serve     ServeConfig        `yaml:"serve"`
	Storage   StorageConfig      `yaml:"storage"`
	Profiles  map[string]Profile `yaml:"profiles"`
}

type BenchmarkConfig struct {
	CaseFile string `yaml:"case_file"`
}

type ServeConfig struct {
	Interval time.Duration `yaml:"-"`
	Listen   string        `yaml:"listen"`

	IntervalRaw string `yaml:"interval"`
}

type StorageConfig struct {
	TimelineDir string `yaml:"timeline_dir"`
}

type Profile struct {
	Driver          string        `yaml:"driver"`
	Model           string        `yaml:"model"`
	Endpoint        string        `yaml:"endpoint"`
	APIKey          string        `yaml:"api_key"`
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
	if c.Serve.IntervalRaw == "" {
		return fmt.Errorf("serve.interval is required")
	}
	interval, err := time.ParseDuration(c.Serve.IntervalRaw)
	if err != nil {
		return fmt.Errorf("parse serve.interval: %w", err)
	}
	c.Serve.Interval = interval
	if c.Serve.Listen == "" {
		c.Serve.Listen = defaultListenAddress
	}

	if c.Benchmark.CaseFile == "" {
		c.Benchmark.CaseFile = benchcase.DefaultCaseRelPath
	}
	c.Benchmark.CaseFile, err = resolvePath(baseDir, c.Benchmark.CaseFile)
	if err != nil {
		return fmt.Errorf("resolve benchmark.case_file: %w", err)
	}

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
	switch p.Driver {
	case "openai", "anthropic", "gemini":
	default:
		return fmt.Errorf("driver must be one of openai, anthropic, gemini")
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if strings.TrimSpace(p.Endpoint) == "" {
		return fmt.Errorf("endpoint is required")
	}
	if p.Driver == "anthropic" && strings.TrimRight(p.Endpoint, "/") != defaultAnthropicAPI {
		return fmt.Errorf("anthropic endpoint overrides are not supported by the configured uniai provider")
	}

	resolvedKey, err := expandAPIKey(p.APIKey)
	if err != nil {
		return err
	}
	p.APIKey = resolvedKey

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

func expandAPIKey(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("api_key is required")
	}
	matches := envPattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		return value, nil
	}
	resolved, ok := os.LookupEnv(matches[1])
	if !ok {
		return "", fmt.Errorf("api_key environment variable %q is not set", matches[1])
	}
	return resolved, nil
}

func TemplateYAML() string {
	return `version: 1

benchmark:
  case_file: "` + benchcase.DefaultCaseRelPath + `"

serve:
  interval: 6h
  listen: "127.0.0.1:8787"

storage:
  timeline_dir: "~/.local/state/llmsnare/timeline"

profiles:
  openai_gpt4o:
    driver: openai
    model: "gpt-4o"
    endpoint: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"
    timeout: 90s
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
