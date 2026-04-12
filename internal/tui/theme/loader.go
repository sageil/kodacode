package theme

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// colorRe matches hex (#rgb, #rrggbb), ANSI-256 integers (0-255), and named ANSI colors.
var colorRe = regexp.MustCompile(
	`^(#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})|(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])|` +
		`(black|red|green|yellow|blue|magenta|cyan|white|` +
		`bright-black|bright-red|bright-green|bright-yellow|` +
		`bright-blue|bright-magenta|bright-cyan|bright-white))$`,
)

// isPaletteToken returns true if s is a palette reference rather than a raw color value.
func isPaletteToken(s string) bool {
	switch s {
	case "primary", "secondary", "surface", "overlay",
		"text", "subtext", "error", "warning", "success", "thinking":
		return true
	}
	return false
}

func validateColor(field, val string) error {
	if val == "" || isPaletteToken(val) {
		return nil
	}
	if !colorRe.MatchString(val) {
		return fmt.Errorf("theme: field %q has invalid color value %q", field, val)
	}
	return nil
}

func validatePalette(p Palette) error {
	fields := map[string]string{
		"primary":   p.Primary,
		"secondary": p.Secondary,
		"surface":   p.Surface,
		"overlay":   p.Overlay,
		"text":      p.Text,
		"subtext":   p.Subtext,
		"error":     p.Error,
		"warning":   p.Warning,
		"success":   p.Success,
	}
	for name, val := range fields {
		if err := validateColor("palette."+name, val); err != nil {
			return err
		}
	}
	return nil
}

func validateComponents(components map[string]ComponentStyle) error {
	for compName, cs := range components {
		colorFields := map[string]*string{
			"bg":           cs.BG,
			"fg":           cs.FG,
			"border-color": cs.BorderColor,
			"selected-bg":  cs.SelectedBG,
			"selected-fg":  cs.SelectedFG,
			"title-fg":     cs.TitleFG,
		}
		for field, val := range colorFields {
			if val == nil {
				continue
			}
			if err := validateColor("components."+compName+"."+field, *val); err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadBytes parses and validates a theme from raw YAML bytes.
func LoadBytes(data []byte) (*Theme, error) {
	var th Theme
	if err := yaml.Unmarshal(data, &th); err != nil {
		return nil, fmt.Errorf("theme: yaml: %w", err)
	}
	if err := validatePalette(th.Palette); err != nil {
		return nil, err
	}
	if err := validateComponents(th.Components); err != nil {
		return nil, err
	}
	return &th, nil
}

// LoaderConfig configures the Loader.
type LoaderConfig struct {
	// Path is the file path to load. Empty string means use the fallback.
	Path string
}

// Loader loads a Theme from a file, falling back to StaticDefault on missing file.
type Loader struct {
	cfg LoaderConfig
}

// NewLoader constructs a Loader.
func NewLoader(cfg LoaderConfig) *Loader {
	return &Loader{cfg: cfg}
}

// Load reads the theme file. Falls back to StaticDefault() on missing file.
// Returns an error only for corrupt or invalid files.
func (l *Loader) Load() (*Theme, error) {
	if l.cfg.Path == "" {
		d := StaticDefault()
		return &d, nil
	}
	data, err := os.ReadFile(l.cfg.Path)
	if errors.Is(err, os.ErrNotExist) {
		d := StaticDefault()
		return &d, nil
	}
	if err != nil {
		return nil, fmt.Errorf("theme: read %s: %w", l.cfg.Path, err)
	}
	return LoadBytes(data)
}
