package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Names() ([]string, error) {
	names := map[string]struct{}{}

	if builtin, err := builtinNames(); err != nil {
		return nil, err
	} else {
		for _, name := range builtin {
			if !isAvailableThemeName(name) {
				continue
			}
			names[name] = struct{}{}
		}
	}
	if user, err := userThemeNames(); err == nil {
		for _, name := range user {
			if !isAvailableThemeName(name) {
				continue
			}
			names[name] = struct{}{}
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func builtinNames() ([]string, error) {
	entries, err := fs.ReadDir(builtinThemeFS, "themes")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".yaml"))
	}
	return names, nil
}

func userThemeNames() ([]string, error) {
	dir, err := userThemeDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".yaml"))
	}
	return names, nil
}
