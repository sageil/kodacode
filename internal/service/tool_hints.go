package service

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/sageil/kodacode/v1/internal/provider"
)

type toolPromptMeta struct {
	Summary               string
	Triggers              []string
	FileExts              []string
	Guidance              string
	PreserveParameterDocs bool
}

var toolPromptMetadata = map[string]toolPromptMeta{
	"search":        {Summary: "Broad project/code search", Triggers: []string{"search codebase", "find implementation", "explore architecture", "look up concept"}, Guidance: "Use first for broad intent or codebase discovery."},
	"read_files":    {Summary: "Read several files together", Triggers: []string{"read multiple files", "inspect search results", "batch file reads"}, Guidance: "Prefer after search when you already have a file list."},
	"read":          {Summary: "Read one file or a specific range", Triggers: []string{"read file", "inspect section", "open code snippet"}, Guidance: "Use for a targeted file or line range."},
	"grep":          {Summary: "Exact string or regex search", Triggers: []string{"grep symbol", "exact text match", "regex search"}, Guidance: "Use when you know the exact string or pattern."},
	"glob":          {Summary: "Find files by path pattern", Triggers: []string{"find files", "match paths", "list files by pattern"}, Guidance: "Use for filename and path discovery."},
	"lsp":           {Summary: "Symbol lookup, references, diagnostics", Triggers: []string{"find references", "go to definition", "list symbols", "check diagnostics"}, FileExts: []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".swift", ".rb", ".php", ".cs"}, Guidance: "Best for symbols, callers, definitions, and diagnostics."},
	"code_action":   {Summary: "LSP quick fixes and refactors", Triggers: []string{"organize imports", "apply quick fix", "run code action", "structured refactor"}, FileExts: []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".swift", ".rb", ".php", ".cs"}, Guidance: "Prefer when the language server offers a structured quick fix or refactor for a source range."},
	"rename_symbol": {Summary: "LSP-backed symbol rename across references", Triggers: []string{"rename symbol", "rename function", "rename variable", "rename type safely"}, FileExts: []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".swift", ".rb", ".php", ".cs"}, Guidance: "Prefer over text edits when renaming code symbols tracked by the language server."},
	"tree":          {Summary: "Directory structure overview", Triggers: []string{"project structure", "directory layout", "overview of files"}, Guidance: "Use for local structure, not full-text search."},
	"edit":          {Summary: "Precise in-place file edits", Triggers: []string{"edit code", "modify file", "small fix", "surgical change"}, Guidance: "Prefer for targeted changes after reading the code."},
	"patch":         {Summary: "Coordinated diff-style edits", Triggers: []string{"apply patch", "multi-file edit", "structured code changes"}, Guidance: "Use for structured multi-hunk edits."},
	"write":         {Summary: "Create or fully replace a file", Triggers: []string{"create file", "rewrite file", "replace file content"}, Guidance: "Use only when full file replacement is intended."},
	"bash":          {Summary: "Run shell/build/diagnostic commands", Triggers: []string{"run command", "build project", "inspect shell output", "manual check"}, Guidance: "Use for shell commands, builds, and diagnostics. Always set purpose to verification, build, diagnostic, or other.", PreserveParameterDocs: true},
	"test":          {Summary: "Run focused tests or checks", Triggers: []string{"run tests", "verify fix", "execute test suite", "check command"}, Guidance: "Prefer for focused validation and reproducible checks."},
	"git":           {Summary: "Inspect git status, diff, and history", Triggers: []string{"git diff", "git status", "commit history", "changed files"}, Guidance: "Use for diff, history, and status rather than shelling out."},
	"open":          {Summary: "Open a file in the editor", Triggers: []string{"open file", "jump to line", "open editor"}, Guidance: "Use when interactive editor navigation is needed."},
	"question":      {Summary: "Ask the user a short option-based question only when a real decision blocks progress.", Triggers: []string{"ask user to choose", "need user decision", "approve or reject choice"}, Guidance: "Use only when you are blocked on a real user decision. Keep the question short and put context in normal text before calling.", PreserveParameterDocs: true},
	"task":          {Summary: "Track plan tasks", Triggers: []string{"create tasks", "update task status", "manage task list"}, Guidance: "Use for structured task tracking."},
	"task_output":   {Summary: "Read background task output", Triggers: []string{"background task output", "check task result", "poll task"}, Guidance: "Use to inspect background bash task progress or results."},
	"search_skills": {Summary: "Find relevant project skills", Triggers: []string{"find skill", "project convention", "workflow guidance", "skill discovery"}, Guidance: "Use before loading a skill when you are not sure which one fits."},
	"skill":         {Summary: "Load a skill's instructions", Triggers: []string{"load skill", "apply convention", "workflow instructions", "project skill"}, Guidance: "Use after identifying the relevant skill."},
	"subagent":      {Summary: "Delegate a bounded task to a specialized agent. Include the needed context because it cannot see the parent conversation.", Triggers: []string{"delegate bounded subtask", "parallel subagent work", "planner agent", "explorer agent", "reviewer agent"}, Guidance: "Use only for bounded subtasks. When tasks are independent, emit multiple subagent calls together so they run in parallel.", PreserveParameterDocs: true},
	"web_fetch":     {Summary: "Fetch a specific external URL or page. This is not a web search tool.", Triggers: []string{"fetch direct url", "load external page", "retrieve docs url"}, Guidance: "Use only with a direct URL. Do not use it to discover sites or search the web.", PreserveParameterDocs: true},
	"memory":        {Summary: "Save or retrieve persistent project memory", Triggers: []string{"remember this", "save memory", "list memory"}, Guidance: "Use for durable project-level notes and discoveries."},
}

