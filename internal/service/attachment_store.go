package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type attachmentStore struct {
	root string
}

func newAttachmentStore(projectDir string) *attachmentStore {
	root := filepath.Join(config.DataDir(), "attachments")
	if projectDir != "" {
		root = filepath.Join(projectDir, ".kodacode", "attachments")
	}
	return &attachmentStore{root: root}
}

func (s *attachmentStore) Store(att FileAttachment) (message.FileContent, error) {
	if s == nil {
		return message.FileContent{
			Path:     att.Path,
			MimeType: att.MimeType,
			Size:     int64(len(att.Data)),
		}, nil
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return message.FileContent{}, fmt.Errorf("mkdir attachments: %w", err)
	}
	sum := sha256.Sum256(att.Data)
	key := hex.EncodeToString(sum[:])
	ext := filepath.Ext(att.Path)
	if ext == "" {
		ext = extensionForMIME(att.MimeType)
	}
	filename := key + ext
	dst := filepath.Join(s.root, filename)
	tmp, err := os.CreateTemp(s.root, filename+".tmp-*")
	if err != nil {
		return message.FileContent{}, fmt.Errorf("create temp attachment blob: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(att.Data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return message.FileContent{}, fmt.Errorf("write attachment blob: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return message.FileContent{}, fmt.Errorf("close attachment blob: %w", err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			_ = os.Remove(tmpPath)
		} else {
			_ = os.Remove(tmpPath)
			return message.FileContent{}, fmt.Errorf("commit attachment blob: %w", err)
		}
	}
	return message.FileContent{
		Path:       att.Path,
		MimeType:   att.MimeType,
		StorageKey: filename,
		Size:       int64(len(att.Data)),
	}, nil
}

func (s *attachmentStore) Delete(storageKeys []string) error {
	if s == nil || len(storageKeys) == 0 {
		return nil
	}
	for _, key := range storageKeys {
		if key == "" {
			continue
		}
		if err := os.Remove(filepath.Join(s.root, key)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove attachment blob %q: %w", key, err)
		}
	}
	return nil
}

func (s *attachmentStore) ListStorageKeys() ([]string, error) {
	if s == nil {
		return nil, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read attachment dir: %w", err)
	}
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		keys = append(keys, entry.Name())
	}
	return keys, nil
}

func buildAttachmentProviderPart(att FileAttachment) provider.FilePart {
	return provider.FilePart{
		Path:     att.Path,
		MimeType: att.MimeType,
		URL:      attachmentPayload(att),
	}
}

func attachmentPayload(att FileAttachment) string {
	switch {
	case strings.HasPrefix(att.MimeType, "image/"), att.MimeType == "application/pdf":
		return "data:" + att.MimeType + ";base64," + base64.StdEncoding.EncodeToString(att.Data)
	case isTextLikeMIME(att.MimeType):
		return string(att.Data)
	default:
		return "data:" + att.MimeType + ";base64," + base64.StdEncoding.EncodeToString(att.Data)
	}
}

func isTextLikeMIME(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml", "application/javascript":
		return true
	default:
		return false
	}
}

func extensionForMIME(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "application/json":
		return ".json"
	default:
		return ""
	}
}
