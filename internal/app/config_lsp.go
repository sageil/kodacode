package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/lsp"
)

type LSPConfig struct {
	AutoDiscover bool
	Servers      []lsp.ServerConfig
}

var errLSPServerNameRequired = errors.New("lsp server name is required")
var errLSPServerCommandRequired = errors.New("lsp server command is required")

func defaultLSPConfig() LSPConfig {
	return LSPConfig{AutoDiscover: true}
}

func (c LSPConfig) Validate() error {
	for _, server := range c.Servers {
		if strings.TrimSpace(server.Name) == "" {
			return errLSPServerNameRequired
		}
		if strings.TrimSpace(server.Command) == "" {
			return errLSPServerCommandRequired
		}
	}
	return nil
}

func resolvedLSPServers(config LSPConfig, workspaceRoot string) []lsp.ServerConfig {
	servers := append([]lsp.ServerConfig(nil), config.Servers...)
	if len(servers) == 0 {
		servers = append(servers, defaultLSPServers()...)
	}
	if config.AutoDiscover {
		servers = appendLSPServers(servers, lsp.DiscoverServers(workspaceRoot))
	}
	return filterEnabledLSPServers(servers)
}

func defaultLSPServers() []lsp.ServerConfig {
	return []lsp.ServerConfig{
		{
			Name:       "gopls",
			Command:    "gopls",
			Env:        map[string]string{"GOFLAGS": "-mod=mod"},
			Extensions: []string{".go"},
		},
		{
			Name:       "vtsls",
			Command:    "vtsls",
			Args:       []string{"--stdio"},
			Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		},
		{
			Name:       "pyright",
			Command:    "pyright-langserver",
			Args:       []string{"--stdio"},
			Extensions: []string{".py", ".pyi"},
		},
	}
}

func appendLSPServers(existing, discovered []lsp.ServerConfig) []lsp.ServerConfig {
	seen := make(map[string]int, len(existing))
	out := append([]lsp.ServerConfig(nil), existing...)
	for index, server := range out {
		seen[strings.TrimSpace(server.Name)] = index
	}
	for _, server := range discovered {
		name := strings.TrimSpace(server.Name)
		if index, ok := seen[name]; ok {
			out[index] = mergeLSPServer(out[index], server)
			continue
		}
		seen[name] = len(out)
		out = append(out, server)
	}
	return out
}

func mergeLSPServer(base, overlay lsp.ServerConfig) lsp.ServerConfig {
	if strings.TrimSpace(base.Command) == "" {
		base.Command = overlay.Command
	}
	if len(base.Args) == 0 {
		base.Args = append([]string(nil), overlay.Args...)
	}
	if len(base.Extensions) == 0 {
		base.Extensions = append([]string(nil), overlay.Extensions...)
	}
	if len(base.Env) == 0 && len(overlay.Env) > 0 {
		base.Env = make(map[string]string, len(overlay.Env))
		for key, value := range overlay.Env {
			base.Env[key] = value
		}
	}
	if len(base.InitOptions) == 0 && len(overlay.InitOptions) > 0 {
		base.InitOptions = make(map[string]any, len(overlay.InitOptions))
		for key, value := range overlay.InitOptions {
			base.InitOptions[key] = value
		}
	}
	return base
}

func filterEnabledLSPServers(servers []lsp.ServerConfig) []lsp.ServerConfig {
	out := make([]lsp.ServerConfig, 0, len(servers))
	for _, server := range servers {
		if !server.IsEnabled() {
			continue
		}
		out = append(out, server)
	}
	return out
}