type scoredTool struct {
	tool    provider.Tool
	score   int
	reasons []string
}

func reorderToolsForTurn(tools []provider.Tool, userMessage string, touchedFiles []string) []provider.Tool {
	scored := scoreToolsForTurn(tools, userMessage, touchedFiles)
	if len(scored) == 0 {
		out := append([]provider.Tool(nil), tools...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	out := make([]provider.Tool, 0, len(tools))
	seen := make(map[string]bool, len(scored))
	for _, item := range scored {
		out = append(out, item.tool)
		seen[item.tool.Name] = true
	}
	var unscored []provider.Tool
	for _, tool := range tools {
		if seen[tool.Name] {
			continue
		}
		unscored = append(unscored, tool)
	}
	sort.Slice(unscored, func(i, j int) bool { return unscored[i].Name < unscored[j].Name })
	out = append(out, unscored...)
	return out
}

func buildRelevantToolsOverlay(tools []provider.Tool, userMessage string, touchedFiles []string) string {
	scored := scoreToolsForTurn(tools, userMessage, touchedFiles)
	if len(scored) == 0 {
		return ""
	}
	if len(scored) > 4 {
		scored = scored[:4]
	}

	var sb strings.Builder
	sb.WriteString("# Likely Relevant Tools For This Turn\n")
	sb.WriteString("Prefer these available tools first if they fit:\n")
	for _, item := range scored {
		meta := toolPromptMetaForTool(item.tool)
		sb.WriteString("- ")
		sb.WriteString(item.tool.Name)
		desc := meta.Summary
		if desc == "" {
			desc = compactDescription(item.tool.Name, item.tool.Description)
		}
		if desc != "" {
			sb.WriteString(": ")
			sb.WriteString(desc)
		}
		if len(item.reasons) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(item.reasons, ", "))
			sb.WriteString("]")
		}
		sb.WriteByte('\n')
		if meta.Guidance != "" {
			sb.WriteString("  ")
			sb.WriteString(meta.Guidance)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimSpace(sb.String())
}

func compactToolDescription(name, description string) string {
	return compactToolDescriptionForTool(provider.Tool{Name: name, Description: description})
}

func compactToolDescriptionForTool(t provider.Tool) string {
	meta := toolPromptMetaForTool(t)
	if meta.Summary != "" {
		return meta.Summary
	}
	return compactDescription(t.Name, t.Description)
}

func compactDescription(_ string, description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}
	if idx := strings.Index(description, "\n"); idx >= 0 {
		description = description[:idx]
	}
	if idx := strings.Index(description, ". "); idx >= 0 {
		description = description[:idx+1]
	}
	if len(description) > 96 {
		description = strings.TrimSpace(description[:96]) + "..."
	}
	return description
}

func scoreToolsForTurn(tools []provider.Tool, userMessage string, touchedFiles []string) []scoredTool {
	queryTokens := toolTokenSet(userMessage)
	fileExts := touchedFileExts(touchedFiles)
	pathCount := len(touchedFiles)
	var scored []scoredTool
	for _, t := range tools {
		score := 0
		var reasons []string
		meta := toolPromptMetaForTool(t)

		if len(queryTokens) > 0 {
			if overlap := tokenOverlap(queryTokens, toolTokenSet(t.Name)); overlap > 0 {
				score += overlap * 20
				reasons = append(reasons, "name match")
			}
			if overlap := tokenOverlap(queryTokens, toolTokenSet(t.Description)); overlap > 0 {
				score += overlap * 4
				reasons = append(reasons, "description overlap")
			}
			for _, trigger := range meta.Triggers {
				if overlap := tokenOverlap(queryTokens, toolTokenSet(trigger)); overlap > 0 {
					score += overlap * 10
					reasons = append(reasons, "task fit")
				}
			}
			score, reasons = applyToolHeuristics(score, reasons, t.Name, userMessage, queryTokens)
		}

		if len(fileExts) > 0 && len(meta.FileExts) > 0 {
			for _, ext := range meta.FileExts {
				if fileExts[ext] {
					score += 8
					reasons = append(reasons, "file context")
					break
				}
			}
		}
		score, reasons = applyPathHeuristics(score, reasons, t.Name, pathCount, fileExts)

		if score == 0 {
			continue
		}
		scored = append(scored, scoredTool{
			tool: provider.Tool{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
				PromptHints: t.PromptHints,
			},
			score:   score,
			reasons: uniqueReasonStrings(reasons),
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].tool.Name < scored[j].tool.Name
		}
		return scored[i].score > scored[j].score
	})

	if len(scored) == 0 {
		return nil
	}
	return scored
}

