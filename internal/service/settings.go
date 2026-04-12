package service

import (
	"context"
	"fmt"

	"github.com/sageil/kodacode/v1/internal/repository"
)

// SettingKeyTheme is the settings key for the persisted TUI theme name.
const SettingKeyTheme = "tui.theme"

// SettingsService provides access to application-wide key/value settings.
type SettingsService struct {
	repo repository.SettingsRepo
}

// NewSettingsService constructs a SettingsService backed by repo.
func NewSettingsService(repo repository.SettingsRepo) *SettingsService {
	return &SettingsService{repo: repo}
}

// GetSetting returns the value for key, or ErrNotFound if not set.
func (s *SettingsService) GetSetting(ctx context.Context, key string) (string, error) {
	v, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}
	return v, nil
}

// SetSetting upserts key with value.
func (s *SettingsService) SetSetting(ctx context.Context, key, value string) error {
	if err := s.repo.Set(ctx, key, value); err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}
