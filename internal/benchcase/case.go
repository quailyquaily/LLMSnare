package benchcase

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	BuiltinCaseID       = "read_write_ratio_sample"
	DefaultCasesRelDir  = "benchmarks"
	BuiltinCaseRelPath  = DefaultCasesRelDir + "/" + BuiltinCaseID + "/case.yaml"
	defaultRootFSRelDir = "rootfs"
)

type Case struct {
	Version       int               `yaml:"version"`
	ID            string            `yaml:"id"`
	Prompt        string            `yaml:"prompt"`
	WritablePaths []string          `yaml:"writable_paths"`
	Scoring       Scoring           `yaml:"scoring"`
	Dir           string            `yaml:"-"`
	RootFSFiles   map[string]string `yaml:"-"`
}

type Summary struct {
	ID            string
	Dir           string
	PromptSummary string
	WritablePaths int
	RootFSFiles   int
}

type ListWarning struct {
	Dir     string
	Message string
}

type Scoring struct {
	Deductions []Rule `yaml:"deductions"`
	Bonuses    []Rule `yaml:"bonuses"`
}

type Rule struct {
	Name          string `yaml:"name"`
	Points        int    `yaml:"points"`
	Description   string `yaml:"description"`
	PerOccurrence bool   `yaml:"per_occurrence"`
	Check         Check  `yaml:"check"`
}

type Check struct {
	Type         string   `yaml:"type"`
	Path         string   `yaml:"path"`
	Paths        []string `yaml:"paths"`
	File         string   `yaml:"file"`
	FunctionName string   `yaml:"function_name"`
	Regex        []string `yaml:"regex"`
	Threshold    float64  `yaml:"threshold"`
	Substrings   []string `yaml:"substrings"`
}

func LoadDir(caseDir string) (Case, error) {
	caseDir = filepath.Clean(caseDir)
	data, err := os.ReadFile(filepath.Join(caseDir, "case.yaml"))
	if err != nil {
		return Case{}, fmt.Errorf("read benchmark case: %w", err)
	}

	var out Case
	if err := yaml.Unmarshal(data, &out); err != nil {
		return Case{}, fmt.Errorf("parse benchmark case: %w", err)
	}
	out.Dir = caseDir
	if err := out.normalize(); err != nil {
		return Case{}, err
	}
	return out, nil
}

func Load(casePath string) (Case, error) {
	if filepath.Base(casePath) != "case.yaml" {
		return LoadDir(casePath)
	}
	return LoadDir(filepath.Dir(casePath))
}

func List(root string) ([]Summary, []ListWarning, error) {
	root = filepath.Clean(root)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("stat benchmark cases root: %w", err)
	}

	items := make([]Summary, 0)
	warnings := make([]ListWarning, 0)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			if filepath.Clean(current) == root {
				return err
			}
			warnings = append(warnings, ListWarning{
				Dir:     current,
				Message: err.Error(),
			})
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Name() != "case.yaml" {
			return nil
		}

		caseDir := filepath.Dir(current)
		caseDef, loadErr := LoadDir(caseDir)
		if loadErr != nil {
			warnings = append(warnings, ListWarning{
				Dir:     caseDir,
				Message: loadErr.Error(),
			})
			return nil
		}
		items = append(items, Summary{
			ID:            caseDef.ID,
			Dir:           caseDir,
			PromptSummary: summarizePrompt(caseDef.Prompt),
			WritablePaths: len(caseDef.WritablePaths),
			RootFSFiles:   len(caseDef.RootFSFiles),
		})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("list benchmark cases: %w", err)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ID == items[j].ID {
			return items[i].Dir < items[j].Dir
		}
		return items[i].ID < items[j].ID
	})
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Dir == warnings[j].Dir {
			return warnings[i].Message < warnings[j].Message
		}
		return warnings[i].Dir < warnings[j].Dir
	})
	return items, warnings, nil
}

func (c *Case) normalize() error {
	if c.Version != 1 {
		return fmt.Errorf("benchmark case version must be 1")
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("benchmark case id is required")
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("benchmark case prompt is required")
	}
	if strings.TrimSpace(c.Dir) == "" {
		return fmt.Errorf("benchmark case directory is required")
	}

	rootFSDir := filepath.Join(c.Dir, defaultRootFSRelDir)
	files, err := loadRootFSFiles(rootFSDir)
	if err != nil {
		return err
	}
	c.RootFSFiles = files
	c.WritablePaths = normalizePaths(c.WritablePaths)
	if err := normalizeRules(c.Scoring.Deductions); err != nil {
		return err
	}
	if err := normalizeRules(c.Scoring.Bonuses); err != nil {
		return err
	}
	return nil
}

func normalizeRules(rules []Rule) error {
	for i := range rules {
		if strings.TrimSpace(rules[i].Name) == "" {
			return fmt.Errorf("every scoring rule must set name")
		}
		rules[i].Check.normalize()
	}
	return nil
}

func (c *Check) normalize() {
	c.Path = normalizeRelPath(c.Path)
	c.File = normalizeRelPath(c.File)
	c.Paths = normalizePaths(c.Paths)
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, item := range paths {
		if cleaned := normalizeRelPath(item); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func loadRootFSFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		data, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		files[path.Clean(filepath.ToSlash(rel))] = string(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load case rootfs: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("case rootfs %q did not contain any files", root)
	}
	return files, nil
}

func loadRootFSFilesFromFS(fsys fs.FS, root string) (map[string]string, error) {
	files := map[string]string{}
	err := fs.WalkDir(fsys, root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		data, readErr := fs.ReadFile(fsys, current)
		if readErr != nil {
			return readErr
		}
		rel := strings.TrimPrefix(current, root+"/")
		files[path.Clean(rel)] = string(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load case rootfs: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("case rootfs %q did not contain any files", root)
	}
	return files, nil
}

func normalizeRelPath(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	cleaned := path.Clean(filepath.ToSlash(strings.TrimSpace(raw)))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func DefaultRootFSRelDir() string {
	return defaultRootFSRelDir
}

func summarizePrompt(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 72 {
			return line[:69] + "..."
		}
		return line
	}
	return ""
}
