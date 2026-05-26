package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

type configFileSnapshot struct {
	configPath   string
	configData   []byte
	configExists bool
	authPath     string
	authData     []byte
	authExists   bool
}

func snapshotConfigFiles(configPath, authPath string) (configFileSnapshot, error) {
	configData, configExists, err := readOptionalFile(configPath)
	if err != nil {
		return configFileSnapshot{}, err
	}
	authData, authExists, err := readOptionalFile(authPath)
	if err != nil {
		return configFileSnapshot{}, err
	}
	return configFileSnapshot{
		configPath:   configPath,
		configData:   configData,
		configExists: configExists,
		authPath:     authPath,
		authData:     authData,
		authExists:   authExists,
	}, nil
}

func stagedConfigStores(snapshot configFileSnapshot) (*app.ConfigStore, *provider.AuthStore, func(), error) {
	dir, err := os.MkdirTemp("", "kodacode-config-*")
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	configStore := app.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	authStore := provider.NewAuthStoreAt(filepath.Join(dir, "auth.yaml"))
	if err := writeOptionalFile(configStore.Path(), snapshot.configData, snapshot.configExists); err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	if err := writeOptionalFile(authStore.Path(), snapshot.authData, snapshot.authExists); err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	return configStore, authStore, cleanup, nil
}

func installStagedConfig(stagedConfigPath, configPath, stagedAuthPath, authPath string) error {
	if err := copyOptionalFile(stagedConfigPath, configPath); err != nil {
		return err
	}
	if err := copyOptionalFile(stagedAuthPath, authPath); err != nil {
		return err
	}
	return nil
}

func restoreConfigFiles(snapshot configFileSnapshot) error {
	configErr := writeOptionalFile(snapshot.configPath, snapshot.configData, snapshot.configExists)
	authErr := writeOptionalFile(snapshot.authPath, snapshot.authData, snapshot.authExists)
	return errors.Join(configErr, authErr)
}

func copyOptionalFile(src, dst string) error {
	data, exists, err := readOptionalFile(src)
	if err != nil {
		return err
	}
	return writeOptionalFile(dst, data, exists)
}

func readOptionalFile(path string) ([]byte, bool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func writeOptionalFile(path string, data []byte, exists bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
