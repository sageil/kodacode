package tool

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/sageil/kodacode/internal/workspace"
)

type bashEffectAnalysis struct {
	Effect ExecutionEffect
}

type bashEffectAnalyzer struct {
	effect       ExecutionEffect
	visitedCalls int
	hereDocs     []string
}

func analyzeBashEffect(command string) (bashEffectAnalysis, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return bashEffectAnalysis{}, err
	}
	analyzer := &bashEffectAnalyzer{effect: ExecutionEffectRead}
	analyzer.visitStmtList(file.Stmts)
	if analyzer.visitedCalls == 0 || analyzer.effect == "" {
		analyzer.effect = ExecutionEffectUnknown
	}
	return bashEffectAnalysis{Effect: analyzer.effect}, nil
}

func bashExecutionEffect(pathAnalysis bashPathAnalysis, intentAnalysis bashIntentAnalysis, networkTargets []string, command string) ExecutionEffect {
	if intentAnalysis.Intent == ExecutionIntentWatcher || intentAnalysis.Intent == ExecutionIntentServer {
		return ExecutionEffectProcess
	}
	for _, request := range pathAnalysis.Requests {
		if request.Access == workspace.AccessWrite {
			return ExecutionEffectWrite
		}
	}
	if strings.TrimSpace(pathAnalysis.OpaquePathReason) != "" {
		if len(networkTargets) > 0 {
			return ExecutionEffectNetwork
		}
		return ExecutionEffectUnknown
	}
	effectAnalysis, err := analyzeBashEffect(command)
	if err != nil {
		if len(networkTargets) > 0 {
			return ExecutionEffectNetwork
		}
		return ExecutionEffectUnknown
	}
	switch effectAnalysis.Effect {
	case ExecutionEffectWrite:
		return ExecutionEffectWrite
	case ExecutionEffectProcess:
		return ExecutionEffectProcess
	case ExecutionEffectRead:
		if len(networkTargets) > 0 {
			return ExecutionEffectNetwork
		}
		return ExecutionEffectRead
	default:
		if len(networkTargets) > 0 {
			return ExecutionEffectNetwork
		}
		return ExecutionEffectUnknown
	}
}

func (a *bashEffectAnalyzer) merge(effect ExecutionEffect) {
	switch effect {
	case ExecutionEffectProcess:
		if a.effect != ExecutionEffectWrite {
			a.effect = ExecutionEffectProcess
		}
	case ExecutionEffectWrite:
		a.effect = ExecutionEffectWrite
	case ExecutionEffectUnknown:
		if a.effect != ExecutionEffectWrite && a.effect != ExecutionEffectProcess {
			a.effect = ExecutionEffectUnknown
		}
	case ExecutionEffectRead:
		if a.effect == "" {
			a.effect = ExecutionEffectRead
		}
	}
}

func (a *bashEffectAnalyzer) visitStmtList(stmts []*syntax.Stmt) {
	for _, stmt := range stmts {
		a.visitStmt(stmt)
	}
}

func (a *bashEffectAnalyzer) visitStmt(stmt *syntax.Stmt) {
	if stmt == nil {
		return
	}
	if stmt.Background || stmt.Coprocess || stmt.Disown {
		a.merge(ExecutionEffectProcess)
		return
	}
	previousHereDocs := a.hereDocs
	a.hereDocs = nil
	for _, redirect := range stmt.Redirs {
		a.visitRedirect(redirect)
	}
	a.visitCommand(stmt.Cmd)
	a.hereDocs = previousHereDocs
}

func (a *bashEffectAnalyzer) visitCommand(cmd syntax.Command) {
	switch typed := cmd.(type) {
	case nil:
		return
	case *syntax.CallExpr:
		a.visitCallExpr(typed)
	case *syntax.BinaryCmd:
		a.visitStmt(typed.X)
		a.visitStmt(typed.Y)
	case *syntax.Block:
		a.visitStmtList(typed.Stmts)
	case *syntax.Subshell:
		a.visitStmtList(typed.Stmts)
	case *syntax.IfClause:
		a.visitStmtList(typed.Cond)
		a.visitStmtList(typed.Then)
		for next := typed.Else; next != nil; next = next.Else {
			a.visitStmtList(next.Cond)
			a.visitStmtList(next.Then)
		}
	case *syntax.WhileClause:
		a.visitStmtList(typed.Cond)
		a.visitStmtList(typed.Do)
	case *syntax.ForClause:
		a.visitStmtList(typed.Do)
	case *syntax.CaseClause:
		for _, item := range typed.Items {
			if item != nil {
				a.visitStmtList(item.Stmts)
			}
		}
	case *syntax.TimeClause:
		a.visitStmt(typed.Stmt)
	case *syntax.CoprocClause:
		a.merge(ExecutionEffectProcess)
	case *syntax.FuncDecl:
		return
	default:
		a.merge(ExecutionEffectUnknown)
	}
}

