package storage

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Status struct {
	TimelineDir           string
	SQLitePath            string
	ReadBackend           string
	WALProfiles           int
	WALRows               int
	SQLiteExists          bool
	SQLiteReady           bool
	SQLiteDirty           bool
	SQLiteRows            int
	SQLiteSchemaVersion   string
	SQLiteDirtyReason     string
	SQLiteInspectionError string
}

func (s *Store) Status() (Status, error) {
	status := Status{
		TimelineDir: s.dir,
		SQLitePath:  s.sqlitePath,
		ReadBackend: "wal",
	}

	names, err := s.walProfileNames()
	if err != nil {
		return Status{}, err
	}
	status.WALProfiles = len(names)

	for _, profile := range names {
		rows, err := countJSONLRows(filepath.Join(s.dir, profile+".jsonl"))
		if err != nil {
			return Status{}, err
		}
		status.WALRows += rows
	}

	if _, err := os.Stat(s.sqlitePath); err == nil {
		status.SQLiteExists = true
	} else if !os.IsNotExist(err) {
		return Status{}, fmt.Errorf("stat sqlite projection: %w", err)
	}

	if _, err := os.Stat(s.readyMarkerPath); err == nil {
		status.SQLiteReady = true
	} else if !os.IsNotExist(err) {
		return Status{}, fmt.Errorf("stat sqlite ready marker: %w", err)
	}

	if _, err := os.Stat(s.dirtyMarkerPath); err == nil {
		status.SQLiteDirty = true
		reason, readErr := os.ReadFile(s.dirtyMarkerPath)
		if readErr != nil {
			return Status{}, fmt.Errorf("read sqlite dirty marker: %w", readErr)
		}
		status.SQLiteDirtyReason = strings.TrimSpace(string(reason))
	} else if !os.IsNotExist(err) {
		return Status{}, fmt.Errorf("stat sqlite dirty marker: %w", err)
	}

	if status.SQLiteExists {
		if err := s.inspectSQLite(&status); err != nil {
			status.SQLiteInspectionError = err.Error()
		}
	}

	if s.projectionReady() {
		status.ReadBackend = "sqlite"
	}
	return status, nil
}

func countJSONLRows(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open timeline file: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan timeline file: %w", err)
	}
	return count, nil
}

func (s *Store) inspectSQLite(status *Status) error {
	db, err := openProjectionDB(s.sqlitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.QueryRow(`SELECT COUNT(*) FROM timeline_runs`).Scan(&status.SQLiteRows); err != nil {
		return fmt.Errorf("count sqlite projection rows: %w", err)
	}

	var schemaVersion sql.NullString
	if err := db.QueryRow(`SELECT value FROM projection_meta WHERE key = 'schema_version'`).Scan(&schemaVersion); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if schemaVersion.Valid {
		status.SQLiteSchemaVersion = schemaVersion.String
	}
	return nil
}