func applyToolHeuristics(score int, reasons []string, toolName, userMessage string, queryTokens map[string]bool) (int, []string) {
	hasAny := func(tokens ...string) bool {
		for _, token := range tokens {
			if queryTokens[token] {
				return true
			}
		}
		return false
	}
	switch toolName {
	case "search":
		if hasAny("find", "search", "where", "which", "explore", "investigate", "architecture") {
			score += 18
			reasons = append(reasons, "broad lookup")
		}
	case "read_files":
		if hasAny("read", "inspect", "compare", "review") {
			score += 14
			reasons = append(reasons, "multi-file read")
		}
	case "read":
		if hasAny("read", "inspect", "look", "show") {
			score += 10
			reasons = append(reasons, "targeted read")
		}
	case "grep":
		if hasAny("regex", "exact", "string", "match", "grep") {
			score += 16
			reasons = append(reasons, "exact search")
		}
	case "glob":
		if hasAny("file", "files", "path", "paths", "pattern", "glob") {
			score += 12
			reasons = append(reasons, "path search")
		}
	case "lsp":
		if hasAny("symbol", "symbols", "reference", "references", "definition", "definitions", "caller", "callers", "rename", "diagnostic", "diagnostics") {
			score += 20
			reasons = append(reasons, "symbol lookup")
		}
	case "edit", "patch", "write":
		if hasAny("fix", "change", "update", "edit", "implement", "refactor", "rename", "create", "write") {
			score += 14
			reasons = append(reasons, "editing")
		}
	case "test":
		if hasAny("test", "tests", "verify", "validation", "reproduce") {
			score += 18
			reasons = append(reasons, "validation")
		}
	case "bash":
		if hasAny("build", "run", "shell", "command", "compile", "benchmark") {
			score += 16
			reasons = append(reasons, "shell command")
		}
	case "git":
		if hasAny("diff", "status", "commit", "history", "branch", "changed") {
			score += 18
			reasons = append(reasons, "git context")
		}
	case "search_skills", "skill":
		if hasAny("skill", "convention", "workflow", "pattern") {
			score += 18
			reasons = append(reasons, "skill guidance")
		}
	case "question":
		if hasAny("choose", "choice", "decision", "approve", "reject") || (queryTokens["user"] && hasAny("ask", "clarify")) {
			score += 18
			reasons = append(reasons, "user decision")
		}
	case "subagent":
		if hasAny("delegate", "parallel", "planner", "explorer", "reviewer", "subagent") {
			score += 16
			reasons = append(reasons, "delegation")
		}
	case "web_fetch":
		if containsDirectURL(userMessage) {
			score += 24
			reasons = append(reasons, "direct url")
		} else if hasAny("docs", "documentation", "page", "fetch", "url") {
			score += 10
			reasons = append(reasons, "page retrieval")
		}
	}
	return score, reasons
}

