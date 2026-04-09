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
	CaseRelPath  string
	CaseYAML     string
	FixtureFiles map[string]string
}

func DefaultScaffolds() ([]Scaffold, error) {
	casePaths := []string{
		DefaultCaseRelPath,
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

	fixtureRoot := path.Join(path.Dir(caseFSPath), DefaultFixtureRelDir())
	fixtureFiles, err := loadFixtureFilesFromFS(builtinFS, fixtureRoot)
	if err != nil {
		return Scaffold{}, fmt.Errorf("load embedded fixture for %q: %w", caseRelPath, err)
	}

	return Scaffold{
		CaseRelPath:  caseRelPath,
		CaseYAML:     string(data),
		FixtureFiles: fixtureFiles,
	}, nil
}
