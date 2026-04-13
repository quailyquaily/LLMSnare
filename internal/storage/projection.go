package storage

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"llmsnare/internal/benchmark"

	_ "modernc.org/sqlite"
)

const (
	projectionSchemaVersion = "2"
	maxJSONLLineBytes       = 10 * 1024 * 1024
)

type ProjectionStats struct {
	Rows           int
	Profiles       int
	BackfilledRows int
}

func normalizeFilter(filter TimelineFilter) TimelineFilter {
	filter.Model = strings.TrimSpace(filter.Model)
	filter.ModelVendor = strings.TrimSpace(filter.ModelVendor)
	filter.InferenceProvider = strings.TrimSpace(filter.InferenceProvider)
	filter.CaseID = strings.TrimSpace(filter.CaseID)
	return filter
}

func matchesFilter(result benchmark.Result, filter TimelineFilter) bool {
	filter = normalizeFilter(filter)
	if filter.Model != "" && result.Model != filter.Model {
		return false
	}
	if filter.ModelVendor != "" && result.ModelVendor != filter.ModelVendor {
		return false
	}
	if filter.InferenceProvider != "" && result.InferenceProvider != filter.InferenceProvider {
		return false
	}
	if filter.CaseID != "" && result.CaseID != filter.CaseID {
		return false
	}
	return true
}

func (s *Store) projectionReady() bool {
	if _, err := os.Stat(s.dirtyMarkerPath); err == nil {
		return false
	} else if !os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(s.readyMarkerPath); err != nil {
		return false
	}
	if _, err := os.Stat(s.sqlitePath); err != nil {
		return false
	}
	return true
}

func (s *Store) projectionVersion() (string, error) {
	info, err := os.Stat(s.sqlitePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil
		}
		return "", fmt.Errorf("stat sqlite projection: %w", err)
	}
	return fileVersion(info), nil
}

func (s *Store) markProjectionDirty(err error) error {
	if ensureErr := s.EnsureDir(); ensureErr != nil {
		return ensureErr
	}
	reason := "projection is dirty"
	if err != nil {
		reason = err.Error()
	}
	return os.WriteFile(s.dirtyMarkerPath, []byte(reason+"\n"), 0o644)
}

func (s *Store) clearProjectionDirty() error {
	if err := os.Remove(s.dirtyMarkerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove projection dirty marker: %w", err)
	}
	return nil
}

func (s *Store) markProjectionReady() error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	return os.WriteFile(s.readyMarkerPath, []byte("ready\n"), 0o644)
}

