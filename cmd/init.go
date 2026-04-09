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

			if !force {
				if err := ensureMissing(target); err != nil {
					return err
				}
				for _, scaffold := range scaffolds {
					casePath := filepath.Join(filepath.Dir(target), filepath.FromSlash(scaffold.CaseRelPath))
					if err := ensureMissing(casePath); err != nil {
						return err
					}
				}
			}

			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}
			if err := os.WriteFile(target, []byte(config.TemplateYAML()), 0o644); err != nil {
				return fmt.Errorf("write config template: %w", err)
			}
			written, err := writeDefaultCaseScaffolds(filepath.Dir(target), scaffolds)
			if err != nil {
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", target); err != nil {
				return err
			}
			for _, path := range written {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path); err != nil {
					return err
				}
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	return cmd
}

func ensureMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; rerun with --force to overwrite", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return nil
}

func writeDefaultCaseScaffolds(root string, scaffolds []benchcase.Scaffold) ([]string, error) {
	written := make([]string, 0, len(scaffolds))
	for _, scaffold := range scaffolds {
		casePath := filepath.Join(root, filepath.FromSlash(scaffold.CaseRelPath))
		if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
			return nil, fmt.Errorf("create benchmark case directory: %w", err)
		}
		if err := os.WriteFile(casePath, []byte(scaffold.CaseYAML), 0o644); err != nil {
			return nil, fmt.Errorf("write benchmark case: %w", err)
		}

		fixtureRoot := filepath.Join(filepath.Dir(casePath), benchcase.DefaultFixtureRelDir())
		for relPath, content := range scaffold.FixtureFiles {
			target := filepath.Join(fixtureRoot, filepath.FromSlash(relPath))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, fmt.Errorf("create fixture directory: %w", err)
			}
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				return nil, fmt.Errorf("write fixture file: %w", err)
			}
		}
		written = append(written, casePath)
	}
	return written, nil
}
