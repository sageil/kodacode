package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const maxBashIntentResolutionDepth = 3

var genericServerReadyPatterns = []string{
	"local:",
	"listening on",
	"started server",
	"server running",
	"ready in",
	"ready on",
	"serving http on",
	"running at:",
	"project is running at",
	"uvicorn running on",
}

type bashIntentAnalysis struct {
	Intent        ExecutionIntent
	IntentCommand string
	ReadyPatterns []string
}

type bashIntentAnalyzer struct {
	workingDir string
	depth      int

	intent        ExecutionIntent
	intentCommand string
	readyPatterns []string
	classified    bool
}

func analyzeBashIntent(workingDir, command string) (bashIntentAnalysis, error) {
	return analyzeBashIntentWithDepth(workingDir, command, 0)
}

func analyzeBashIntentWithDepth(workingDir, command string, depth int) (bashIntentAnalysis, error) {
	if strings.TrimSpace(command) == "" {
		return bashIntentAnalysis{Intent: ExecutionIntentUnknown}, nil
	}
	if depth >= maxBashIntentResolutionDepth {
		return bashIntentAnalysis{Intent: ExecutionIntentUnknown}, nil
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return bashIntentAnalysis{}, err
	}
	analyzer := &bashIntentAnalyzer{
		workingDir: strings.TrimSpace(workingDir),
		depth:      depth,
		intent:     ExecutionIntentUnknown,
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		if analyzer.classified {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		analyzer.visitCallExpr(call)
		return !analyzer.classified
	})
	if analyzer.intent == "" {
		analyzer.intent = ExecutionIntentUnknown
	}
	return bashIntentAnalysis{
		Intent:        analyzer.intent,
		IntentCommand: analyzer.intentCommand,
		ReadyPatterns: append([]string(nil), analyzer.readyPatterns...),
	}, nil
}

func (a *bashIntentAnalyzer) visitCallExpr(call *syntax.CallExpr) {
	args, ok := shellLiteralCallArgs(call)
	if !ok || len(args) == 0 {
		if call != nil && len(call.Args) > 0 && shellWordHasDynamicExpansion(call.Args[0]) {
			a.setUnknown()
		}
		return
	}
	commandName := strings.TrimSpace(args[0])
	if commandName == "" {
		return
	}
	if isDirectoryChangingShellBuiltin(commandName) {
		a.updateWorkingDir(args[1:])
		return
	}
	if isIntentSetupBuiltin(commandName) {
		return
	}

	analysis := classifyLiteralExecutionIntent(a.workingDir, args, a.depth)
	a.intent = analysis.Intent
	if strings.TrimSpace(a.intentCommand) == "" || analysis.Intent != ExecutionIntentOneShot {
		a.intentCommand = analysis.IntentCommand
		a.readyPatterns = append([]string(nil), analysis.ReadyPatterns...)
	}
	if analysis.Intent != ExecutionIntentOneShot {
		a.classified = true
	}
}

func (a *bashIntentAnalyzer) updateWorkingDir(args []string) {
	if nextDir, ok := resolveLiteralShellWorkingDir(a.workingDir, args); ok {
		a.workingDir = nextDir
	}
}

func (a *bashIntentAnalyzer) setUnknown() {
	if a.classified {
		return
	}
	a.intent = ExecutionIntentUnknown
	a.classified = true
}

func shellLiteralCallArgs(call *syntax.CallExpr) ([]string, bool) {
	if call == nil || len(call.Args) == 0 {
		return nil, false
	}
	args := make([]string, 0, len(call.Args))
	for _, arg := range call.Args {
		literal, ok := shellWordLiteral(arg)
		if !ok {
			return nil, false
		}
		args = append(args, literal)
	}
	return args, true
}

