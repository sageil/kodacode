package service

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/v1/internal/sandbox"
)

func (s *SessionService) permissionSubject(toolName, input string) string {
	if s == nil || s.runtime == nil {
		return ""
	}
	switch toolName {
	case "bash":
		return bashPermissionSubject(input, s.runtime.projectDir)
	case "web_fetch":
		return webFetchPermissionSubject(input)
	default:
		return permissionPathSubject(extractPathFromInput(input), s.runtime.projectDir)
	}
}

func permissionPathSubject(rawPath, projectDir string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return ""
	}

	rootClean := ""
	rootResolved := ""
	if projectDir != "" {
		rootClean = filepath.Clean(projectDir)
		rootResolved = rootClean
		if resolved, err := filepath.EvalSymlinks(projectDir); err == nil {
			rootResolved = resolved
		}
	}

	abs := rawPath
	if !filepath.IsAbs(abs) && rootResolved != "" {
		abs = filepath.Join(rootResolved, abs)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else {
		abs = filepath.Clean(abs)
	}

	for _, root := range []string{rootResolved, rootClean} {
		if root == "" {
			continue
		}
		if rel, err := filepath.Rel(root, abs); err == nil && !pathEscapesBase(rel) {
			return "project:" + filepath.ToSlash(filepath.Clean(rel))
		}
	}

	if filepath.IsAbs(abs) {
		return "external:" + filepath.ToSlash(abs)
	}
	return "relative:" + filepath.ToSlash(filepath.Clean(abs))
}

func bashPermissionSubject(input, projectDir string) string {
	var fields struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if json.Unmarshal([]byte(input), &fields) != nil {
		return ""
	}

	tokens := sandbox.ShellTokens(fields.Command)
	if len(tokens) == 0 {
		return ""
	}

	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(shellQuote(tok))
	}
	if workdir := permissionPathSubject(fields.Workdir, projectDir); workdir != "" {
		b.WriteString(" [workdir=")
		b.WriteString(workdir)
		b.WriteByte(']')
	}
	return b.String()
}

func shellQuote(tok string) string {
	if tok == "" {
		return "''"
	}
	if !strings.ContainsAny(tok, " \t\n'\"\\") {
		return tok
	}
	return "'" + strings.ReplaceAll(tok, "'", `'\''`) + "'"
}

func webFetchPermissionSubject(input string) string {
	var fields struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(input), &fields) != nil || fields.URL == "" {
		return ""
	}
	parsed, err := url.Parse(fields.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func pathEscapesBase(rel string) bool {
	rel = filepath.ToSlash(rel)
	return rel == ".." || strings.HasPrefix(rel, "../")
}