func (s *Store) walProfileNames() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read timeline dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".jsonl"))
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) BackfillRunIDs() (int, error) {
	if err := s.EnsureDir(); err != nil {
		return 0, err
	}

	names, err := s.walProfileNames()
	if err != nil {
		return 0, err
	}

	total := 0
	for _, profile := range names {
		changed, err := s.backfillRunIDsForFile(filepath.Join(s.dir, profile+".jsonl"))
		if err != nil {
			return total, err
		}
		total += changed
	}
	if total > 0 {
		if err := s.markProjectionDirty(nil); err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *Store) backfillRunIDsForFile(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open timeline file: %w", err)
	}
	defer file.Close()

	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return 0, fmt.Errorf("create temp timeline file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		_ = tempFile.Close()
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	changed := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		item, shouldRewrite, err := lineWithRunID(line)
		if err != nil {
			return changed, err
		}

		if !shouldRewrite {
			if _, err := tempFile.Write(append(append([]byte(nil), line...), '\n')); err != nil {
				return changed, fmt.Errorf("write temp timeline line: %w", err)
			}
			continue
		}

		changed++
		data, err := json.Marshal(item)
		if err != nil {
			return changed, fmt.Errorf("marshal backfilled timeline line: %w", err)
		}
		if _, err := tempFile.Write(append(data, '\n')); err != nil {
			return changed, fmt.Errorf("write temp timeline line: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return changed, fmt.Errorf("scan timeline file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return changed, fmt.Errorf("close temp timeline file: %w", err)
	}
	if changed == 0 {
		return 0, nil
	}
	if err := os.Rename(tempPath, path); err != nil {
		return changed, fmt.Errorf("replace timeline file: %w", err)
	}
	cleanupTemp = false
	return changed, nil
}

func lineWithRunID(line []byte) (benchmark.Result, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return benchmark.Result{}, false, fmt.Errorf("decode timeline line: %w", err)
	}

	var item benchmark.Result
	if err := json.Unmarshal(line, &item); err != nil {
		return benchmark.Result{}, false, fmt.Errorf("decode timeline line: %w", err)
	}

	rawRunID, hasRunID := raw["run_id"]
	if hasRunID {
		var runID string
		if err := json.Unmarshal(rawRunID, &runID); err == nil && strings.TrimSpace(runID) != "" {
			return item, false, nil
		}
	}

	if err := ensureRunID(&item); err != nil {
		return benchmark.Result{}, false, err
	}
	return item, true, nil
}

func (s *Store) RebuildProjection() (ProjectionStats, error) {
	if err := s.EnsureDir(); err != nil {
		return ProjectionStats{}, err
	}

	names, err := s.walProfileNames()
	if err != nil {
		return ProjectionStats{}, err
	}

	tempPath := s.sqlitePath + ".tmp"
	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		return ProjectionStats{}, fmt.Errorf("remove temp sqlite projection: %w", err)
	}

	db, err := openProjectionDB(tempPath)
	if err != nil {
		return ProjectionStats{}, err
	}
	defer func() {
		_ = db.Close()
	}()

	if err := ensureProjectionSchema(db); err != nil {
		return ProjectionStats{}, err
	}

	tx, err := db.Begin()
	if err != nil {
		return ProjectionStats{}, fmt.Errorf("begin sqlite projection rebuild: %w", err)
	}
	stats := ProjectionStats{Profiles: len(names)}
	for _, profile := range names {
		path := filepath.Join(s.dir, profile+".jsonl")
		rows, err := loadJSONL(path, 0, TimelineFilter{})
		if err != nil {
			_ = tx.Rollback()
			return stats, err
		}
		for _, row := range rows {
			if strings.TrimSpace(row.RunID) == "" {
				_ = tx.Rollback()
				return stats, fmt.Errorf("timeline WAL is missing run_id; run timeline backfill-run-id first")
			}
			if err := upsertProjectionRow(tx, row); err != nil {
				_ = tx.Rollback()
				return stats, err
			}
			stats.Rows++
		}
	}
	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit sqlite projection rebuild: %w", err)
	}

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM timeline_runs`).Scan(&rowCount); err != nil {
		return stats, fmt.Errorf("count sqlite projection rows: %w", err)
	}
	if rowCount != stats.Rows {
		return stats, fmt.Errorf("sqlite projection row count mismatch: wrote %d rows, found %d rows", stats.Rows, rowCount)
	}

	if err := db.Close(); err != nil {
		return stats, fmt.Errorf("close sqlite projection: %w", err)
	}
	if err := os.Rename(tempPath, s.sqlitePath); err != nil {
		return stats, fmt.Errorf("replace sqlite projection: %w", err)
	}
	if err := s.clearProjectionDirty(); err != nil {
		return stats, err
	}
	if err := s.markProjectionReady(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (s *Store) upsertProjection(result benchmark.Result) error {
	db, err := openProjectionDB(s.sqlitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureProjectionSchema(db); err != nil {
		return err
	}
	return upsertProjectionRow(db, result)
}

func openProjectionDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite projection: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite projection: %w", err)
		}
	}
	return db, nil
}

func ensureProjectionSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projection_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS timeline_runs (
			run_id TEXT PRIMARY KEY,
			timestamp INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			case_id TEXT NOT NULL,
			profile TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			model_vendor TEXT NOT NULL,
			inference_provider TEXT NOT NULL,
			success INTEGER NOT NULL,
			total_score INTEGER NOT NULL,
			raw_score INTEGER NOT NULL,
			max_score INTEGER NOT NULL,
			normalized_score REAL NOT NULL,
			metrics_json TEXT NOT NULL,
			deductions_json TEXT NOT NULL,
			bonuses_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_profile_finished_at ON timeline_runs (profile, finished_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_model_vendor_finished_at ON timeline_runs (model_vendor, finished_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_inference_provider_finished_at ON timeline_runs (inference_provider, finished_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_model_vendor_inference_provider_profile_finished_at ON timeline_runs (model_vendor, inference_provider, profile, finished_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_model_profile_finished_at ON timeline_runs (model, profile, finished_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_runs_case_id_profile_finished_at ON timeline_runs (case_id, profile, finished_at DESC)`,
		`INSERT INTO projection_meta (key, value) VALUES ('schema_version', ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
	}
	for i, stmt := range stmts {
		if i == len(stmts)-1 {
			if _, err := db.Exec(stmt, projectionSchemaVersion); err != nil {
				return fmt.Errorf("initialize sqlite projection: %w", err)
			}
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("initialize sqlite projection: %w", err)
		}
	}
	return nil
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertProjectionRow(execer sqlExecer, result benchmark.Result) error {
	if strings.TrimSpace(result.RunID) == "" {
		return fmt.Errorf("sqlite projection row is missing run_id")
	}

	metricsJSON, err := json.Marshal(result.Metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics_json: %w", err)
	}
	deductionsJSON, err := json.Marshal(result.Deductions)
	if err != nil {
		return fmt.Errorf("marshal deductions_json: %w", err)
	}
	bonusesJSON, err := json.Marshal(result.Bonuses)
	if err != nil {
		return fmt.Errorf("marshal bonuses_json: %w", err)
	}

	_, err = execer.Exec(`
		INSERT INTO timeline_runs (
			run_id, timestamp, finished_at, case_id, profile, provider, model,
			model_vendor, inference_provider, success, total_score, raw_score,
			max_score, normalized_score, metrics_json, deductions_json, bonuses_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			timestamp=excluded.timestamp,
			finished_at=excluded.finished_at,
			case_id=excluded.case_id,
			profile=excluded.profile,
			provider=excluded.provider,
			model=excluded.model,
			model_vendor=excluded.model_vendor,
			inference_provider=excluded.inference_provider,
			success=excluded.success,
			total_score=excluded.total_score,
			raw_score=excluded.raw_score,
			max_score=excluded.max_score,
			normalized_score=excluded.normalized_score,
			metrics_json=excluded.metrics_json,
			deductions_json=excluded.deductions_json,
			bonuses_json=excluded.bonuses_json
	`,
		result.RunID,
		result.Timestamp.UTC().UnixNano(),
		result.FinishedAt.UTC().UnixNano(),
		result.CaseID,
		result.Profile,
		result.Provider,
		result.Model,
		result.ModelVendor,
		result.InferenceProvider,
		boolToInt(result.Success),
		result.TotalScore,
		result.RawScore,
		result.MaxScore,
		result.NormalizedScore,
		string(metricsJSON),
		string(deductionsJSON),
		string(bonusesJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert sqlite projection row: %w", err)
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) loadProfileProjection(profile string, limit int, filter TimelineFilter) ([]benchmark.Result, error) {
	db, err := openProjectionDB(s.sqlitePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureProjectionSchema(db); err != nil {
		return nil, err
	}
	return queryProjectionRows(db, profile, limit, filter)
}

func (s *Store) loadAllProjection(limit int, filter TimelineFilter) (map[string][]benchmark.Result, error) {
	db, err := openProjectionDB(s.sqlitePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureProjectionSchema(db); err != nil {
		return nil, err
	}

	profiles, err := queryProjectionProfiles(db, filter)
	if err != nil {
		return nil, err
	}
	results := make(map[string][]benchmark.Result, len(profiles))
	for _, profile := range profiles {
		rows, err := queryProjectionRows(db, profile, limit, filter)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		results[profile] = rows
	}
	return results, nil
}

func queryProjectionProfiles(db *sql.DB, filter TimelineFilter) ([]string, error) {
	where, args := buildProjectionWhere("", filter)
	rows, err := db.Query(`SELECT DISTINCT profile FROM timeline_runs`+where+` ORDER BY profile ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite projection profiles: %w", err)
	}
	defer rows.Close()

	profiles := make([]string, 0)
	for rows.Next() {
		var profile string
		if err := rows.Scan(&profile); err != nil {
			return nil, fmt.Errorf("scan sqlite projection profile: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite projection profiles: %w", err)
	}
	return profiles, nil
}

