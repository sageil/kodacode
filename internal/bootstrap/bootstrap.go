package bootstrap

import (
	_ "embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

//go:embed default_config.yaml
var defaultConfig []byte

func EnsureDefaults() {
	base := config.ConfigDir()

	for _, dir := range []string{
		base,
		filepath.Join(base, "agents"),
		filepath.Join(base, "themes"),
		filepath.Join(base, "prompts"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("bootstrap: mkdir %s: %v", dir, err)
		}
	}

	cfgPath := filepath.Join(base, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.WriteFile(cfgPath, defaultConfig, 0o644); err != nil {
			log.Printf("bootstrap: write %s: %v", cfgPath, err)
		}
	}

	copyEmbedDir(agent.BuiltinAgentFS(), "agents", filepath.Join(base, "agents"))
	copyEmbedDir(agent.BuiltinPromptFS(), "prompt", filepath.Join(base, "prompts"))
	copyEmbedDir(theme.BuiltinThemeFS(), "themes", filepath.Join(base, "themes"))
}

func copyEmbedDir(fsys fs.FS, root, dest string) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		log.Printf("bootstrap: read embedded %s: %v", root, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target := filepath.Join(dest, e.Name())
		if _, err := os.Stat(target); err == nil {
			continue
		}
		data, err := fs.ReadFile(fsys, root+"/"+e.Name())
		if err != nil {
			log.Printf("bootstrap: read %s/%s: %v", root, e.Name(), err)
			continue
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			log.Printf("bootstrap: write %s: %v", target, err)
		}
	}
}
