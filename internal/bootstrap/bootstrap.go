package bootstrap

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

//go:embed default_config.yaml
var defaultConfig []byte

//go:embed default_auth.yaml
var defaultAuth []byte

func EnsureDefaults() error {
	configPath := app.NewConfigStore().Path()
	authPath := provider.NewAuthStore().Path()
	configDir := filepath.Dir(configPath)

	for _, dir := range []string{
		configDir,
		filepath.Join(configDir, "agents"),
		filepath.Join(configDir, "prompts"),
		filepath.Join(configDir, "themes"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	if err := ensureFile(configPath, defaultConfig, 0o600); err != nil {
		return err
	}
	if err := ensureFile(authPath, defaultAuth, 0o600); err != nil {
		return err
	}
	if err := copyEmbedDir(agent.BuiltinAgentFS(), ".", filepath.Join(configDir, "agents")); err != nil {
		return err
	}
	if err := copyEmbedDir(provider.BuiltinPromptFS(), "prompts", filepath.Join(configDir, "prompts")); err != nil {
		return err
	}
	if err := copyEmbedDir(theme.BuiltinThemeFS(), "themes", filepath.Join(configDir, "themes")); err != nil {
		return err
	}

	runtimeConfig, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		return err
	}
	return ensureDataArtifacts(runtimeConfig)
}

func ensureFile(path string, data []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func copyEmbedDir(fsys fs.FS, root, dest string) error {
	return fs.WalkDir(fsys, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if _, err := os.Stat(target); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func ensureDataArtifacts(config app.Config) error {
	logger, err := observability.New(config.Logging)
	if err != nil {
		return err
	}
	if logger != nil {
		defer func() {
			_ = logger.Close()
		}()
	}

	store, err := events.NewSQLiteStore(sessionDBPath(config))
	if err != nil {
		return err
	}
	return store.Close()
}

func sessionDBPath(config app.Config) string {
	if path := strings.TrimSpace(config.Sessions.DBPath); path != "" {
		return path
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode", "kodacode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode.db")
	}
	return filepath.Join(home, ".local", "share", "kodacode", "kodacode.db")
}
