package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var configPath string

func ExecuteContext(ctx context.Context) error {
	return newRootCommand().ExecuteContext(ctx)
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "llmsnare",
		Short:         "Run the LLM context fidelity benchmark",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config.yaml")
	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newRunCommand())
	rootCmd.AddCommand(newServeCommand())
	return rootCmd
}
