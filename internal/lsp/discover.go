package lsp

import (
	"os"
	"path/filepath"
	"strings"
)

type projectTool struct {
	configFiles []string
	server      ServerConfig
}

var knownTools = []projectTool{
	{
		configFiles: []string{"biome.json", "biome.jsonc"},
		server: ServerConfig{
			Name:       "biome",
			Command:    "biome",
			Args:       []string{"lsp-proxy"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx", ".json", ".css"},
		},
	},
	{
		configFiles: []string{"Cargo.toml"},
		server: ServerConfig{
			Name:       "rust-analyzer",
			Command:    "rust-analyzer",
			Extensions: []string{".rs"},
		},
	},
	{
		configFiles: []string{"tailwind.config.js", "tailwind.config.ts", "tailwind.config.cjs", "tailwind.config.mjs"},
		server: ServerConfig{
			Name:       "tailwindcss",
			Command:    "tailwindcss-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".css", ".html", ".tsx", ".jsx"},
		},
	},
}

func DiscoverServers(projectDir string) []ServerConfig {
	var discovered []ServerConfig
	for _, tool := range knownTools {
		if !projectUses(projectDir, tool.configFiles) {
			continue
		}
		command, err := resolveCommand(tool.server.Command)
		if err != nil {
			continue
		}
		server := tool.server
		server.Command = command
		discovered = append(discovered, server)
	}
	return discovered
}

func projectUses(root string, configFiles []string) bool {
	for _, name := range configFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	for _, name := range configFiles {
		if !strings.HasPrefix(name, ".") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "*", name))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}