func queryProjectionRows(db *sql.DB, profile string, limit int, filter TimelineFilter) ([]benchmark.Result, error) {
	where, args := buildProjectionWhere(profile, filter)
	query := `
		SELECT
			run_id, timestamp, finished_at, case_id, profile, provider, model,
			model_vendor, inference_provider, success, total_score, raw_score,
			max_score, normalized_score, metrics_json, deductions_json, bonuses_json
		FROM timeline_runs
	` + where + ` ORDER BY finished_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sqlite projection rows: %w", err)
	}
	defer rows.Close()

	results := make([]benchmark.Result, 0)
	for rows.Next() {
		item, err := scanProjectionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite projection rows: %w", err)
	}
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results, nil
}

func buildProjectionWhere(profile string, filter TimelineFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if strings.TrimSpace(profile) != "" {
		clauses = append(clauses, "profile = ?")
		args = append(args, profile)
	}
	if filter.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, filter.Model)
	}
	if filter.ModelVendor != "" {
		clauses = append(clauses, "model_vendor = ?")
		args = append(args, filter.ModelVendor)
	}
	if filter.InferenceProvider != "" {
		clauses = append(clauses, "inference_provider = ?")
		args = append(args, filter.InferenceProvider)
	}
	if filter.CaseID != "" {
		clauses = append(clauses, "case_id = ?")
		args = append(args, filter.CaseID)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanProjectionRow(scanner interface {
	Scan(dest ...any) error
}) (benchmark.Result, error) {
	var (
		runID             string
		timestamp         int64
		finishedAt        int64
		caseID            string
		profile           string
		provider          string
		model             string
		modelVendor       string
		inferenceProvider string
		success           int
		totalScore        int
		rawScore          int
		maxScore          int
		normalizedScore   float64
		metricsJSON       string
		deductionsJSON    string
		bonusesJSON       string
	)

	if err := scanner.Scan(
		&runID,
		&timestamp,
		&finishedAt,
		&caseID,
		&profile,
		&provider,
		&model,
		&modelVendor,
		&inferenceProvider,
		&success,
		&totalScore,
		&rawScore,
		&maxScore,
		&normalizedScore,
		&metricsJSON,
		&deductionsJSON,
		&bonusesJSON,
	); err != nil {
		return benchmark.Result{}, fmt.Errorf("scan sqlite projection row: %w", err)
	}

	result := benchmark.Result{
		RunID:             runID,
		Timestamp:         time.Unix(0, timestamp).UTC(),
		FinishedAt:        time.Unix(0, finishedAt).UTC(),
		CaseID:            caseID,
		Profile:           profile,
		Provider:          provider,
		Model:             model,
		ModelVendor:       modelVendor,
		InferenceProvider: inferenceProvider,
		Success:           success == 1,
		TotalScore:        totalScore,
		RawScore:          rawScore,
		MaxScore:          maxScore,
		NormalizedScore:   normalizedScore,
	}
	if err := json.Unmarshal([]byte(metricsJSON), &result.Metrics); err != nil {
		return benchmark.Result{}, fmt.Errorf("decode metrics_json: %w", err)
	}
	if err := json.Unmarshal([]byte(deductionsJSON), &result.Deductions); err != nil {
		return benchmark.Result{}, fmt.Errorf("decode deductions_json: %w", err)
	}
	if err := json.Unmarshal([]byte(bonusesJSON), &result.Bonuses); err != nil {
		return benchmark.Result{}, fmt.Errorf("decode bonuses_json: %w", err)
	}
	return result, nil
}
