package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/service"
)

// fakeSettingsRepo is an in-memory SettingsRepo for testing.
type fakeSettingsRepo struct {
	data map[string]string
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{data: make(map[string]string)}
}

func (r *fakeSettingsRepo) Get(_ context.Context, key string) (string, error) {
	v, ok := r.data[key]
	if !ok {
		return "", repository.ErrNotFound
	}
	return v, nil
}

func (r *fakeSettingsRepo) Set(_ context.Context, key, value string) error {
	r.data[key] = value
	return nil
}

func TestSettingsService_SetAndGet(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := service.NewSettingsService(repo)
	ctx := context.Background()

	if err := svc.SetSetting(ctx, service.SettingKeyTheme, "rose-pine"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	got, err := svc.GetSetting(ctx, service.SettingKeyTheme)
	if err != nil {
		t.Fatalf("GetSetting() error = %v", err)
	}
	if got != "rose-pine" {
		t.Errorf("GetSetting() = %q, want %q", got, "rose-pine")
	}
}

func TestSettingsService_Get_NotFound(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := service.NewSettingsService(repo)
	ctx := context.Background()

	_, err := svc.GetSetting(ctx, "missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("GetSetting(missing) error = %v, want ErrNotFound", err)
	}
}
