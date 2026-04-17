package benchcase

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed testdata/builtin/benchmarks
var builtinFS embed.FS

type Scaffold struct {
	CaseRelPath string
	CaseYAML    string
	RootFSFiles map[string]string
}

func DefaultScaffolds() ([]Scaffold, error) {
	casePaths, err := builtinCasePaths()
	if err != nil {
		return nil, err
	}

	scaffolds := make([]Scaffold, 0, len(casePaths))
	for _, caseRelPath := range casePaths {
		scaffold, err := loadEmbeddedScaffold(caseRelPath)
		if err != nil {
			return nil, err
		}
		scaffolds = append(scaffolds, scaffold)
	}
	return scaffolds, nil
}

func builtinCasePaths() ([]string, error) {
	const root = "testdata/builtin/benchmarks"

	var casePaths []string
	err := fs.WalkDir(builtinFS, root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path.Base(current) != "case.yaml" {
			return nil
		}
		casePaths = append(casePaths, strings.TrimPrefix(current, "testdata/builtin/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list embedded cases: %w", err)
	}
	sort.Strings(casePaths)
	return casePaths, nil
}

func loadEmbeddedScaffold(caseRelPath string) (Scaffold, error) {
	caseFSPath := path.Join("testdata", "builtin", caseRelPath)
	data, err := fs.ReadFile(builtinFS, caseFSPath)
	if err != nil {
		return Scaffold{}, fmt.Errorf("read embedded case %q: %w", caseRelPath, err)
	}

	rootFSDir := path.Join(path.Dir(caseFSPath), DefaultRootFSRelDir())
	rootFSFiles, err := loadRootFSFilesFromFS(builtinFS, rootFSDir)
	if err != nil {
		return Scaffold{}, fmt.Errorf("load embedded rootfs for %q: %w", caseRelPath, err)
	}

	return Scaffold{
		CaseRelPath: caseRelPath,
		CaseYAML:    string(data),
		RootFSFiles: rootFSFiles,
	}, nil
}
