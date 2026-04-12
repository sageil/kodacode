package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Attachment struct {
	Path     string
	MimeType string
	Name     string
	Size     int64
}

const defaultMaxAttachmentSize int64 = 20 * 1024 * 1024 // 20 MB

func ValidateAttachment(path string, maxSize int64) (Attachment, error) {
	if maxSize <= 0 {
		maxSize = defaultMaxAttachmentSize
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("invalid path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Attachment{}, fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return Attachment{}, fmt.Errorf("cannot attach a directory")
	}
	if info.Size() > maxSize {
		return Attachment{}, fmt.Errorf("file too large (%s, max %s)", formatSize(info.Size()), formatSize(maxSize))
	}
	return Attachment{
		Path:     abs,
		MimeType: detectMimeType(abs),
		Name:     filepath.Base(abs),
		Size:     info.Size(),
	}, nil
}

func detectMimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".txt", ".md", ".go", ".py", ".js", ".ts", ".jsx", ".tsx",
		".cjs", ".mjs", ".rs", ".c", ".cpp", ".cc", ".h", ".hpp",
		".java", ".kt", ".kts", ".rb", ".sh", ".bash", ".zsh",
		".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf",
		".html", ".htm", ".css", ".scss", ".less", ".sass",
		".sql", ".lua", ".zig", ".swift", ".cs", ".fs",
		".r", ".m", ".mm", ".pl", ".pm", ".php",
		".tf", ".hcl", ".nix", ".el", ".clj", ".ex", ".exs",
		".vue", ".svelte", ".astro", ".graphql", ".gql",
		".proto", ".env", ".lock", ".sum", ".mod":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatAttachmentLabel(index int, att Attachment) string {
	prefix := "File"
	if strings.HasPrefix(att.MimeType, "image/") {
		prefix = "Image"
	} else if att.MimeType == "application/pdf" {
		prefix = "pdf"
	}
	return fmt.Sprintf("[%s %s #%d]", prefix, att.Name, index)
}
