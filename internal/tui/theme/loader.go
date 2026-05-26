package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrThemeNotFound = errors.New("theme not found")

var colorPattern = regexp.MustCompile(
	`^(#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})|(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])|` +
		`(black|red|green|yellow|blue|magenta|cyan|white|` +
		`bright-black|bright-red|bright-green|bright-yellow|` +
		`bright-blue|bright-magenta|bright-cyan|bright-white))$`,
)

func Load(name string) (*Theme, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		defaultTheme := StaticDefault()
		return &defaultTheme, nil
	}
	if !isAvailableThemeName(name) {
		return nil, fmt.Errorf("theme: %w: builtin %q", ErrThemeNotFound, name)
	}
	if custom, err := loadUserTheme(name); err == nil {
		return custom, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return loadBuiltin(name)
}

func LoadFile(path string) (*Theme, error) {
	if path == "" {
		defaultTheme := StaticDefault()
		return &defaultTheme, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		defaultTheme := StaticDefault()
		return &defaultTheme, nil
	}
	if err != nil {
		return nil, fmt.Errorf("theme: read %s: %w", path, err)
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) (*Theme, error) {
	var loaded Theme
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("theme: yaml: %w", err)
	}
	if err := validatePalette(loaded.Palette); err != nil {
		return nil, err
	}
	if err := validateTones(loaded.Tones); err != nil {
		return nil, err
	}
	if err := validateComponents(loaded.Components); err != nil {
		return nil, err
	}
	if loaded.Name == "" {
		loaded.Name = "custom"
	}
	return &loaded, nil
}

func loadBuiltin(name string) (*Theme, error) {
	path := filepath.Join("themes", name+".yaml")
	data, err := fs.ReadFile(builtinThemeFS, path)
	if err != nil {
		return nil, fmt.Errorf("theme: %w: builtin %q", ErrThemeNotFound, name)
	}
	return LoadBytes(data)
}

func loadUserTheme(name string) (*Theme, error) {
	dir, err := userThemeDir()
	if err != nil {
		return nil, fmt.Errorf("theme: resolve home: %w", err)
	}
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	loaded, err := LoadBytes(data)
	if err != nil {
		return nil, fmt.Errorf("theme: user theme %q: %w", name, err)
	}
	return loaded, nil
}

func validatePalette(p Palette) error {
	values := map[string]string{
		"primary":   p.Primary,
		"secondary": p.Secondary,
		"surface":   p.Surface,
		"overlay":   p.Overlay,
		"text":      p.Text,
		"subtext":   p.Subtext,
		"error":     p.Error,
		"warning":   p.Warning,
		"success":   p.Success,
		"thinking":  p.Thinking,
	}
	for field, value := range values {
		if err := validateColor("palette."+field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateComponents(components map[string]ComponentStyle) error {
	for name, component := range components {
		for field, value := range map[string]*string{
			"bg":           component.BG,
			"fg":           component.FG,
			"border-color": component.BorderColor,
		} {
			if value == nil {
				continue
			}
			if err := validateColor("components."+name+"."+field, *value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTones(t Tones) error {
	values := map[string]string{
		"tones.bg":          t.BG,
		"tones.bg-alt":      t.BGAlt,
		"tones.panel":       t.Panel,
		"tones.panel-alt":   t.PanelAlt,
		"tones.line":        t.Line,
		"tones.line-strong": t.LineStrong,
		"tones.soft":        t.Soft,
	}
	for field, value := range values {
		if err := validateColor(field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateColor(field, value string) error {
	if value == "" || isPaletteToken(value) {
		return nil
	}
	if !colorPattern.MatchString(value) {
		return fmt.Errorf("theme: field %q has invalid color value %q", field, value)
	}
	return nil
}

func isPaletteToken(value string) bool {
	switch value {
	case "primary", "secondary", "surface", "overlay", "text", "subtext", "error", "warning", "success", "thinking",
		"bg", "bg-alt", "panel", "panel-alt", "line", "line-strong", "soft":
		return true
	default:
		return false
	}
}
