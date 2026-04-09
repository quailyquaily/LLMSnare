package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"llmsnare/internal/benchcase"

	"github.com/spf13/cobra"
)

func newCasesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cases",
		Short: "List available benchmark cases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, warnings, root, err := listCases()
			if err != nil {
				return err
			}
			renderCaseList(cmd.OutOrStdout(), items, root)
			renderCaseWarnings(cmd.ErrOrStderr(), warnings, root)
			return nil
		},
	}
	return cmd
}

func renderCaseList(out io.Writer, items []benchcase.Summary, root string) {
	style := newANSIStyle(out)
	fmt.Fprintln(out, style.header("Available Cases"))
	if len(items) == 0 {
		fmt.Fprintln(out, "- no cases found")
		return
	}

	for i, item := range items {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "- %s\n", style.emphasis(item.ID))
		fmt.Fprintf(out, "  dir: %s\n", relativeCaseDir(root, item.Dir))
		fmt.Fprintf(out, "  rootfs files: %d\n", item.RootFSFiles)
		fmt.Fprintf(out, "  writable paths: %d\n", item.WritablePaths)
		fmt.Fprintf(out, "  prompt: %s\n", item.PromptSummary)
	}
}

func renderCaseWarnings(out io.Writer, warnings []benchcase.ListWarning, root string) {
	if len(warnings) == 0 {
		return
	}

	style := newANSIStyle(out)
	for _, warning := range warnings {
		fmt.Fprintf(out, "%s skipped case %s: %s\n",
			style.warn("warning:"),
			relativeCaseDir(root, warning.Dir),
			warning.Message,
		)
	}
}

func relativeCaseDir(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return filepath.ToSlash(dir)
	}
	return filepath.ToSlash(rel)
}
