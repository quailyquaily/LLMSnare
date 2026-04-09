package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"llmsnare/internal/api"
	"llmsnare/internal/benchcase"
	"llmsnare/internal/benchmark"
	"llmsnare/internal/config"
	"llmsnare/internal/storage"

	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var caseRef string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the benchmark on a schedule and expose timelines over HTTP",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			caseDef, err := loadCase(caseRef)
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

			go runScheduledBenchmarks(ctx, caseDef, cfg, store)

			fmt.Fprintf(cmd.OutOrStdout(), "listening on %s\n", cfg.Serve.Listen)
			select {
			case <-ctx.Done():
				return nil
			case err := <-errCh:
				return err
			}
		},
	}
	cmd.Flags().StringVar(&caseRef, "case", "", "Case ID or case directory path")
	return cmd
}

func runScheduledBenchmarks(ctx context.Context, caseDef benchcase.Case, cfg config.Config, store *storage.Store) {
	runner := benchmark.NewRunner()
	runAll := func() {
		names := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			select {
			case <-ctx.Done():
				return
			default:
			}
			result, err := runner.Run(ctx, caseDef, name, cfg.Profiles[name])
			if err != nil {
				continue
			}
			_ = store.Append(result)
		}
	}

	runAll()

	ticker := time.NewTicker(cfg.Serve.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runAll()
		}
	}
}
