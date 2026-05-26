package app

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

func PromptStartupTrust(reader *bufio.Reader, out io.Writer, state StartupTrustState) (ResolveStartupTrustInput, bool, error) {
	result := ResolveStartupTrustInput{
		WorkspaceRoot:   state.WorkspaceRoot,
		ServerDecisions: make(map[string]bool, len(state.Servers)),
	}
	if !state.Pending() {
		return result, true, nil
	}

	lines := []string{
		"Startup trust required:",
	}
	if strings.TrimSpace(state.WorkspaceRoot) != "" {
		lines = append(lines, "workspace: "+state.WorkspaceRoot)
	}
	if len(state.Servers) > 0 {
		lines = append(lines, "mcp servers:")
		for _, server := range state.Servers {
			entry := fmt.Sprintf("- %s (%s) %s", server.Name, server.Type, server.Command)
			if len(server.Args) > 0 {
				entry += " " + strings.Join(server.Args, " ")
			}
			lines = append(lines, entry)
			if len(server.EnvKeys) > 0 {
				lines = append(lines, "  env keys: "+strings.Join(server.EnvKeys, ", "))
			}
		}
	}

	options := startupTrustOptions(state)
	for idx, option := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, option.label))
	}
	lines = append(lines, fmt.Sprintf("Choice [%d]: ", len(options)))
	if _, err := fmt.Fprint(out, strings.Join(lines, "\n")); err != nil {
		return ResolveStartupTrustInput{}, false, err
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return ResolveStartupTrustInput{}, false, err
	}
	choice := strings.TrimSpace(line)
	selected := options[len(options)-1]
	if choice != "" {
		if idx := startupTrustChoiceIndex(choice, len(options)); idx >= 0 {
			selected = options[idx]
		} else {
			return ResolveStartupTrustInput{}, false, fmt.Errorf("invalid choice %q", choice)
		}
	}
	if !selected.proceed {
		return ResolveStartupTrustInput{}, false, nil
	}
	result.TrustWorkspace = selected.trustWorkspace
	for _, server := range state.Servers {
		result.ServerDecisions[server.Fingerprint] = selected.trustServers
	}
	return result, selected.proceed, nil
}

type startupTrustOption struct {
	label          string
	proceed        bool
	trustWorkspace bool
	trustServers   bool
}

func startupTrustOptions(state StartupTrustState) []startupTrustOption {
	switch {
	case state.WorkspaceRequired && len(state.Servers) > 0:
		return []startupTrustOption{
			{label: "Trust workspace and all listed MCP servers", proceed: true, trustWorkspace: true, trustServers: true},
			{label: "Trust workspace only", proceed: true, trustWorkspace: true, trustServers: false},
			{label: "Cancel", proceed: false},
		}
	case state.WorkspaceRequired:
		return []startupTrustOption{
			{label: "Trust workspace", proceed: true, trustWorkspace: true},
			{label: "Cancel", proceed: false},
		}
	default:
		return []startupTrustOption{
			{label: "Trust all listed MCP servers", proceed: true, trustServers: true},
			{label: "Continue without trusting listed MCP servers", proceed: true, trustServers: false},
		}
	}
}

func startupTrustChoiceIndex(raw string, count int) int {
	switch strings.TrimSpace(raw) {
	case "1":
		return 0
	case "2":
		if count >= 2 {
			return 1
		}
	case "3":
		if count >= 3 {
			return 2
		}
	}
	return -1
}

func SortedStartupTrustServerNames(state StartupTrustState) []string {
	names := make([]string, 0, len(state.Servers))
	for _, server := range state.Servers {
		names = append(names, server.Name)
	}
	sort.Strings(names)
	return names
}
