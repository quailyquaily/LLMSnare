package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"llmsnare/internal/benchmark"

	"github.com/google/uuid"
)

const projectionFilename = "timeline.sqlite3"

type TimelineFilter struct {
	ModelVendor       string
	InferenceProvider string
}

type Store struct {
	dir             string
	sqlitePath      string
	dirtyMarkerPath string
	readyMarkerPath string
}

func New(dir string) *Store {
	sqlitePath := filepath.Join(dir, projectionFilename)
	return &Store{
		dir:             dir,
		sqlitePath:      sqlitePath,
		dirtyMarkerPath: sqlitePath + ".dirty",
		readyMarkerPath: sqlitePath + ".ready",
	}
}

func (s *Store) EnsureDir() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create timeline dir: %w", err)
	}
	return nil
}

func (s *Store) Append(result *benchmark.Result) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("timeline result is required")
	}
	if err := ensureRunID(result); err != nil {
		return err
	}

	path := filepath.Join(s.dir, result.Profile+".jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open timeline file: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal timeline result: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append timeline result: %w", err)
	}
	if err := s.upsertProjection(*result); err != nil {
		_ = s.markProjectionDirty(err)
		return err
	}
	return nil
}

func (s *Store) ProjectionPath() string {
	return s.sqlitePath
}

func ensureRunID(result *benchmark.Result) error {
	if strings.TrimSpace(result.RunID) != "" {
		return nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate run_id: %w", err)
	}
	result.RunID = id.String()
	return nil
}

func (s *Store) LoadProfile(profile string, limit int, filter TimelineFilter) ([]benchmark.Result, error) {
	filter = normalizeFilter(filter)
	if s.projectionReady() {
		results, err := s.loadProfileProjection(profile, limit, filter)
		if err == nil {
			return results, nil
		}
		_ = s.markProjectionDirty(err)
	}
	path := filepath.Join(s.dir, profile+".jsonl")
	return loadJSONL(path, limit, filter)
}

func (s *Store) ProfileVersion(profile string) (string, error) {
	if s.projectionReady() {
		return s.projectionVersion()
	}
	info, err := os.Stat(filepath.Join(s.dir, profile+".jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil
		}
		return "", fmt.Errorf("stat timeline file: %w", err)
	}
	return fileVersion(info), nil
}

func (s *Store) LoadAll(limit int, filter TimelineFilter) (map[string][]benchmark.Result, error) {
	filter = normalizeFilter(filter)
	if s.projectionReady() {
		results, err := s.loadAllProjection(limit, filter)
		if err == nil {
			return results, nil
		}
		_ = s.markProjectionDirty(err)
	}

	names, err := s.walProfileNames()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]benchmark.Result, len(names))
	for _, profile := range names {
		loaded, err := loadJSONL(filepath.Join(s.dir, profile+".jsonl"), limit, filter)
		if err != nil {
			return nil, err
		}
		if len(loaded) == 0 {
			continue
		}
		result[profile] = loaded
	}
	return result, nil
}

func (s *Store) AllVersion() (string, error) {
	if s.projectionReady() {
		return s.projectionVersion()
	}
	return s.walVersion()
}

func (s *Store) walVersion() (string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil
		}
		return "", fmt.Errorf("read timeline dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	meta := make(map[string]os.FileInfo, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", fmt.Errorf("stat timeline entry: %w", err)
		}
		names = append(names, entry.Name())
		meta[entry.Name()] = info
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		if b.Len() > 0 {
			b.WriteByte('|')
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(fileVersion(meta[name]))
	}
	if b.Len() == 0 {
		return "empty", nil
	}
	return b.String(), nil
}

func loadJSONL(path string, limit int, filter TimelineFilter) ([]benchmark.Result, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []benchmark.Result{}, nil
		}
		return nil, fmt.Errorf("open timeline file: %w", err)
	}
	defer file.Close()

	results := make([]benchmark.Result, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		var item benchmark.Result
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("decode timeline line: %w", err)
		}
		if !matchesFilter(item, filter) {
			continue
		}
		results = append(results, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan timeline file: %w", err)
	}
	if limit > 0 && len(results) > limit {
		results = results[len(results)-limit:]
	}
	return results, nil
}

func fileVersion(info os.FileInfo) string {
	return strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
}
