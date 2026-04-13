package cmd

import (
	"fmt"

	"llmsnare/internal/storage"

	"github.com/spf13/cobra"
)

func newTimelineCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Maintain timeline storage",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newTimelineStatusCommand())
	cmd.AddCommand(newTimelineBackfillRunIDCommand())
	cmd.AddCommand(newTimelineRebuildSQLiteCommand())
	return cmd
}

func newTimelineStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show timeline WAL and SQLite projection status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			store := storage.New(cfg.Storage.TimelineDir)
			status, err := store.Status()
			if err != nil {
				return err
			}
			renderTimelineStatus(cmd, status)
			return nil
		},
	}
}

func newTimelineBackfillRunIDCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "backfill-run-id",
		Short: "Backfill missing run_id values in timeline WAL files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			store := storage.New(cfg.Storage.TimelineDir)
			changed, err := store.BackfillRunIDs()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backfilled %d WAL rows\n", changed)
			return nil
		},
	}
}

func newTimelineRebuildSQLiteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild-sqlite",
		Short: "Rebuild the SQLite timeline projection from WAL",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			store := storage.New(cfg.Storage.TimelineDir)
			stats, err := store.RebuildProjection()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "rebuilt sqlite projection with %d rows across %d profiles\n", stats.Rows, stats.Profiles)
			return nil
		},
	}
}

func renderTimelineStatus(cmd *cobra.Command, status storage.Status) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "read_backend: %s\n", status.ReadBackend)
	fmt.Fprintf(out, "timeline_dir: %s\n", status.TimelineDir)
	fmt.Fprintf(out, "sqlite_path: %s\n", status.SQLitePath)
	fmt.Fprintf(out, "wal_profiles: %d\n", status.WALProfiles)
	fmt.Fprintf(out, "wal_rows: %d\n", status.WALRows)
	fmt.Fprintf(out, "sqlite_exists: %t\n", status.SQLiteExists)
	fmt.Fprintf(out, "sqlite_ready: %t\n", status.SQLiteReady)
	fmt.Fprintf(out, "sqlite_dirty: %t\n", status.SQLiteDirty)
	if status.SQLiteExists {
		fmt.Fprintf(out, "sqlite_rows: %d\n", status.SQLiteRows)
	}
	if status.SQLiteSchemaVersion != "" {
		fmt.Fprintf(out, "sqlite_schema_version: %s\n", status.SQLiteSchemaVersion)
	}
	if status.SQLiteDirtyReason != "" {
		fmt.Fprintf(out, "sqlite_dirty_reason: %s\n", status.SQLiteDirtyReason)
	}
	if status.SQLiteInspectionError != "" {
		fmt.Fprintf(out, "sqlite_inspection_error: %s\n", status.SQLiteInspectionError)
	}
}
