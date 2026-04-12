package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Memory struct {
	ID      string
	Content string
	ModTime time.Time
}

type MemoryStore struct {
	dir string
}

var memoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func NewMemoryStore(projectDir string) *MemoryStore {
	return &MemoryStore{dir: filepath.Join(projectDir, ".kodacode", "memories")}
}

func (s *MemoryStore) Dir() string { return s.dir }

func (s *MemoryStore) Save(content string) (Memory, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Memory{}, fmt.Errorf("create memories dir: %w", err)
	}
	id := time.Now().Format("20060102-150405.000000000")
	path := filepath.Join(s.dir, id+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Memory{}, fmt.Errorf("write memory: %w", err)
	}
	return Memory{ID: id, Content: content, ModTime: time.Now()}, nil
}

func (s *MemoryStore) List() ([]Memory, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var memories []Memory
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info, _ := e.Info()
		modTime := time.Time{}
		if info != nil {
			modTime = info.ModTime()
		}
		memories = append(memories, Memory{
			ID:      strings.TrimSuffix(e.Name(), ".md"),
			Content: strings.TrimSpace(string(data)),
			ModTime: modTime,
		})
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].ModTime.After(memories[j].ModTime)
	})
	return memories, nil
}

func (s *MemoryStore) Delete(id string) error {
	id, err := normalizeMemoryID(id)
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, id+".md")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("memory %q not found", id)
		}
		return err
	}
	return nil
}

func (s *MemoryStore) LoadWithBudget(maxChars int) string {
	memories, err := s.List()
	if err != nil || len(memories) == 0 {
		return ""
	}
	const header = "# Project Memories\n\n"
	if maxChars <= len(header) {
		return ""
	}

	// Newest first from List(); include newest memories, drop oldest when over budget.
	var included []string
	total := len(header)
	for _, m := range memories {
		separatorCost := 0
		if len(included) > 0 {
			separatorCost = 1
		}
		if total+separatorCost+len(m.Content) <= maxChars {
			included = append([]string{m.Content}, included...)
			total += separatorCost + len(m.Content)
			continue
		}
		if len(included) == 0 {
			truncated := truncateUTF8ToBytes(m.Content, maxChars-len(header))
			if truncated != "" {
				included = append(included, truncated)
			}
		}
		if len(included) == 0 {
			break
		}
		break
	}
	if len(included) == 0 {
		return ""
	}
	return header + strings.Join(included, "\n")
}

func normalizeMemoryID(id string) (string, error) {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return "", fmt.Errorf("memory id is required")
	case strings.Contains(id, ".."):
		return "", fmt.Errorf("invalid memory id %q", id)
	case strings.ContainsRune(id, '/'), strings.ContainsRune(id, '\\'):
		return "", fmt.Errorf("invalid memory id %q", id)
	case filepath.Base(id) != id:
		return "", fmt.Errorf("invalid memory id %q", id)
	case !memoryIDPattern.MatchString(id):
		return "", fmt.Errorf("invalid memory id %q", id)
	default:
		return id, nil
	}
}

func truncateUTF8ToBytes(s string, maxBytes int) string {
	if maxBytes <= 0 || s == "" {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}
	return s[:cut]
}
