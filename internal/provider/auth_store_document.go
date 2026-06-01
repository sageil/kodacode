package provider

import (
	"bufio"
	"bytes"
	"sort"
	"strconv"
	"strings"
)

type authDocument map[string]map[string]string

func parseAuthDocument(data []byte) authDocument {
	document := authDocument{}
	var current string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			current = strings.TrimSuffix(trimmed, ":")
			if _, ok := document[current]; !ok {
				document[current] = map[string]string{}
			}
			continue
		}
		if current == "" || !strings.HasPrefix(line, "  ") {
			continue
		}
		key, value, ok := parseAuthField(strings.TrimSpace(line))
		if !ok {
			continue
		}
		document[current][key] = value
	}

	return document
}

func renderAuthDocument(document authDocument) []byte {
	if len(document) == 0 {
		return nil
	}

	var out strings.Builder
	providers := make([]string, 0, len(document))
	for providerID := range document {
		providers = append(providers, providerID)
	}
	sort.Strings(providers)

	for _, providerID := range providers {
		out.WriteString(providerID)
		out.WriteString(":\n")
		for _, key := range authFieldKeys(document[providerID]) {
			out.WriteString("  ")
			out.WriteString(key)
			out.WriteString(": ")
			out.WriteString(renderScalar(document[providerID][key]))
			out.WriteByte('\n')
		}
	}

	return []byte(out.String())
}

func parseAuthField(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return "", "", false
	}
	if value == "" {
		return key, "", true
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		decoded, err := strconv.Unquote(value)
		if err == nil {
			return key, decoded, true
		}
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return key, strings.ReplaceAll(value[1:len(value)-1], "''", "'"), true
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return key, value, true
}

func authFieldKeys(fields map[string]string) []string {
	if len(fields) == 0 {
		return nil
	}
	keys := []string{"type", "access", "refresh", "expires", "project", "account_id", "client_version"}
	seen := map[string]bool{}
	ordered := make([]string, 0, len(fields))
	for _, key := range keys {
		if _, ok := fields[key]; ok {
			ordered = append(ordered, key)
			seen[key] = true
		}
	}
	var extra []string
	for key := range fields {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return append(ordered, extra...)
}

func renderScalar(value string) string {
	if value == "" {
		return `""`
	}
	return strconv.Quote(value)
}

func decodeAuthEntry(fields map[string]string) (AuthEntry, bool) {
	entryType := AuthType(strings.TrimSpace(fields["type"]))
	if entryType == "" {
		return AuthEntry{}, false
	}
	entry := AuthEntry{
		Type:          entryType,
		Access:        fields["access"],
		Refresh:       fields["refresh"],
		Project:       fields["project"],
		AccountID:     fields["account_id"],
		ClientVersion: fields["client_version"],
	}
	if raw := strings.TrimSpace(fields["expires"]); raw != "" {
		expires, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			entry.Expires = expires
		}
	}
	return entry, true
}

func mergeAuthEntry(existing map[string]string, entry AuthEntry) map[string]string {
	merged := map[string]string{}
	for key, value := range existing {
		merged[key] = value
	}
	setAuthField(merged, "type", string(entry.Type))
	setAuthField(merged, "access", entry.Access)
	setAuthField(merged, "refresh", entry.Refresh)
	setAuthField(merged, "project", entry.Project)
	setAuthField(merged, "account_id", entry.AccountID)
	setAuthField(merged, "client_version", entry.ClientVersion)
	if entry.Expires > 0 {
		merged["expires"] = strconv.FormatInt(entry.Expires, 10)
	} else {
		delete(merged, "expires")
	}
	return merged
}

func setAuthField(fields map[string]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		delete(fields, key)
		return
	}
	fields[key] = value
}
