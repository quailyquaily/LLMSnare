package benchcase

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultCaseRelPath   = "benchmarks/read_write_ratio_smoke_v1/case.yaml"
	defaultFixtureRelDir = "fixture"
)

type Case struct {
	Version       int               `yaml:"version"`
	ID            string            `yaml:"id"`
	Prompt        string            `yaml:"prompt"`
	FixtureDir    string            `yaml:"fixture_dir"`
	WritablePaths []string          `yaml:"writable_paths"`
	Scoring       Scoring           `yaml:"scoring"`
	Metrics       Metrics           `yaml:"metrics"`
	FixtureFiles  map[string]string `yaml:"-"`
}

type Scoring struct {
	Deductions []Rule `yaml:"deductions"`
	Bonuses    []Rule `yaml:"bonuses"`
}

type Metrics struct {
	VendorTrapRecovered *Check `yaml:"vendor_trap_recovered"`
	UtilTrapTriggered   *Check `yaml:"util_trap_triggered"`
}

type Rule struct {
	Name          string `yaml:"name"`
	Points        int    `yaml:"points"`
	Description   string `yaml:"description"`
	PerOccurrence bool   `yaml:"per_occurrence"`
	Check         Check  `yaml:"check"`
}

type Check struct {
	Type           string   `yaml:"type"`
	Path           string   `yaml:"path"`
	Paths          []string `yaml:"paths"`
	File           string   `yaml:"file"`
	WrongPath      string   `yaml:"wrong_path"`
	CorrectPath    string   `yaml:"correct_path"`
	ListDir        string   `yaml:"list_dir"`
	Threshold      float64  `yaml:"threshold"`
	Substrings     []string `yaml:"substrings"`
	RequiredCalls  []string `yaml:"required_calls"`
	ForbiddenRegex []string `yaml:"forbidden_regex"`
	ReferenceFile  string   `yaml:"reference_file"`
	TypeName       string   `yaml:"type_name"`
}

func Load(casePath string) (Case, error) {
	return load(casePath, "")
}

func LoadWithFixtureDir(casePath, fixtureDir string) (Case, error) {
	return load(casePath, fixtureDir)
}

func load(casePath, fixtureDirOverride string) (Case, error) {
	data, err := os.ReadFile(casePath)
	if err != nil {
		return Case{}, fmt.Errorf("read benchmark case: %w", err)
	}

	var out Case
	if err := yaml.Unmarshal(data, &out); err != nil {
		return Case{}, fmt.Errorf("parse benchmark case: %w", err)
	}

	baseDir := filepath.Dir(casePath)
	if strings.TrimSpace(fixtureDirOverride) != "" {
		out.FixtureDir = fixtureDirOverride
	}
	if err := out.normalize(baseDir); err != nil {
		return Case{}, err
	}
	return out, nil
}

func (c *Case) normalize(baseDir string) error {
	if c.Version != 1 {
		return fmt.Errorf("benchmark case version must be 1")
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("benchmark case id is required")
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("benchmark case prompt is required")
	}
	if strings.TrimSpace(c.FixtureDir) == "" {
		return fmt.Errorf("benchmark case fixture_dir is required")
	}

	c.FixtureDir = resolvePath(baseDir, c.FixtureDir)
	files, err := loadFixtureFiles(c.FixtureDir)
	if err != nil {
		return err
	}
	c.FixtureFiles = files
	c.WritablePaths = normalizePaths(c.WritablePaths)
	if err := normalizeRules(c.Scoring.Deductions); err != nil {
		return err
	}
	if err := normalizeRules(c.Scoring.Bonuses); err != nil {
		return err
	}
	if c.Metrics.VendorTrapRecovered != nil {
		c.Metrics.VendorTrapRecovered.normalize()
	}
	if c.Metrics.UtilTrapTriggered != nil {
		c.Metrics.UtilTrapTriggered.normalize()
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
	c.WrongPath = normalizeRelPath(c.WrongPath)
	c.CorrectPath = normalizeRelPath(c.CorrectPath)
	c.ListDir = normalizeDir(c.ListDir)
	c.ReferenceFile = normalizeRelPath(c.ReferenceFile)
	c.Paths = normalizePaths(c.Paths)
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, item := range paths {
		out = append(out, normalizeRelPath(item))
	}
	return out
}

func loadFixtureFiles(root string) (map[string]string, error) {
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
		return nil, fmt.Errorf("load fixture files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("fixture_dir %q did not contain any files", root)
	}
	return files, nil
}

func loadFixtureFilesFromFS(fsys fs.FS, root string) (map[string]string, error) {
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
		return nil, fmt.Errorf("load fixture files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("fixture_dir %q did not contain any files", root)
	}
	return files, nil
}

func resolvePath(baseDir, raw string) string {
	if strings.HasPrefix(raw, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if raw == "~" {
				return home
			}
			if strings.HasPrefix(raw, "~/") {
				return filepath.Join(home, raw[2:])
			}
		}
	}
	if filepath.IsAbs(raw) {
		return raw
	}
	return filepath.Join(baseDir, raw)
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

func normalizeDir(raw string) string {
	cleaned := normalizeRelPath(raw)
	if cleaned == "" {
		return ""
	}
	if !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

func DefaultFixtureRelDir() string {
	return defaultFixtureRelDir
}

func ExtractStructFields(source, typeName string) (map[string]struct{}, error) {
	file, err := parser.ParseFile(token.NewFileSet(), typeName+".go", source, 0)
	if err != nil {
		return nil, err
	}

	fields := map[string]struct{}{}
	found := false
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("%s is not a struct", typeName)
			}
			found = true
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = struct{}{}
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("struct %s not found", typeName)
	}
	return fields, nil
}
