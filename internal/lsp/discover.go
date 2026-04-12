package lsp

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
)

// projectTool maps a project config file to the LSP server it implies.
type projectTool struct {
	configFiles []string // files that indicate this tool is used
	server      config.LSPServerConfig
}

var knownTools = []projectTool{
	{
		configFiles: []string{"biome.json", "biome.jsonc"},
		server: config.LSPServerConfig{
			Name:       "biome",
			Command:    "biome",
			Args:       []string{"lsp-proxy"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx", ".json", ".css"},
		},
	},
	{
		configFiles: []string{".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs"},
		server: config.LSPServerConfig{
			Name:       "eslint",
			Command:    "vscode-eslint-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
	},
	{
		configFiles: []string{"Cargo.toml"},
		server: config.LSPServerConfig{
			Name:       "rust-analyzer",
			Command:    "rust-analyzer",
			Extensions: []string{".rs"},
		},
	},
	{
		configFiles: []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"},
		server: config.LSPServerConfig{
			Name:       "docker",
			Command:    "docker-langserver",
			Args:       []string{"--stdio"},
			Extensions: []string{},
		},
	},
	{
		configFiles: []string{"tailwind.config.js", "tailwind.config.ts", "tailwind.config.cjs", "tailwind.config.mjs"},
		server: config.LSPServerConfig{
			Name:       "tailwindcss",
			Command:    "tailwindcss-language-server",
			Args:       []string{"--stdio"},
			Extensions: []string{".css", ".html", ".tsx", ".jsx"},
		},
	},
}

// DiscoverServers scans the project root for config files and returns
// LSP server configs for tools that are both used by the project and
// installed on the system.
func DiscoverServers(projectDir string) []config.LSPServerConfig {
	var discovered []config.LSPServerConfig
	for _, tool := range knownTools {
		if !projectUses(projectDir, tool.configFiles) {
			continue
		}
		cmd, err := resolveCommand(tool.server.Command)
		if err != nil {
			continue
		}
		srv := tool.server
		srv.Command = cmd
		discovered = append(discovered, srv)
		log.Printf("lsp: auto-discovered %s (found %s)", srv.Name, cmd)
	}
	return discovered
}

func projectUses(dir string, configFiles []string) bool {
	for _, name := range configFiles {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	// Check subdirectories for nested configs (e.g. .eslintrc in src/).
	for _, name := range configFiles {
		if !strings.HasPrefix(name, ".") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "*", name))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}
