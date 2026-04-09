package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"llmsnare/internal/api"
	"llmsnare/internal/storage"

	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Expose timelines over HTTP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			store := storage.New(cfg.Storage.TimelineDir)
			if err := store.EnsureDir(); err != nil {
				return err
			}

			server := api.NewServer(store)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.ListenAndServe(ctx, cfg.Serve.Listen)
			}()

			fmt.Fprintf(cmd.OutOrStdout(), "listening on %s\n", cfg.Serve.Listen)
			select {
			case <-ctx.Done():
				return nil
			case err := <-errCh:
				return err
			}
		},
	}
	return cmd
}