func (a *bashEffectAnalyzer) visitCallExpr(call *syntax.CallExpr) {
	args, ok := shellLiteralCallArgs(call)
	if !ok || len(args) == 0 {
		a.visitedCalls++
		a.merge(ExecutionEffectUnknown)
		return
	}
	a.visitedCalls++
	a.merge(classifyLiteralBashCallEffect(args, a.hereDocs))
}

func (a *bashEffectAnalyzer) visitRedirect(redirect *syntax.Redirect) {
	if redirect == nil {
		return
	}
	if redirectAccess(redirect) == workspace.AccessWrite {
		a.merge(ExecutionEffectWrite)
		return
	}
	if redirect.Word != nil && shellWordHasDynamicExpansion(redirect.Word) {
		a.merge(ExecutionEffectUnknown)
	}
	if redirect.Hdoc != nil {
		if body, ok := shellWordLiteral(redirect.Hdoc); ok {
			a.hereDocs = append(a.hereDocs, body)
		} else {
			a.merge(ExecutionEffectUnknown)
		}
	}
}

func classifyLiteralBashCallEffect(args []string, hereDocs []string) ExecutionEffect {
	return classifyLiteralBashCallEffectDepth(args, hereDocs, 0)
}

func classifyLiteralBashCallEffectDepth(args []string, hereDocs []string, depth int) ExecutionEffect {
	if depth > 3 {
		return ExecutionEffectUnknown
	}
	command := strings.ToLower(filepath.Base(strings.TrimSpace(args[0])))
	switch command {
	case "", "cd", "pushd", "popd", "pwd", "true", "false", ":", "echo", "printf", "date", "sleep", "printenv":
		return ExecutionEffectRead
	case "env":
		return classifyEnvCallEffect(args[1:], hereDocs, depth)
	case "ls", "cat", "head", "tail", "wc", "nl", "file", "stat", "find", "grep", "egrep", "fgrep", "rg", "sed",
		"awk", "jq", "cut", "sort", "uniq", "du", "tree", "basename", "dirname", "realpath", "readlink":
		if literalReadCommandHasMutatingFlags(command, args[1:]) {
			return ExecutionEffectUnknown
		}
		return ExecutionEffectRead
	case "git":
		if len(args) < 2 || !gitCommandLooksReadOnly(args[1:]) {
			return ExecutionEffectUnknown
		}
		return ExecutionEffectRead
	case "go":
		if len(args) < 2 || !goCommandLooksReadOnly(args[1:]) {
			return ExecutionEffectUnknown
		}
		return ExecutionEffectRead
	case "npm", "pnpm", "yarn":
		if len(args) < 2 || !nodePackageManagerCommandLooksReadOnly(args[1:]) {
			return ExecutionEffectUnknown
		}
		return ExecutionEffectRead
	case "python", "python3":
		if pythonLiteralCommandLooksReadOnly(args[1:], hereDocs) {
			return ExecutionEffectRead
		}
		return ExecutionEffectUnknown
	case "command":
		if wrapped, ok := commandWrappedCommandArgs(args[1:]); ok {
			return classifyLiteralBashCallEffectDepth(wrapped, hereDocs, depth+1)
		}
		return ExecutionEffectRead
	case "type", "which":
		return ExecutionEffectRead
	default:
		return ExecutionEffectUnknown
	}
}

func classifyEnvCallEffect(args []string, hereDocs []string, depth int) ExecutionEffect {
	for idx := 0; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "" {
			continue
		}
		if arg == "-" {
			continue
		}
		if strings.Contains(arg, "=") && !strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "-i" || arg == "--ignore-environment" || arg == "-0" || arg == "--null" {
			continue
		}
		if arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir" {
			idx++
			continue
		}
		if arg == "-S" || arg == "--split-string" || strings.HasPrefix(arg, "--split-string=") {
			return ExecutionEffectUnknown
		}
		if strings.HasPrefix(arg, "-u") && len(arg) > len("-u") {
			continue
		}
		if strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir=") {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return ExecutionEffectUnknown
		}
		return classifyLiteralBashCallEffectDepth(args[idx:], hereDocs, depth+1)
	}
	return ExecutionEffectRead
}

func commandWrappedCommandArgs(args []string) ([]string, bool) {
	for idx := 0; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "" {
			continue
		}
		switch arg {
		case "-p":
			continue
		case "-v", "-V":
			return nil, false
		}
		if strings.HasPrefix(arg, "-") {
			return nil, false
		}
		return args[idx:], true
	}
	return nil, false
}

func literalReadCommandHasMutatingFlags(command string, args []string) bool {
	switch command {
	case "find":
		for _, arg := range args {
			switch strings.TrimSpace(arg) {
			case "-delete", "-exec", "-execdir", "-ok", "-okdir":
				return true
			}
		}
	case "sed":
		for _, arg := range args {
			trimmed := strings.TrimSpace(arg)
			if trimmed == "-i" || strings.HasPrefix(trimmed, "-i") ||
				trimmed == "--in-place" || strings.HasPrefix(trimmed, "--in-place=") {
				return true
			}
		}
	case "awk":
		for _, arg := range args {
			trimmed := strings.TrimSpace(arg)
			if trimmed == "-i" || strings.HasPrefix(trimmed, "-i") {
				return true
			}
		}
	}
	return false
}

