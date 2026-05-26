package tool

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const bashScopedNetworkTargetPrefix = "command: "

var (
	urlLikePattern                = regexp.MustCompile(`https?://[^\s"'<>]+`)
	networkCommandPattern         = regexp.MustCompile(`(?i)(^|[;&|()[:space:]])(curl|wget|scp|ssh|rsync|ping|nc|telnet|ftp|dig|nslookup)([;&|()[:space:]]|$)`)
	packageRegistryCommandPattern = regexp.MustCompile(`(?i)\b(npm|pnpm|yarn|bun)\s+(install|add|update|upgrade|create|dlx|exec|import|ci)\b|\bgo\s+(get|install)\b|\bgo\s+mod\s+(download|tidy|vendor)\b|\bcargo\s+(add|install|search|update|publish)\b|\bcomposer\s+(create-project|install|require|update)\b|\bpoetry\s+(add|install|update)\b|\bpip3?\s+install\b|\buv\s+(add|sync|tool|pip)\b|\bzig\s+fetch\b`)
	gitNetworkCommandPattern      = regexp.MustCompile(`(?i)\bgit\s+(clone|fetch|pull|push|ls-remote)\b`)
)

func assessBashNetworkTargets(command string, prefixRule []string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	if targets := explicitBashNetworkTargets(command); len(targets) > 0 {
		return targets
	}
	return knownBashNetworkTargets(command)
}

func explicitBashNetworkTargets(command string) []string {
	targets := make(map[string]struct{})
	for _, match := range urlLikePattern.FindAllString(command, -1) {
		parsed, err := url.Parse(match)
		if err != nil {
			continue
		}
		host := strings.TrimSpace(parsed.Hostname())
		if host == "" {
			continue
		}
		targets[strings.ToLower(host)] = struct{}{}
	}
	return sortedBashTargets(targets)
}

func knownBashNetworkTargets(command string) []string {
	switch {
	case networkCommandPattern.MatchString(command):
		return []string{"external network"}
	case packageRegistryCommandPattern.MatchString(command):
		return []string{"package registries"}
	case gitNetworkCommandPattern.MatchString(command):
		return []string{"git remotes"}
	default:
		return nil
	}
}

func CommandScopedNetworkTarget(command string, prefixRule []string) string {
	if len(prefixRule) > 0 {
		return bashScopedNetworkTargetPrefix + strings.Join(prefixRule, " ")
	}
	return bashScopedNetworkTargetPrefix + strings.TrimSpace(command)
}

func sortedBashTargets(targets map[string]struct{}) []string {
	if len(targets) == 0 {
		return nil
	}
	out := make([]string, 0, len(targets))
	for target := range targets {
		out = append(out, target)
	}
	slices.Sort(out)
	return out
}
