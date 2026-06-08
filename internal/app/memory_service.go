package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
)

const projectMemoryDirName = ".kodacode/memories"
const projectMemoryPromptMaxChars = 4000

var memoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type MemoryService struct{}

func NewMemoryService() *MemoryService {
	return &MemoryService{}
}

type projectMemoryStore struct {
	dir string
}

func (s *MemoryService) Store(workspaceRoot string) *projectMemoryStore {
	root := filepath.Clean(strings.TrimSpace(workspaceRoot))
	if root == "" {
		return nil
	}
	return &projectMemoryStore{dir: filepath.Join(root, projectMemoryDirName)}
}

func (s *MemoryService) PromptFragment(workspaceRoot string) (prompt.Fragment, bool, error) {
	store := s.Store(workspaceRoot)
	if store == nil {
		return prompt.Fragment{}, false, nil
	}
	content, err := store.loadPromptContent(projectMemoryPromptMaxChars)
	if err != nil {
		return prompt.Fragment{}, false, err
	}
	if strings.TrimSpace(content) == "" {
		return prompt.Fragment{}, false, nil
	}
	return prompt.Fragment{
		Kind:      prompt.KindMemory,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "project-memory",
		Key:       "project-memory",
		Label:     "project-memory",
		Content:   content,
	}, true, nil
}

func (s *projectMemoryStore) SaveMemory(input tool.MemorySaveRequest) (tool.MemoryRecord, error) {
	content := strings.TrimSpace(input.Content)
	switch {
	case content == "":
		return tool.MemoryRecord{}, fmt.Errorf("memory content is required")
	case utf8.RuneCountInString(content) > tool.MemoryContentMaxChars:
		return tool.MemoryRecord{}, fmt.Errorf("memory content exceeds %d characters; keep only saved facts and summarize before saving", tool.MemoryContentMaxChars)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return tool.MemoryRecord{}, fmt.Errorf("create memories dir: %w", err)
	}
	now := time.Now().UTC()
	id := now.Format("20060102-150405.000000000")
	path := filepath.Join(s.dir, id+".md")
	relativePath := filepath.ToSlash(filepath.Join(projectMemoryDirName, id+".md"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return tool.MemoryRecord{}, fmt.Errorf("write memory: %w", err)
	}
	return tool.MemoryRecord{
		ID:        id,
		Path:      relativePath,
		Content:   content,
		UpdatedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *projectMemoryStore) ListMemories() ([]tool.MemoryRecord, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	records := make([]tool.MemoryRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read memory %q: %w", entry.Name(), err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat memory %q: %w", entry.Name(), err)
		}
		records = append(records, tool.MemoryRecord{
			ID:        strings.TrimSuffix(entry.Name(), ".md"),
			Path:      filepath.ToSlash(filepath.Join(projectMemoryDirName, entry.Name())),
			Content:   strings.TrimSpace(string(data)),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].UpdatedAt > records[j].UpdatedAt })
	return records, nil
}

func (s *projectMemoryStore) DeleteMemory(input tool.MemoryDeleteRequest) error {
	id, err := normalizeMemoryID(input.ID)
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

func (s *projectMemoryStore) loadPromptContent(maxChars int) (string, error) {
	memories, err := s.ListMemories()
	if err != nil || len(memories) == 0 {
		return "", err
	}
	const header = "Project memory:\n"
	if maxChars <= len(header) {
		return "", nil
	}
	parts := make([]string, 0, len(memories))
	total := len(header)
	for _, memory := range memories {
		entry := fmt.Sprintf("- [%s] %s", memory.ID, memory.Content)
		separator := 0
		if len(parts) > 0 {
			separator = 1
		}
		if total+separator+len(entry) <= maxChars {
			parts = append([]string{entry}, parts...)
			total += separator + len(entry)
			continue
		}
		if len(parts) == 0 {
			truncated := truncateUTF8ToBytes(entry, maxChars-len(header))
			if truncated != "" {
				parts = append(parts, truncated)
			}
		}
		break
	}
	if len(parts) == 0 {
		return "", nil
	}
	return header + strings.Join(parts, "\n"), nil
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

func truncateUTF8ToBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut]
}
