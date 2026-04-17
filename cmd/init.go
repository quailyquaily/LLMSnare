package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"llmsnare/internal/benchcase"
	"llmsnare/internal/config"

	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a template config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveConfigPath(configPath)
			if err != nil {
				return err
			}
			scaffolds, err := benchcase.DefaultScaffolds()
			if err != nil {
				return err
			}

			configWritten, err := writeInitFile(target, []byte(config.TemplateYAML()), force)
			if err != nil {
				return fmt.Errorf("write config template: %w", err)
			}
			results, err := writeDefaultCaseScaffolds(filepath.Dir(target), scaffolds, force)
			if err != nil {
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", initAction(configWritten), target); err != nil {
				return err
			}
			for _, result := range results {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", initAction(result.Written), result.Path); err != nil {
					return err
				}
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config and built-in benchmark files")
	return cmd
}

type initWriteResult struct {
	Path    string
	Written bool
}

func initAction(written bool) string {
	if written {
		return "wrote"
	}
	return "skipped"
}

func writeInitFile(path string, content []byte, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("stat %s: %w", path, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func writeDefaultCaseScaffolds(root string, scaffolds []benchcase.Scaffold, force bool) ([]initWriteResult, error) {
	results := make([]initWriteResult, 0, len(scaffolds))
	for _, scaffold := range scaffolds {
		casePath := filepath.Join(root, filepath.FromSlash(scaffold.CaseRelPath))
		wroteCase, err := writeInitFile(casePath, []byte(scaffold.CaseYAML), force)
		if err != nil {
			return nil, fmt.Errorf("write benchmark case: %w", err)
		}

		rootFSDir := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir())
		for relPath, content := range scaffold.RootFSFiles {
			target := filepath.Join(rootFSDir, filepath.FromSlash(relPath))
			if _, err := writeInitFile(target, []byte(content), force); err != nil {
				return nil, fmt.Errorf("write rootfs file: %w", err)
			}
		}
		results = append(results, initWriteResult{
			Path:    casePath,
			Written: wroteCase,
		})
	}
	return results, nil
}
