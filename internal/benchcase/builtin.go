package benchcase

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
)

//go:embed testdata/builtin/benchmarks
var builtinFS embed.FS

type Scaffold struct {
	CaseRelPath string
	CaseYAML    string
	RootFSFiles map[string]string
}

func DefaultScaffolds() ([]Scaffold, error) {
	casePaths := []string{
		BuiltinCaseRelPath,
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
