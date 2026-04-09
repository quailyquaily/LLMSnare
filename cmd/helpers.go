package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func loadCase(caseRef string) (benchcase.Case, error) {
	caseDir, err := resolveCaseDir(caseRef)
	if err != nil {
		return benchcase.Case{}, err
	}

	caseDef, err := benchcase.LoadDir(caseDir)
	if err != nil {
		return benchcase.Case{}, fmt.Errorf("load benchmark case: %w", err)
	}
	return caseDef, nil
}

func listCases() ([]benchcase.Summary, []benchcase.ListWarning, string, error) {
	root, err := resolveCasesRoot()
	if err != nil {
		return nil, nil, "", err
	}
	items, warnings, err := benchcase.List(root)
	if err != nil {
		return nil, nil, "", err
	}
	return items, warnings, root, nil
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

func resolveCasesRoot() (string, error) {
	configFile, err := resolveConfigPath(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configFile), benchcase.DefaultCasesRelDir), nil
}

func resolveCaseDir(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		root, err := resolveCasesRoot()
		if err != nil {
			return "", err
		}
		items, warnings, err := benchcase.List(root)
		if err != nil {
			return "", err
		}
		switch {
		case len(items) == 0 && len(warnings) == 0:
			return "", fmt.Errorf("no cases found under %s; run `llmsnare init`", root)
		default:
			return "", fmt.Errorf("no --case provided; run `llmsnare cases` and pass --case <case_id|case_dir>")
		}
	}

	if looksLikePath(ref) {
		dir, err := resolveOverridePath(ref)
		if err != nil {
			return "", fmt.Errorf("resolve --case: %w", err)
		}
		if filepath.Base(dir) == "case.yaml" {
			return "", fmt.Errorf("--case must point to a case directory, not case.yaml")
		}
		return dir, nil
	}

	root, err := resolveCasesRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(ref)), nil
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

func looksLikePath(raw string) bool {
	return filepath.IsAbs(raw) ||
		strings.HasPrefix(raw, ".") ||
		strings.HasPrefix(raw, "~") ||
		strings.Contains(raw, "/") ||
		strings.Contains(raw, string(os.PathSeparator))
}