func applyPathHeuristics(score int, reasons []string, toolName string, pathCount int, fileExts map[string]bool) (int, []string) {
	if pathCount == 0 {
		return score, reasons
	}
	switch toolName {
	case "read":
		if pathCount == 1 {
			score += 16
			reasons = append(reasons, "explicit file")
		}
	case "read_files":
		if pathCount > 1 {
			score += 20
			reasons = append(reasons, "multiple files")
		}
	case "open":
		if pathCount == 1 {
			score += 12
			reasons = append(reasons, "file navigation")
		}
	case "lsp":
		if hasCodeFileExt(fileExts) {
			score += 12
			reasons = append(reasons, "code file")
		}
	}
	return score, reasons
}

func hasCodeFileExt(fileExts map[string]bool) bool {
	for _, ext := range toolPromptMetadata["lsp"].FileExts {
		if fileExts[ext] {
			return true
		}
	}
	return false
}

func toolPromptMetaForTool(t provider.Tool) toolPromptMeta {
	if meta, ok := toolPromptMetadata[t.Name]; ok {
		return mergeToolPromptMeta(meta, t.PromptHints)
	}
	return mergeToolPromptMeta(deriveToolPromptMeta(t), t.PromptHints)
}

func deriveToolPromptMeta(t provider.Tool) toolPromptMeta {
	allText := strings.TrimSpace(strings.Join([]string{
		t.Name,
		t.Description,
		strings.Join(toolSchemaPropertyNames(t.Parameters), " "),
	}, " "))
	tokens := toolTokenSet(allText)
	kind := classifyDerivedTool(tokens)
	meta := toolPromptMeta{
		Summary:  compactDescription(t.Name, t.Description),
		Guidance: "Use when this specialized tool clearly matches the target system or task.",
	}
	if meta.Summary == "" {
		meta.Summary = humanizeToolName(t.Name)
	}
	switch kind {
	case derivedToolRead:
		meta.Guidance = "Prefer this specialized read/search tool before shelling out when it matches the target system."
		meta.Triggers = append(meta.Triggers, "specialized lookup", "read external data")
	case derivedToolWrite:
		meta.Guidance = "Use only when this specialized tool is the intended system of record or action surface."
		meta.Triggers = append(meta.Triggers, "update external system", "create external resource")
	case derivedToolCode:
		meta.Guidance = "Prefer for precise symbols, references, definitions, or diagnostics in its domain."
		meta.Triggers = append(meta.Triggers, "references and diagnostics", "code intelligence")
	case derivedToolAnalysis:
		meta.Guidance = "Use for bounded expert analysis when its domain matches the task."
		meta.Triggers = append(meta.Triggers, "specialized analysis")
	default:
		meta.Triggers = append(meta.Triggers, "specialized tool")
	}
	if keywords := salientToolKeywords(t.Name, t.Description, 4); len(keywords) > 0 {
		meta.Triggers = append(meta.Triggers, strings.Join(keywords, " "))
	}
	return meta
}