func classifyLiteralExecutionIntent(workingDir string, args []string, depth int) bashIntentAnalysis {
	args = unwrapIntentCommandArgs(args)
	if len(args) == 0 {
		return bashIntentAnalysis{Intent: ExecutionIntentUnknown}
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	commandText := strings.Join(args, " ")

	if scriptName, ok := packageManagerScriptName(args); ok {
		if scriptCommand, scriptDir, ok := resolvePackageScript(workingDir, scriptName); ok {
			resolved, err := analyzeBashIntentWithDepth(scriptDir, scriptCommand, depth+1)
			if err == nil && resolved.Intent != ExecutionIntentUnknown {
				if strings.TrimSpace(resolved.IntentCommand) == "" {
					resolved.IntentCommand = strings.TrimSpace(scriptCommand)
				}
				return resolved
			}
		}
	}

	switch {
	case base == "vite":
		return classifyViteIntent(args, commandText)
	case base == "next" && hasLiteralArg(args[1:], "dev"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"ready in", "started server", "local:"}}
	case base == "nuxt" && hasLiteralArg(args[1:], "dev"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"local:", "ready"}}
	case base == "astro" && hasLiteralArg(args[1:], "dev"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"local:", "ready in"}}
	case base == "webpack" && hasLiteralArg(args[1:], "serve"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"project is running at", "webpack compiled"}}
	case base == "react-scripts" && hasLiteralArg(args[1:], "start"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"local:", "compiled successfully"}}
	case isPythonHTTPServer(args):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"serving http on"}}
	case base == "air" || base == "fresh" || base == "reflex":
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case base == "cargo" && hasLiteralArg(args[1:], "watch"):
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case base == "trunk" && hasLiteralArg(args[1:], "serve"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: appendServerReadyPatterns("listening at", "available at")}
	case base == "mdbook" && hasLiteralArg(args[1:], "serve"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: appendServerReadyPatterns("serving on")}
	case base == "dotnet" && hasLiteralArg(args[1:], "watch"):
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case isPHPBuiltInServer(args):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: appendServerReadyPatterns("development server")}
	case isPHPArtisanServe(args):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: appendServerReadyPatterns("server running on", "development server")}
	case base == "symfony" && hasLiteralArg(args[1:], "serve"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: appendServerReadyPatterns("web server listening")}
	case base == "uvicorn":
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"uvicorn running on"}}
	case base == "rails" && hasLiteralArg(args[1:], "server"):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"listening on", "booting"}}
	case isDockerComposeUp(args):
		return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"started", "listening on"}}
	case base == "vitest" && hasWatchFlag(args[1:]):
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case base == "jest" && hasWatchFlag(args[1:]):
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case (base == "tsc" || base == "vue-tsc") && hasWatchFlag(args[1:]):
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case base == "ts-node-dev" || base == "nodemon":
		if looksLikeServerTarget(args[1:]) {
			return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: append([]string(nil), genericServerReadyPatterns...)}
		}
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	case base == "node" && hasLiteralArg(args[1:], "--watch"):
		if looksLikeServerTarget(removeFlagAndValue(args[1:], "--watch")) {
			return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: append([]string(nil), genericServerReadyPatterns...)}
		}
		return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
	default:
		return bashIntentAnalysis{Intent: ExecutionIntentOneShot, IntentCommand: commandText}
	}
}

func unwrapIntentCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	switch base {
	case "env":
		for idx := 1; idx < len(args); idx++ {
			if !looksLikeEnvAssignment(args[idx]) {
				return args[idx:]
			}
		}
		return nil
	case "cross-env", "cross-env-shell":
		idx := 1
		for idx < len(args) && looksLikeEnvAssignment(args[idx]) {
			idx++
		}
		if idx < len(args) {
			return args[idx:]
		}
		return nil
	default:
		return args
	}
}

func isIntentSetupBuiltin(command string) bool {
	switch filepath.Base(strings.TrimSpace(command)) {
	case "export", "unset", "alias", "unalias":
		return true
	default:
		return false
	}
}

func looksLikeEnvAssignment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	idx := strings.IndexRune(value, '=')
	if idx <= 0 || idx == len(value)-1 {
		return false
	}
	for _, r := range value[:idx] {
		if r != '_' && r != '-' && r != '.' &&
			(r < 'A' || r > 'Z') &&
			(r < 'a' || r > 'z') &&
			(r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func packageManagerScriptName(args []string) (string, bool) {
	if len(args) < 2 {
		return "", false
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	switch base {
	case "npm":
		if len(args) >= 3 && strings.EqualFold(strings.TrimSpace(args[1]), "run") {
			return strings.TrimSpace(args[2]), true
		}
	case "pnpm":
		if len(args) >= 3 && strings.EqualFold(strings.TrimSpace(args[1]), "run") {
			return strings.TrimSpace(args[2]), true
		}
		if isImplicitPackageScript(args[1]) {
			return strings.TrimSpace(args[1]), true
		}
	case "yarn":
		if len(args) >= 3 && strings.EqualFold(strings.TrimSpace(args[1]), "run") {
			return strings.TrimSpace(args[2]), true
		}
		if isImplicitPackageScript(args[1]) {
			return strings.TrimSpace(args[1]), true
		}
	case "bun":
		if len(args) >= 3 && strings.EqualFold(strings.TrimSpace(args[1]), "run") {
			return strings.TrimSpace(args[2]), true
		}
	}
	return "", false
}

func isImplicitPackageScript(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	switch value {
	case "install", "add", "remove", "update", "upgrade", "exec", "create", "dlx", "publish", "why", "info", "config", "cache":
		return false
	default:
		return true
	}
}

func resolvePackageScript(workingDir, scriptName string) (command string, packageDir string, ok bool) {
	scriptName = strings.TrimSpace(scriptName)
	if scriptName == "" {
		return "", "", false
	}
	for dir := strings.TrimSpace(workingDir); dir != ""; dir = parentDir(dir) {
		path := filepath.Join(dir, "package.json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				if next := parentDir(dir); next != "" && next != dir {
					continue
				}
				break
			}
			return "", "", false
		}
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			return "", "", false
		}
		if script, exists := pkg.Scripts[scriptName]; exists && strings.TrimSpace(script) != "" {
			return strings.TrimSpace(script), dir, true
		}
		if next := parentDir(dir); next != "" && next != dir {
			continue
		}
		break
	}
	return "", "", false
}

