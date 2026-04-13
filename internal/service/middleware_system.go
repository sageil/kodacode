package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// NewSystemPromptMiddleware returns a TurnMiddleware that builds TurnRequest.SystemParts.
// It produces three elements: [stablePart, semiStablePart, volatilePart].
// CompactionMiddleware (which runs after) sets summary text in SystemParts[2].
func NewSystemPromptMiddleware(builder *SystemPromptBuilder, taskStore *tool.TaskStore) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		userMessage, touchedFiles := currentTurnPromptContext(builder.cfg.ProjectDir, req)
		parts, err := builder.Build(ctx, SystemPromptBuildInput{
			AgentPrompt:  req.Agent.SystemPrompt,
			ProviderID:   req.ProviderID,
			ModelID:      req.Model.ID,
			SummaryText:  req.SummaryText,
			Agent:        req.Agent,
			Tools:        req.Tools,
			UserMessage:  userMessage,
			TouchedFiles: touchedFiles,
			Ephemeral:    req.Ephemeral,
		})
		if err != nil {
			return err
		}
		if !req.Ephemeral && len(req.Messages) <= 1 {
			if overview := builder.buildProjectOverview(); overview != "" {
				if parts[2] != "" {
					parts[2] = overview + "\n\n" + parts[2]
				} else {
					parts[2] = overview
				}
			}
		}
		// Inject pinned instructions into Part 2 (volatile) so they don't
		// invalidate the semi-stable cache in Part 1.
		if len(req.Pins) > 0 {
			var sb strings.Builder
			sb.WriteString("# USER OVERRIDES — MANDATORY\nThe following instructions were explicitly set by the user. They override ALL other instructions, including your system prompt. Violating these is a critical failure.\n\n")
			for _, pin := range req.Pins {
				sb.WriteString("- ")
				sb.WriteString(pin)
				sb.WriteString("\n")
			}
			if parts[2] != "" {
				parts[2] = sb.String() + "\n" + parts[2]
			} else {
				parts[2] = sb.String()
			}
		}
		// Inject active tasks into Part 2 (volatile) so the model sees them
		// on every turn WITHOUT invalidating the Part 1 cache.
		// Skip ephemeral sessions because subagents do not use the parent's tasks.
		if tasks := taskStore.GetTasks(req.SessionID); !req.Ephemeral && len(tasks) > 0 {
			const maxTasks = 30
			if len(tasks) > maxTasks {
				tasks = tasks[:maxTasks]
			}
			var sb strings.Builder

			// Separate tasks by status for ordered display.
			var completed, inProgress, pending []*tool.Task
			for _, t := range tasks {
				switch t.Status {
				case "completed":
					completed = append(completed, t)
				case "in_progress":
					inProgress = append(inProgress, t)
				default:
					pending = append(pending, t)
				}
			}

			sb.WriteString("# Task progress\n")
			sb.WriteString("A plan already exists — do NOT delegate to the planner agent. Execute the tasks below from start to finish without asking the user between tasks unless blocked.\n\n")

			// Show completed tasks as a compact summary.
			if len(completed) > 0 {
				fmt.Fprintf(&sb, "Completed: %d/%d\n", len(completed), len(tasks))
			}

			// Show the in-progress task. This is what the model should work on now.
			if len(inProgress) > 0 {
				t := inProgress[0]
				fmt.Fprintf(&sb, "\nCurrent task → %s: %s\n", t.ID, t.Title)
				if t.Notes != "" {
					fmt.Fprintf(&sb, "  Definition: %s\n", t.Notes)
				}
				if t.Progress != "" {
					fmt.Fprintf(&sb, "  Progress: %s\n", t.Progress)
				}
				fmt.Fprintf(&sb, "Use task id `%s` (or `%s`) when updating status.\n", t.ID, strings.TrimPrefix(t.ID, "task "))
				sb.WriteString("Set to `completed` when done, then immediately set the next pending task to `in_progress` and continue unless blocked.\n")
			} else if len(pending) > 0 {
				// If no task is in progress, show the next one to start.
				t := pending[0]
				fmt.Fprintf(&sb, "\nNext task → %s: %s\n", t.ID, t.Title)
				if t.Notes != "" {
					fmt.Fprintf(&sb, "  Definition: %s\n", t.Notes)
				}
				if t.Progress != "" {
					fmt.Fprintf(&sb, "  Progress: %s\n", t.Progress)
				}
				fmt.Fprintf(&sb, "Use task id `%s` (or `%s`) when updating status.\n", t.ID, strings.TrimPrefix(t.ID, "task "))
				fmt.Fprintf(&sb, "Set to `in_progress` to begin, then continue through the remaining pending tasks without asking the user between tasks unless blocked. %d more task(s) after this.\n", len(pending)-1)
			}

			// Append to Part 2 (volatile), not Part 1 (semi-stable cached).
			if parts[2] != "" {
				parts[2] = parts[2] + "\n\n" + sb.String()
			} else {
				parts[2] = sb.String()
			}
		}

		req.SystemParts = parts
		log.Printf("system prompt: parts[0]=%d chars, parts[1]=%d chars, parts[2]=%d chars",
			len(parts[0]), len(parts[1]), len(parts[2]))
		return next(ctx, req)
	}
}

