package cmd

import (
	"fmt"
	"path/filepath"

	"llmsnare/internal/benchcase"
	"llmsnare/internal/config"
)

func loadConfig() (config.Config, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func loadCase(cfg config.Config, casePathOverride, fixtureDirOverride string) (benchcase.Case, error) {
	casePath := cfg.Benchmark.CaseFile
	var err error
	if casePathOverride != "" {
		casePath, err = resolveOverridePath(casePathOverride)
		if err != nil {
			return benchcase.Case{}, fmt.Errorf("resolve --case: %w", err)
		}
	}

	fixturePath := ""
	if fixtureDirOverride != "" {
		fixturePath, err = resolveOverridePath(fixtureDirOverride)
		if err != nil {
			return benchcase.Case{}, fmt.Errorf("resolve --fixture-dir: %w", err)
		}
	}

	caseDef, err := benchcase.LoadWithFixtureDir(casePath, fixturePath)
	if err != nil {
		return benchcase.Case{}, fmt.Errorf("load benchmark case: %w", err)
	}
	return caseDef, nil
}

func resolveConfigPath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = config.DefaultConfigPath()
		if err != nil {
			return "", err
		}
	}
	resolved, err := config.ExpandHome(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return resolved, nil
}

func resolveOverridePath(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if resolved, err := config.ExpandHome(raw); err == nil && resolved != raw {
		return resolved, nil
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	return filepath.Abs(raw)
}
