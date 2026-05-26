package app

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

const maxImageAttachmentBytes int64 = 8 * 1024 * 1024

type AttachmentInput struct {
	Path string
}

func (a AttachmentInput) Validate() error {
	if strings.TrimSpace(a.Path) == "" {
		return ErrAttachmentPathRequired
	}
	return nil
}

func (r *Runtime) resolveTurnAttachments(workspaceRoot string, route provider.ModelRoute, attachments []AttachmentInput) ([]provider.Attachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	if unsupported, known := r.imageAttachmentsUnsupported(route.Primary); known && unsupported {
		return nil, fmt.Errorf("model %q does not support image attachments", route.Primary.String())
	}
	resolved := make([]provider.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		next, err := resolveImageAttachment(workspaceRoot, attachment)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, next)
	}
	return resolved, nil
}

func (r *Runtime) imageAttachmentsUnsupported(ref provider.ModelRef) (bool, bool) {
	if r == nil || r.ModelCatalog == nil {
		return false, false
	}
	model, ok := r.lookupCatalogModel(ref)
	if !ok || !model.VisionKnown {
		return false, false
	}
	return !model.Vision, true
}

func (r *Runtime) lookupCatalogModel(ref provider.ModelRef) (provider.CatalogModel, bool) {
	if r == nil || r.ModelCatalog == nil {
		return provider.CatalogModel{}, false
	}
	providerID := strings.TrimSpace(ref.ProviderID)
	modelID := strings.TrimSpace(ref.ModelID)
	if providerID == "" || modelID == "" {
		return provider.CatalogModel{}, false
	}
	for _, model := range r.ModelCatalog.ModelsForProvider(providerID) {
		if strings.TrimSpace(model.ID) == modelID {
			return provider.NormalizeCatalogModelCapabilities(providerID, model), true
		}
	}
	return provider.CatalogModel{}, false
}

func resolveImageAttachment(workspaceRoot string, input AttachmentInput) (provider.Attachment, error) {
	if err := input.Validate(); err != nil {
		return provider.Attachment{}, err
	}
	path, err := resolveAttachmentPath(workspaceRoot, input.Path)
	if err != nil {
		return provider.Attachment{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return provider.Attachment{}, fmt.Errorf("attachment %q: %w", filepath.Base(path), err)
	}
	if info.IsDir() {
		return provider.Attachment{}, fmt.Errorf("attachment %q is a directory", filepath.Base(path))
	}
	if info.Size() > maxImageAttachmentBytes {
		return provider.Attachment{}, fmt.Errorf("attachment %q exceeds the %d MB limit", filepath.Base(path), maxImageAttachmentBytes/(1024*1024))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return provider.Attachment{}, fmt.Errorf("attachment %q: read failed: %w", filepath.Base(path), err)
	}
	mimeType := detectImageAttachmentMIME(path, data)
	if !supportedImageAttachmentMIME(mimeType) {
		return provider.Attachment{}, fmt.Errorf("attachment %q must be a PNG, JPEG, GIF, or WebP image", filepath.Base(path))
	}
	return provider.Attachment{
		Name:     filepath.Base(path),
		MIMEType: mimeType,
		DataURL:  "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
	}, nil
}

func resolveAttachmentPath(workspaceRoot, raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if len(path) >= 2 {
		if (path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'') {
			path = path[1 : len(path)-1]
		}
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(workspaceRoot) != "" {
		path = filepath.Join(workspaceRoot, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid attachment path %q: %w", raw, err)
	}
	return abs, nil
}

func detectImageAttachmentMIME(path string, data []byte) string {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return "image/svg+xml"
	}
	header := data
	if len(header) > 512 {
		header = header[:512]
	}
	return http.DetectContentType(header)
}

func supportedImageAttachmentMIME(mimeType string) bool {
	switch strings.TrimSpace(mimeType) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}
