package tool

import (
	"path/filepath"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/syntax"

	"github.com/sageil/kodacode/internal/workspace"
)

func shellArgumentPath(workingDir, literal string) (string, bool) {
	path, ok := shellLiteralPathCandidate(literal)
	if !ok {
		return "", false
	}
	return resolveShellPath(workingDir, path)
}

func resolveShellPath(workingDir, path string) (string, bool) {
	if strings.TrimSpace(workingDir) == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	if isShellPseudoDevicePath(path) {
		return "", false
	}
	scope, err := workspace.New(workingDir)
	if err != nil {
		return "", false
	}
	decision, err := scope.Check(workspace.AccessRead, path)
	if err != nil {
		return "", false
	}
	return decision.ResolvedPath, true
}

func isShellPseudoDevicePath(path string) bool {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	switch normalized {
	case "/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr", "/dev/fd/0", "/dev/fd/1", "/dev/fd/2", "/dev/random", "/dev/urandom", "/dev/zero":
		return true
	default:
		return false
	}
}

func shellLiteralPathCandidate(literal string) (string, bool) {
	literal = strings.TrimSpace(literal)
	switch {
	case literal == "", literal == "-":
		return "", false
	case strings.HasPrefix(literal, "-"):
		index := strings.Index(literal, "=")
		if index <= 0 || index == len(literal)-1 {
			return "", false
		}
		return shellLiteralPathCandidate(literal[index+1:])
	case looksLikeShellRemotePathSpec(literal):
		return "", false
	case looksLikeShellURI(literal):
		return "", false
	case !literalTextMayReferencePath(literal):
		return "", false
	default:
		return shellPathAnchor(literal), true
	}
}

func literalTextMayReferencePath(text string) bool {
	text = strings.TrimSpace(text)
	switch {
	case text == "", text == "-":
		return false
	case text == ".", text == "..", text == "~":
		return true
	case strings.HasPrefix(text, "~/"), strings.HasPrefix(text, "~"+string(filepath.Separator)):
		return true
	case filepath.IsAbs(text):
		return true
	case strings.HasPrefix(text, "."+string(filepath.Separator)), strings.HasPrefix(text, ".."+string(filepath.Separator)):
		return true
	case strings.Contains(text, string(filepath.Separator)):
		return true
	case strings.Contains(text, "/"):
		return true
	default:
		return false
	}
}

func looksLikeShellURI(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.Contains(text, "://") {
		return true
	}
	index := strings.IndexRune(text, ':')
	if index <= 0 || index == len(text)-1 {
		return false
	}
	prefix := text[:index]
	if len(prefix) == 1 {
		next := text[index+1]
		if next == '\\' || next == '/' {
			return false
		}
	}
	for _, r := range prefix {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func looksLikeShellRemotePathSpec(text string) bool {
	text = strings.TrimSpace(text)
	index := strings.IndexRune(text, ':')
	if index <= 0 || index == len(text)-1 {
		return false
	}
	prefix := text[:index]
	if len(prefix) == 1 {
		next := text[index+1]
		if next == '\\' || next == '/' {
			return false
		}
	}
	if slash := strings.IndexAny(text, `/\`); slash >= 0 && slash < index {
		return false
	}
	return strings.Contains(prefix, "@") || strings.Contains(prefix, ".")
}

func shellPathAnchor(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if !literalTextMayReferencePath(text) {
		return text
	}
	normalized := filepath.ToSlash(text)
	prefix := ""
	switch {
	case strings.HasPrefix(normalized, "~/"):
		prefix = "~"
		normalized = strings.TrimPrefix(normalized, "~/")
	case strings.HasPrefix(normalized, "/"):
		prefix = "/"
		normalized = strings.TrimPrefix(normalized, "/")
	}
	parts := strings.Split(normalized, "/")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if part == "..." || strings.ContainsAny(part, "*?[") {
			break
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		switch {
		case prefix == "/":
			return prefix
		case prefix == "~":
			return prefix
		case strings.HasPrefix(text, ".."+string(filepath.Separator)), strings.HasPrefix(text, "../"):
			return ".."
		case strings.HasPrefix(text, "."+string(filepath.Separator)), strings.HasPrefix(text, "./"):
			return "."
		default:
			return "."
		}
	}
	joined := filepath.Join(kept...)
	switch prefix {
	case "/":
		return filepath.Join(prefix, joined)
	case "~":
		if joined == "" {
			return prefix
		}
		return filepath.ToSlash(filepath.Join(prefix, joined))
	default:
		return joined
	}
}

func redirectAccess(redirect *syntax.Redirect) workspace.Access {
	if redirect == nil {
		return workspace.AccessRead
	}
	switch redirect.Op {
	case syntax.RdrIn, syntax.DplIn, syntax.Hdoc, syntax.WordHdoc:
		return workspace.AccessRead
	default:
		return workspace.AccessWrite
	}
}

func dedupePathRequests(requests []PathRequest) []PathRequest {
	if len(requests) == 0 {
		return nil
	}
	out := make([]PathRequest, 0, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		key := string(request.Access) + "\x00" + strings.TrimSpace(request.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, request)
	}
	return out
}
