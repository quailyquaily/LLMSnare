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
)

type Store struct {
	dir string
}

func New(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) EnsureDir() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create timeline dir: %w", err)
	}
	return nil
}

func (s *Store) Append(result benchmark.Result) error {
	if err := s.EnsureDir(); err != nil {
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
	return nil
}

func (s *Store) LoadProfile(profile string, limit int) ([]benchmark.Result, error) {
	path := filepath.Join(s.dir, profile+".jsonl")
	return loadJSONL(path, limit)
}

func (s *Store) ProfileVersion(profile string) (string, error) {
	info, err := os.Stat(filepath.Join(s.dir, profile+".jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", nil
		}
		return "", fmt.Errorf("stat timeline file: %w", err)
	}
	return fileVersion(info), nil
}

func (s *Store) LoadAll(limit int) (map[string][]benchmark.Result, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]benchmark.Result{}, nil
		}
		return nil, fmt.Errorf("read timeline dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	result := make(map[string][]benchmark.Result, len(names))
	for _, name := range names {
		profile := strings.TrimSuffix(name, ".jsonl")
		loaded, err := loadJSONL(filepath.Join(s.dir, name), limit)
		if err != nil {
			return nil, err
		}
		result[profile] = loaded
	}
	return result, nil
}

func (s *Store) AllVersion() (string, error) {
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

func loadJSONL(path string, limit int) ([]benchmark.Result, error) {
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
	for scanner.Scan() {
		line := scanner.Bytes()
		var item benchmark.Result
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("decode timeline line: %w", err)
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