type derivedToolKind int

const (
	derivedToolUnknown derivedToolKind = iota
	derivedToolRead
	derivedToolWrite
	derivedToolCode
	derivedToolAnalysis
)

func classifyDerivedTool(tokens map[string]bool) derivedToolKind {
	hasAny := func(words ...string) bool {
		for _, word := range words {
			if tokens[word] {
				return true
			}
		}
		return false
	}
	switch {
	case hasAny("definition", "definitions", "reference", "references", "symbol", "symbols", "diagnostic", "diagnostics", "lint"):
		return derivedToolCode
	case hasAny("read", "get", "list", "lookup", "search", "query", "fetch", "show", "inspect"):
		return derivedToolRead
	case hasAny("write", "create", "update", "edit", "delete", "remove", "apply", "submit", "trigger", "execute"):
		return derivedToolWrite
	case hasAny("review", "analyze", "analysis", "summarize", "summary", "plan"):
		return derivedToolAnalysis
	default:
		return derivedToolUnknown
	}
}

func toolSchemaPropertyNames(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var toolKeywordStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "by": true, "for": true, "from": true,
	"in": true, "into": true, "of": true, "on": true, "or": true, "the": true,
	"this": true, "to": true, "tool": true, "use": true, "with": true,
}

func salientToolKeywords(name, description string, limit int) []string {
	text := humanizeToolName(name) + " " + description
	var out []string
	seen := make(map[string]bool)
	for _, field := range strings.Fields(strings.ToLower(text)) {
		token := strings.TrimFunc(field, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if token == "" || toolKeywordStopwords[token] || seen[token] {
			continue
		}
		if len(token) < 3 {
			continue
		}
		seen[token] = true
		out = append(out, token)
		if len(out) == limit {
			break
		}
	}
	return out
}

func humanizeToolName(name string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ")
	return strings.TrimSpace(replacer.Replace(name))
}

func containsDirectURL(text string) bool {
	return strings.Contains(text, "http://") || strings.Contains(text, "https://")
}

func mergeToolPromptMeta(base toolPromptMeta, hints provider.ToolPromptHints) toolPromptMeta {
	if hints.Summary != "" {
		base.Summary = hints.Summary
	}
	if hints.Guidance != "" {
		base.Guidance = hints.Guidance
	}
	if len(hints.Triggers) > 0 {
		base.Triggers = appendUniqueStrings(base.Triggers, hints.Triggers...)
	}
	if len(hints.FileExts) > 0 {
		base.FileExts = appendUniqueStrings(base.FileExts, hints.FileExts...)
	}
	if hints.PreserveParameterDocs {
		base.PreserveParameterDocs = true
	}
	return base
}

func touchedFileExts(paths []string) map[string]bool {
	out := make(map[string]bool)
	for _, path := range paths {
		dot := strings.LastIndexByte(path, '.')
		if dot <= 0 || dot >= len(path)-1 {
			continue
		}
		out[strings.ToLower(path[dot:])] = true
	}
	return out
}

func toolTokenSet(text string) map[string]bool {
	out := make(map[string]bool)
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		out[current.String()] = true
		current.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func tokenOverlap(a, b map[string]bool) int {
	count := 0
	for token := range a {
		if b[token] {
			count++
		}
	}
	return count
}

func uniqueReasonStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func appendUniqueStrings(base []string, extra ...string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out))
	for _, item := range out {
		seen[item] = true
	}
	for _, item := range extra {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
