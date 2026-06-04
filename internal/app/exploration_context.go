package app

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

func providerStepExplorationTargets(toolName, arguments string) ([]string, bool) {
	switch strings.TrimSpace(toolName) {
	case "read":
		var raw struct {
			Path   string   `json:"path"`
			Paths  []string `json:"paths"`
			Offset *int     `json:"offset"`
			Limit  *int     `json:"limit"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		paths := append([]string(nil), raw.Paths...)
		if len(paths) == 0 && strings.TrimSpace(raw.Path) != "" {
			paths = append(paths, raw.Path)
		}
		targets := make([]string, 0, len(paths))
		offset := 0
		if raw.Offset != nil && *raw.Offset > 0 {
			offset = *raw.Offset
		}
		limit := tool.DefaultReadLimit
		if raw.Limit != nil && *raw.Limit > 0 {
			limit = *raw.Limit
		}
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			targets = append(targets, "read:path="+path+
				":offset="+intFingerprint(offset)+
				":limit="+intFingerprint(limit))
		}
		return uniqueSortedStrings(targets), len(targets) > 0
	case "locate":
		var raw struct {
			Path          string `json:"path"`
			Query         string `json:"query"`
			IncludeHidden *bool  `json:"include_hidden"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		path := strings.TrimSpace(raw.Path)
		query := strings.TrimSpace(raw.Query)
		if path == "" || query == "" {
			return nil, false
		}
		return []string{
			"locate:path=" + path +
				":query=" + query +
				":include_hidden=" + boolFingerprint(boolPointerValue(raw.IncludeHidden)),
		}, true
	case "search":
		var raw struct {
			Path          string `json:"path"`
			Query         string `json:"query"`
			Glob          string `json:"glob"`
			Mode          string `json:"mode"`
			Regex         bool   `json:"regex"`
			CaseSensitive bool   `json:"case_sensitive"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		path := strings.TrimSpace(raw.Path)
		query := strings.TrimSpace(raw.Query)
		if path == "" || query == "" {
			return nil, false
		}
		mode := strings.TrimSpace(raw.Mode)
		if mode == "" {
			mode = "auto"
		}
		glob := strings.TrimSpace(raw.Glob)
		return []string{
			"search:path=" + path +
				":query=" + query +
				":mode=" + mode +
				":glob=" + glob +
				":regex=" + boolFingerprint(raw.Regex) +
				":case_sensitive=" + boolFingerprint(raw.CaseSensitive),
		}, true
	default:
		return nil, false
	}
}