func gitCommandLooksReadOnly(args []string) bool {
	if len(args) == 0 {
		return false
	}
	subcommand := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcommand {
	case "status", "diff", "show", "log", "grep", "ls-files", "ls-tree", "rev-parse", "describe", "show-ref",
		"blame", "shortlog":
		return true
	case "tag":
		return gitTagLooksReadOnly(args[1:])
	case "branch":
		return !hasAnyFlag(args[1:], "-d", "-D", "-m", "-M", "-c", "-C", "--delete", "--move", "--copy", "--set-upstream-to", "-u", "--unset-upstream")
	case "remote":
		if len(args) == 1 {
			return true
		}
		switch strings.ToLower(strings.TrimSpace(args[1])) {
		case "-v", "--verbose", "show", "get-url":
			return true
		default:
			return false
		}
	case "config":
		return gitConfigLooksReadOnly(args[1:])
	default:
		return false
	}
}

func gitTagLooksReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	listMode := false
	for idx := 0; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "" {
			continue
		}
		switch arg {
		case "-d", "--delete", "-a", "--annotate", "-s", "--sign", "-u", "--local-user", "-f", "--force", "-m", "--message", "-F", "--file", "--cleanup", "--create-reflog":
			return false
		case "-l", "--list":
			listMode = true
			continue
		case "-n", "--contains", "--no-contains", "--points-at", "--merged", "--no-merged", "--sort", "--format", "--color", "--column":
			if idx+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[idx+1]), "-") {
				idx++
			}
			continue
		case "--no-column", "--no-color", "-i", "--ignore-case":
			continue
		}
		switch {
		case strings.HasPrefix(arg, "-n"),
			strings.HasPrefix(arg, "--contains="),
			strings.HasPrefix(arg, "--no-contains="),
			strings.HasPrefix(arg, "--points-at="),
			strings.HasPrefix(arg, "--merged="),
			strings.HasPrefix(arg, "--no-merged="),
			strings.HasPrefix(arg, "--sort="),
			strings.HasPrefix(arg, "--format="),
			strings.HasPrefix(arg, "--color="),
			strings.HasPrefix(arg, "--column="):
			continue
		case strings.HasPrefix(arg, "-"):
			return false
		case listMode:
			continue
		default:
			return false
		}
	}
	return true
}

func gitConfigLooksReadOnly(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--get" || trimmed == "--get-all" || trimmed == "--get-regexp" ||
			trimmed == "--list" || trimmed == "-l" || trimmed == "--name-only" ||
			trimmed == "--show-origin" || trimmed == "--show-scope" ||
			trimmed == "--includes" || trimmed == "--null" || trimmed == "-z" {
			continue
		}
		if strings.HasPrefix(trimmed, "--type=") || strings.HasPrefix(trimmed, "--bool") ||
			strings.HasPrefix(trimmed, "--int") || strings.HasPrefix(trimmed, "--path") ||
			strings.HasPrefix(trimmed, "--fixed-value") || strings.HasPrefix(trimmed, "--file=") ||
			strings.HasPrefix(trimmed, "--global") || strings.HasPrefix(trimmed, "--system") ||
			strings.HasPrefix(trimmed, "--local") || strings.HasPrefix(trimmed, "--worktree") {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			return false
		}
	}
	return hasAnyFlag(args, "--get", "--get-all", "--get-regexp", "--list", "-l") || len(nonFlagArgs(args)) <= 1
}

func goCommandLooksReadOnly(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "version", "list", "doc", "help":
		return true
	case "env":
		return !hasAnyFlag(args[1:], "-w", "-u")
	default:
		return false
	}
}

func nodePackageManagerCommandLooksReadOnly(args []string) bool {
	subcommand := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcommand {
	case "list", "ls", "view", "info", "outdated", "why", "help", "--version", "-v":
		return true
	default:
		return false
	}
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if strings.TrimSpace(arg) == flag {
				return true
			}
		}
	}
	return false
}

func nonFlagArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func pythonLiteralCommandLooksReadOnly(args []string, hereDocs []string) bool {
	if len(hereDocs) != 1 {
		return false
	}
	if len(args) > 1 {
		return false
	}
	if len(args) == 1 {
		switch strings.TrimSpace(args[0]) {
		case "-", "":
		default:
			return false
		}
	}
	return pythonScriptLooksReadOnly(hereDocs[0])
}

func pythonScriptLooksReadOnly(script string) bool {
	text := strings.ToLower(script)
	for _, marker := range []string{
		"write_text", "write_bytes", ".write(", "open(", "touch(", "unlink(", "rename(", "replace(",
		"mkdir(", "makedirs(", "rmdir(", "removedirs(", "remove(", "shutil.", "subprocess.",
		"os.system", "os.popen", "exec(", "eval(",
	} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	for _, marker := range []string{".read_text(", ".read_bytes(", ".read(", "path(", "print("} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