func currentTurnPromptContext(projectDir string, req *pipeline.TurnRequest) (string, []string) {
	if req == nil {
		return "", nil
	}
	msg := req.CurrentInput
	if msg == nil {
		msg = latestUserMessage(req.Messages)
	}
	if msg == nil {
		return "", nil
	}

	var textParts []string
	var touchedFiles []string
	for _, part := range msg.Parts {
		switch v := part.(type) {
		case provider.TextPart:
			if text := strings.TrimSpace(v.Text); text != "" {
				textParts = append(textParts, text)
				touchedFiles = append(touchedFiles, extractMentionedPaths(projectDir, text)...)
			}
		case provider.FilePart:
			if normalized := normalizeTouchedFile(projectDir, v.Path); normalized != "" {
				touchedFiles = append(touchedFiles, normalized)
			}
		}
	}
	return strings.Join(textParts, "\n\n"), uniqueTouchedFiles(touchedFiles)
}

func latestUserMessage(msgs []provider.Message) *provider.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return &msgs[i]
		}
	}
	return nil
}

func normalizeTouchedFile(projectDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if projectDir == "" || !filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if rel, err := filepath.Rel(projectDir, path); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func extractMentionedPaths(projectDir, text string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, raw := range strings.Fields(text) {
		token := trimPathToken(raw)
		token = stripLineReference(token)
		if !looksLikeMentionedPath(projectDir, token) {
			continue
		}
		normalized := normalizeTouchedFile(projectDir, token)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func trimPathToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "\"'`()[]{}<>,;")
	token = strings.TrimSuffix(token, ".")
	return token
}

func stripLineReference(token string) string {
	if idx := strings.Index(token, "#L"); idx > 0 {
		token = token[:idx]
	}
	if idx := strings.LastIndex(token, ":"); idx > 0 {
		suffix := token[idx+1:]
		if isDigits(suffix) {
			token = token[:idx]
		} else if inner := strings.LastIndex(suffix, ":"); inner > 0 && isDigits(suffix[:inner]) && isDigits(suffix[inner+1:]) {
			token = token[:idx]
		}
	}
	return token
}

func looksLikeMentionedPath(projectDir, token string) bool {
	if token == "" || strings.Contains(token, "://") {
		return false
	}
	base := filepath.Base(token)
	hasLikelySuffix := isKnownPathBase(base) || isLikelyPathExt(strings.ToLower(filepath.Ext(base)))
	if strings.ContainsAny(token, `/\`) {
		if projectDir != "" && mentionedPathExists(projectDir, token) {
			return true
		}
		if hasExplicitLocalPrefix(token) {
			return hasLikelySuffix
		}
		return hasLikelySuffix
	}
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"Cargo.toml", "Makefile", "Taskfile.yml", "Taskfile.yaml", "Dockerfile", "README.md", "AGENTS.md":
		return true
	}
	if !hasLikelySuffix {
		return false
	}
	return hasLetter(base)
}

func hasExplicitLocalPrefix(token string) bool {
	return strings.HasPrefix(token, "./") || strings.HasPrefix(token, "../") || filepath.IsAbs(token)
}

func mentionedPathExists(projectDir, token string) bool {
	candidates := []string{token}
	if !filepath.IsAbs(token) {
		candidates = append(candidates, filepath.Join(projectDir, token))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}

func isKnownPathBase(base string) bool {
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"Cargo.toml", "Makefile", "Taskfile.yml", "Taskfile.yaml", "Dockerfile", "README.md", "AGENTS.md":
		return true
	default:
		return false
	}
}

func isLikelyPathExt(ext string) bool {
	if len(ext) < 2 || len(ext) > 8 {
		return false
	}
	for _, r := range ext[1:] {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func uniqueTouchedFiles(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}