func parentDir(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	parent := filepath.Dir(path)
	if parent == path {
		return ""
	}
	return parent
}

func hasLiteralArg(args []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, arg := range args {
		if strings.TrimSpace(arg) == target {
			return true
		}
	}
	return false
}

func hasWatchFlag(args []string) bool {
	for _, arg := range args {
		trimmed := strings.TrimSpace(strings.ToLower(arg))
		switch trimmed {
		case "--watch", "-w", "--watchall":
			return true
		}
		if strings.HasPrefix(trimmed, "--watch=") {
			return true
		}
	}
	return false
}

func isPythonHTTPServer(args []string) bool {
	if len(args) < 3 {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	if base != "python" && base != "python3" {
		return false
	}
	return strings.TrimSpace(args[1]) == "-m" && strings.TrimSpace(args[2]) == "http.server"
}

func isPHPBuiltInServer(args []string) bool {
	if len(args) < 2 {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	if base != "php" {
		return false
	}
	for _, arg := range args[1:] {
		if strings.TrimSpace(arg) == "-S" {
			return true
		}
	}
	return false
}

func isPHPArtisanServe(args []string) bool {
	if len(args) < 3 {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	if base != "php" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(args[1]), "artisan") && strings.EqualFold(strings.TrimSpace(args[2]), "serve")
}

func isDockerComposeUp(args []string) bool {
	if len(args) < 3 {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	if base != "docker" {
		return false
	}
	return strings.TrimSpace(args[1]) == "compose" && strings.TrimSpace(args[2]) == "up"
}

func classifyViteIntent(args []string, commandText string) bashIntentAnalysis {
	if hasLiteralArg(args[1:], "build") {
		if hasWatchFlag(args[1:]) {
			return bashIntentAnalysis{Intent: ExecutionIntentWatcher, IntentCommand: commandText}
		}
		return bashIntentAnalysis{Intent: ExecutionIntentOneShot, IntentCommand: commandText}
	}
	return bashIntentAnalysis{Intent: ExecutionIntentServer, IntentCommand: commandText, ReadyPatterns: []string{"local:", "ready in"}}
}

func looksLikeServerTarget(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(strings.TrimSpace(arg))
		if lower == "" || strings.HasPrefix(lower, "-") {
			continue
		}
		base := strings.ToLower(filepath.Base(lower))
		switch {
		case strings.Contains(base, "server"),
			strings.Contains(base, "app"),
			strings.Contains(base, "api"),
			strings.Contains(base, "web"),
			strings.Contains(base, "main"):
			return true
		}
	}
	return false
}

func removeFlagAndValue(args []string, flag string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		trimmed := strings.TrimSpace(arg)
		if trimmed == flag {
			skipNext = true
			continue
		}
		if strings.HasPrefix(trimmed, flag+"=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func appendServerReadyPatterns(extra ...string) []string {
	patterns := append([]string(nil), genericServerReadyPatterns...)
	for _, pattern := range extra {
		trimmed := strings.TrimSpace(strings.ToLower(pattern))
		if trimmed == "" {
			continue
		}
		duplicate := false
		for _, existing := range patterns {
			if existing == trimmed {
				duplicate = true
				break
			}
		}
		if !duplicate {
			patterns = append(patterns, trimmed)
		}
	}
	return patterns
}
