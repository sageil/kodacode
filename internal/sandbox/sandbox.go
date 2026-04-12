package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/sageil/kodacode/v1/internal/permission"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// resolveDir canonicalises dir by following symlinks.
func resolveDir(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return filepath.Clean(dir)
}

var errNeedsPermission = errors.New("needs permission")
var errBashGitMutationDenied = errors.New("git mutations via bash are not allowed; use the git tool")

var pathTools = map[string]string{
	"read":          "filePath",
	"write":         "filePath",
	"edit":          "filePath",
	"patch":         "filePath",
	"code_action":   "filePath",
	"rename_symbol": "filePath",
	"open":          "filePath",
	"glob":          "path",
	"grep":          "path",
	"read_files":    "path",
	"tree":          "path",
	"search":        "path",
	"test":          "path",
}

var rmPattern = regexp.MustCompile(`\b(rm|rmdir)\b`)
var awkPathScriptPattern = regexp.MustCompile(`(?:>>?|<)\s*["']?(?:/|~|\.\.?/)`)
var sedPathScriptPattern = regexp.MustCompile(`(^|[;{}])\s*[rw]\s+(?:/|~|\.\.?/)`)
var sedExecScriptPattern = regexp.MustCompile(`(^|[;{}])\s*e(?:\s|$)`)

var safeDevPaths = map[string]bool{
	"/dev/null":    true,
	"/dev/zero":    true,
	"/dev/urandom": true,
	"/dev/random":  true,
	"/dev/stdin":   true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
}

// ShellTokens splits a shell command into tokens, respecting single and
// double quotes so that paths with spaces are kept as single tokens.
// Quotes are stripped from the returned tokens.
func ShellTokens(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inSingle, inDouble, escaped := false, false, false

	for _, r := range cmd {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if (r == ' ' || r == '\t') && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// Sandbox dispatches tool calls with path confinement, delete permissions,
// and tool name normalization.
type Sandbox struct {
	projectDir       string
	origin           Origin
	registry         *tool.Registry
	permission       permission.Config
	allowedPaths     []string
	ignorePatterns   []string
	skillDirs        []string
	onBackgroundDone func(sessionID, taskID, command, description string, exitCode int, output string, elapsedMS int64)
}

type permissionDecision struct {
	action  permission.Action
	enforce bool
	denyMsg string
}

type bashPathAnalysis struct {
	externalPaths []string
	displayLines  []string
	ambiguous     bool
}

func (s *Sandbox) ProjectDir() string            { return s.projectDir }
func (s *Sandbox) Permission() permission.Config { return s.permission }
func (s *Sandbox) SkillDirs() []string           { return append([]string(nil), s.skillDirs...) }

// WithPermission returns a shallow copy of the Sandbox with overridden
// permission rules. This allows per-request agent permission overrides
// without mutating the shared Sandbox.
func (s *Sandbox) WithPermission(perm permission.Config) *Sandbox {
	if perm == nil {
		return s
	}
	cp := *s
	cp.permission = perm
	return &cp
}

type SandboxOptions struct {
	AllowedPaths     []string
	IgnorePatterns   []string
	SkillDirs        []string
	OnBackgroundDone func(sessionID, taskID, command, description string, exitCode int, output string, elapsedMS int64)
}

// New constructs a Sandbox rooted at projectDir (resolved through symlinks).
func New(projectDir string, origin Origin, registry *tool.Registry, perm permission.Config, opts ...SandboxOptions) *Sandbox {
	var resolved []string
	var ignorePatterns []string
	if len(opts) > 0 {
		for _, p := range opts[0].AllowedPaths {
			resolved = append(resolved, resolveDir(p))
		}
		ignorePatterns = opts[0].IgnorePatterns
	}
	var skillDirs []string
	if len(opts) > 0 {
		skillDirs = opts[0].SkillDirs
	}
	return &Sandbox{
		projectDir:     resolveDir(projectDir),
		origin:         origin,
		registry:       registry,
		permission:     perm,
		allowedPaths:   resolved,
		ignorePatterns: ignorePatterns,
		skillDirs:      skillDirs,
		onBackgroundDone: func() func(sessionID, taskID, command, description string, exitCode int, output string, elapsedMS int64) {
			if len(opts) == 0 {
				return nil
			}
			return opts[0].OnBackgroundDone
		}(),
	}
}

// Execute dispatches a tool call, enforcing all sandbox rules.
func (s *Sandbox) Execute(
	ctx context.Context,
	sessionID string,
	tc provider.ToolCall,
	ask func(toolName, input string) error,
	askUser func(question string, options []string, multiple bool, purpose string) (string, error),
	spawnSubagent func(ctx context.Context, agentID, task string) (string, error),
	publish func(eventType string, data any),
) (*tool.Result, error) {
	name, ok := s.normalizeToolName(tc.Name)
	if !ok {
		names := s.registry.Names()
		return &tool.Result{
			Output: fmt.Sprintf(
				"Unknown tool %q. Available tools: %s. Please use one of the available tool names.",
				tc.Name, strings.Join(names, ", "),
			),
		}, nil
	}
	tc.Name = name

	if field, isPathTool := pathTools[name]; isPathTool {
		tc.Arguments = normalizePathField(tc.Arguments, field)
		confined, err := s.confinePathArg(tc.Arguments, field)
		switch {
		case err == nil:
			tc.Arguments = confined
			if err := s.requirePermissionDecision(s.toolPermissionDecision(tc), ask, tc, name+" is denied by configuration"); err != nil {
				return nil, err
			}
		case errors.Is(err, errDeniedDirectory):
			return &tool.Result{Output: err.Error(), ErrorCode: "denied"}, nil
		default:
			resolvedArgs := s.resolvePathArg(tc.Arguments, field)
			resolvedPath, _ := extractStringArg(resolvedArgs, field)
			decision := combinePermissionDecisions(
				s.externalPathDecision(resolvedPath),
				s.toolPermissionDecisionForPath(name, resolvedPath),
			)
			tc.Arguments = resolvedArgs
			if askErr := s.requirePermissionDecision(decision, ask, tc, "external path access requires permission"); askErr != nil {
				return nil, askErr
			}
		}
	}

	if _, isPathTool := pathTools[name]; !isPathTool {
		if err := s.requirePermissionDecision(s.toolPermissionDecision(tc), ask, tc, name+" is denied by configuration"); err != nil {
			return nil, err
		}
	}

	switch name {
	case "bash":
		decision, promptArgs := s.bashOutsidePathDecision(tc.Arguments)
		tcForPrompt := tc
		tcForPrompt.Arguments = promptArgs
		if err := s.requirePermissionDecision(decision, ask, tcForPrompt, "bash command references paths outside project directory"); err != nil {
			return nil, err
		}
		if err := s.requirePermission(s.checkBashDelete(tc.Arguments), ask, tc, "bash delete commands require permission"); err != nil {
			return nil, err
		}
		if err := checkBashGitMutation(tc.Arguments); err != nil {
			return nil, err
		}
	case "git":
		if err := s.requirePermission(checkDestructiveGit(name, tc.Arguments), ask, tc, "destructive git operation requires permission"); err != nil {
			return nil, err
		}
	}

	t, _ := s.registry.Get(name)
	ectx := tool.ExecutionContext{
		SessionID:      sessionID,
		TaskSessionID:  tool.TaskSessionIDFromContext(ctx),
		ContextUsage:   tool.ContextUsageFromContext(ctx),
		Ephemeral:      ctx.Value(tool.EphemeralKey{}) != nil,
		IgnorePatterns: s.ignorePatterns,
		WorkDir:        s.projectDir,
		WriteOutput: func(chunk string) {
			publish("tool_output", map[string]string{"tool": name, "chunk": chunk})
		},
		Ask:           ask,
		AskUser:       askUser,
		SpawnSubagent: spawnSubagent,
		SkillDirs:     s.skillDirs,
		SkillPolicy:   tool.SkillPolicyFromContext(ctx),
		OnBackgroundDone: func(task *tool.BashTask) {
			out := task.Output()
			if len(out) > 4096 {
				out = out[len(out)-4096:]
			}
			if s.onBackgroundDone != nil {
				s.onBackgroundDone(sessionID, task.ID, task.Command, task.Description, task.ExitCode(), out, time.Since(task.StartTime).Milliseconds())
			}
		},
	}
	return t.Execute(ctx, ectx, []byte(tc.Arguments))
}

func (s *Sandbox) normalizeToolName(name string) (string, bool) {
	if _, ok := s.registry.Get(name); ok {
		return name, true
	}
	lower := strings.ToLower(name)
	if _, ok := s.registry.Get(lower); ok {
		return lower, true
	}
	return "", false
}

// KnownTool reports whether name (or its lowercase form) is registered.
func (s *Sandbox) KnownTool(name string) bool {
	_, ok := s.normalizeToolName(name)
	return ok
}

// requirePermission handles a sandbox check result. If checkErr is nil, no
// permission is needed. If it's errNeedsPermission, the ask callback is called
// (or denyMsg is returned when ask is nil). Other errors are returned as-is.
func (s *Sandbox) requirePermission(checkErr error, ask func(string, string) error, tc provider.ToolCall, denyMsg string) error {
	if checkErr == nil {
		return nil
	}
	if checkErr != errNeedsPermission {
		return checkErr
	}
	if ask == nil {
		return errors.New(denyMsg)
	}
	return ask(tc.Name, tc.Arguments)
}

func (s *Sandbox) requirePermissionDecision(decision permissionDecision, ask func(string, string) error, tc provider.ToolCall, fallbackDenyMsg string) error {
	if !decision.enforce || decision.action == permission.ActionAllow {
		return nil
	}
	if decision.action == permission.ActionDeny {
		if decision.denyMsg != "" {
			return errors.New(decision.denyMsg)
		}
		return errors.New(fallbackDenyMsg)
	}
	if ask == nil {
		if decision.denyMsg != "" {
			return errors.New(decision.denyMsg)
		}
		return errors.New(fallbackDenyMsg)
	}
	return ask(tc.Name, tc.Arguments)
}

func (s *Sandbox) IsReadOnly(name string) bool {
	return s.registry.IsReadOnly(name)
}

var errDeniedDirectory = fmt.Errorf("access denied: requesting access to the home or root directory is always denied. Use paths within the project directory")

func (s *Sandbox) confinePath(raw string) (string, error) {
	var abs string
	if filepath.IsAbs(raw) {
		abs = raw
	} else {
		abs = filepath.Join(s.projectDir, raw)
	}
	resolved := resolvePathAllowMissing(abs)
	if s.isDeniedDirectory(resolved) {
		return "", errDeniedDirectory
	}
	if safeDevPaths[resolved] {
		return resolved, nil
	}
	if s.isAllowedPath(resolved) {
		return resolved, nil
	}
	rel, err := filepath.Rel(strings.ToLower(s.projectDir), strings.ToLower(resolved))
	if err != nil || escapesBase(rel) {
		if filepath.IsAbs(raw) {
			stripped := strings.TrimPrefix(raw, "/")
			joined := filepath.Join(s.projectDir, stripped)
			if _, statErr := os.Stat(joined); statErr == nil {
				jResolved := resolvePathAllowMissing(joined)
				jRel, jRelErr := filepath.Rel(strings.ToLower(s.projectDir), strings.ToLower(jResolved))
				if jRelErr == nil && !escapesBase(jRel) {
					return jResolved, nil
				}
			}
		}
		return "", fmt.Errorf("path %q is outside project directory", raw)
	}
	return resolved, nil
}

func (s *Sandbox) isDeniedDirectory(resolved string) bool {
	home, _ := os.UserHomeDir()
	switch resolved {
	case "/", home:
		return true
	}
	return false
}

func (s *Sandbox) resolvePathArg(argsJSON, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON
	}
	raw, ok := m[field]
	if !ok {
		return argsJSON
	}
	pathStr, ok := raw.(string)
	if !ok {
		return argsJSON
	}
	if !filepath.IsAbs(pathStr) {
		pathStr = filepath.Join(s.projectDir, pathStr)
	}
	pathStr = resolvePathAllowMissing(pathStr)
	m[field] = pathStr
	out, err := json.Marshal(m)
	if err != nil {
		return argsJSON
	}
	return string(out)
}

func (s *Sandbox) confinePathArg(argsJSON, field string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON, nil
	}
	raw, ok := m[field]
	if !ok {
		return argsJSON, nil
	}
	pathStr, ok := raw.(string)
	if !ok {
		return argsJSON, nil
	}
	confined, err := s.confinePath(pathStr)
	if err != nil {
		return "", err
	}
	m[field] = confined
	out, err := json.Marshal(m)
	if err != nil {
		return argsJSON, nil
	}
	return string(out), nil
}

func resolvePathAllowMissing(path string) string {
	candidate := filepath.Clean(path)
	if candidate == "" {
		return candidate
	}
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			return joinResolvedSuffix(resolved, suffix)
		}
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(candidate)
			if err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(candidate), target)
				}
				return joinResolvedSuffix(filepath.Clean(target), suffix)
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return filepath.Clean(path)
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}

func joinResolvedSuffix(base string, suffix []string) string {
	resolved := base
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return filepath.Clean(resolved)
}

var pathEnvVars = map[string]bool{
	"HOME": true, "TMPDIR": true, "TEMP": true, "TMP": true,
	"PWD": true, "OLDPWD": true, "GOPATH": true, "GOROOT": true,
	"XDG_DATA_HOME": true, "XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true,
}

var contentCmds = map[string]bool{
	"echo": true, "printf": true, "print": true,
}

func wordIsQuoted(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, part := range w.Parts {
		switch part.(type) {
		case *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		}
	}
	return false
}

func wordText(w *syntax.Word) string {
	if w == nil || len(w.Parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, dp := range p.Parts {
				if lit, ok := dp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				} else {
					return ""
				}
			}
		default:
			return ""
		}
	}
	return sb.String()
}

func wordHasPathEnvVar(w *syntax.Word) bool {
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.ParamExp:
			if pathEnvVars[p.Param.Value] {
				return true
			}
		case *syntax.DblQuoted:
			for _, dp := range p.Parts {
				if pe, ok := dp.(*syntax.ParamExp); ok && pathEnvVars[pe.Param.Value] {
					return true
				}
			}
		}
	}
	return false
}

func pathOrParentExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	parent := filepath.Dir(path)
	if parent == path {
		return false
	}
	_, err := os.Stat(parent)
	return err == nil
}

func (s *Sandbox) isAllowedPath(resolved string) bool {
	for _, ap := range s.allowedPaths {
		if strings.HasPrefix(resolved, ap+string(filepath.Separator)) || resolved == ap {
			return true
		}
	}
	return false
}

func (s *Sandbox) isExternalPath(resolved string) bool {
	rel, err := filepath.Rel(strings.ToLower(s.projectDir), strings.ToLower(resolved))
	return err != nil || escapesBase(rel)
}

func containsDotDot(tok string) bool {
	for seg := range strings.SplitSeq(filepath.ToSlash(tok), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// normalizePathField rewrites common aliases for the expected field name in
// raw JSON tool arguments (e.g. "file" → "filePath"). This handles models
// that send variant field names instead of the canonical schema name.
func normalizePathField(argsJSON, canonical string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return argsJSON
	}
	if _, ok := m[canonical]; ok {
		return argsJSON
	}
	aliases := map[string][]string{
		"filePath": {"file", "file_path", "filename", "filepath"},
		"path":     {"directory", "dir", "folder"},
	}
	for _, alias := range aliases[canonical] {
		if v, ok := m[alias]; ok {
			m[canonical] = v
			delete(m, alias)
			if out, err := json.Marshal(m); err == nil {
				return string(out)
			}
			return argsJSON
		}
	}
	return argsJSON
}

func extractStringArg(argsJSON, field string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return "", false
	}
	v, ok := m[field].(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func combinePermissionDecisions(decisions ...permissionDecision) permissionDecision {
	combined := permissionDecision{}
	for _, decision := range decisions {
		if !decision.enforce {
			continue
		}
		combined.enforce = true
		if decision.action == permission.ActionDeny {
			return decision
		}
		if decision.action == permission.ActionAsk {
			combined.action = permission.ActionAsk
			if combined.denyMsg == "" {
				combined.denyMsg = decision.denyMsg
			}
			continue
		}
		if combined.action == "" {
			combined.action = permission.ActionAllow
		}
		if combined.denyMsg == "" {
			combined.denyMsg = decision.denyMsg
		}
	}
	return combined
}

func (s *Sandbox) toolPermissionDecision(tc provider.ToolCall) permissionDecision {
	switch tc.Name {
	case "bash":
		cmd, ok := extractStringArg(tc.Arguments, "command")
		return s.permissionDecisionForRule(tc.Name, cmd, tc.Name+" is denied by configuration", ok)
	case "git":
		return s.permissionDecisionForRule(tc.Name, gitPermissionKey(tc.Arguments), tc.Name+" is denied by configuration", true)
	case "web_fetch":
		url, ok := extractStringArg(tc.Arguments, "url")
		return s.permissionDecisionForRule(tc.Name, url, tc.Name+" is denied by configuration", ok)
	default:
		field, ok := pathTools[tc.Name]
		if !ok {
			return permissionDecision{}
		}
		path, _ := extractStringArg(tc.Arguments, field)
		return s.toolPermissionDecisionForPath(tc.Name, path)
	}
}

func (s *Sandbox) toolPermissionDecisionForPath(toolName, path string) permissionDecision {
	return s.permissionDecisionForRule(toolName, path, toolName+" is denied by configuration", path != "")
}

func (s *Sandbox) permissionDecisionForRule(toolName, keyArg, denyMsg string, keyKnown bool) permissionDecision {
	if s.permission == nil {
		return permissionDecision{}
	}
	if _, ok := s.permission[toolName]; !ok {
		return permissionDecision{}
	}
	if !keyKnown {
		keyArg = ""
	}
	return permissionDecision{
		action:  permission.Resolve(s.permission, toolName, keyArg),
		enforce: true,
		denyMsg: denyMsg,
	}
}

func (s *Sandbox) externalDirectoryDecision(path string) permissionDecision {
	action := permission.ActionAsk
	if s.permission != nil {
		if _, ok := s.permission["external_directory"]; ok {
			action = permission.Resolve(s.permission, "external_directory", path)
		}
	}
	return permissionDecision{
		action:  action,
		enforce: true,
		denyMsg: "external path access is denied by configuration",
	}
}

func (s *Sandbox) externalPathDecision(path string) permissionDecision {
	if path != "" && s.isDeniedDirectory(path) {
		return permissionDecision{
			action:  permission.ActionDeny,
			enforce: true,
			denyMsg: "access denied: requesting access to the home or root directory is always denied. Use paths within the project directory",
		}
	}
	if path != "" && s.isAllowedPath(path) {
		return permissionDecision{action: permission.ActionAllow, enforce: true}
	}
	return s.externalDirectoryDecision(path)
}

func escapesBase(rel string) bool {
	if rel == ".." {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// checkDestructiveGit detects destructive git operations (force push, hard
// reset, clean -f, checkout --) in both the git tool and bash commands.
func checkDestructiveGit(toolName, argsJSON string) error {
	var m map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return nil
	}

	var cmd string
	switch toolName {
	case "git":
		action := m["action"]
		args := m["args"]
		if action != "" {
			cmd = "git " + action + " " + args
		}
	case "bash":
		cmd = m["command"]
	default:
		return nil
	}

	if cmd != "" && tool.IsDestructiveGitCommand(cmd) {
		return errNeedsPermission
	}
	return nil
}

func checkBashGitMutation(argsJSON string) error {
	var m map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return nil
	}
	if tool.BashHasMutatingGitCommand(m["command"]) {
		return errBashGitMutationDenied
	}
	return nil
}

func gitPermissionKey(argsJSON string) string {
	var m map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return ""
	}
	action := strings.TrimSpace(m["action"])
	args := strings.TrimSpace(m["args"])
	switch {
	case action == "" && args == "":
		return ""
	case args == "":
		return "git " + action
	case action == "":
		return "git " + args
	default:
		return "git " + action + " " + args
	}
}

func (s *Sandbox) bashOutsidePathDecision(argsJSON string) (permissionDecision, string) {
	analysis := s.analyzeBashOutsidePath(argsJSON)
	if !analysis.ambiguous && len(analysis.externalPaths) == 0 {
		return permissionDecision{}, argsJSON
	}
	var decisions []permissionDecision
	if analysis.ambiguous {
		decisions = append(decisions, permissionDecision{
			action:  permission.ActionAsk,
			enforce: true,
			denyMsg: "bash command uses dynamic shell evaluation and requires permission",
		})
	}
	for _, path := range analysis.externalPaths {
		decisions = append(decisions, s.externalPathDecision(path))
	}
	return combinePermissionDecisions(decisions...), augmentPermissionDisplay(argsJSON, analysis)
}

func (s *Sandbox) checkBashDelete(argsJSON string) error {
	var m map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return nil
	}
	if !rmPattern.MatchString(m["command"]) {
		return nil
	}
	action := permission.ActionAsk
	if s.permission != nil {
		if rule, ok := s.permission["bash_rm"]; ok && rule != nil {
			action = rule.Action
		}
	}
	switch action {
	case permission.ActionAllow:
		return nil
	case permission.ActionDeny:
		return fmt.Errorf("bash rm/rmdir is disabled by configuration")
	default:
		return errNeedsPermission
	}
}

func (s *Sandbox) analyzeBashOutsidePath(argsJSON string) bashPathAnalysis {
	var params struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return bashPathAnalysis{}
	}

	analysis := bashPathAnalysis{}
	addPath := func(path, label string) {
		if path == "" {
			return
		}
		for _, existing := range analysis.externalPaths {
			if existing == path {
				return
			}
		}
		analysis.externalPaths = append(analysis.externalPaths, path)
		if label != "" {
			analysis.displayLines = append(analysis.displayLines, label)
			return
		}
		analysis.displayLines = append(analysis.displayLines, path)
	}

	if params.Workdir != "" {
		workdir := params.Workdir
		if !filepath.IsAbs(workdir) {
			workdir = filepath.Join(s.projectDir, workdir)
		}
		workdir = resolvePathAllowMissing(workdir)
		if s.isDeniedDirectory(workdir) || (s.isExternalPath(workdir) && !s.isAllowedPath(workdir)) {
			addPath(workdir, "workdir: "+workdir)
		}
	}

	if params.Command == "" {
		return analysis
	}

	prog, err := syntax.NewParser().Parse(strings.NewReader(params.Command), "")
	if err != nil {
		analysis.ambiguous = true
		if len(analysis.displayLines) == 0 {
			analysis.displayLines = append(analysis.displayLines, params.Command)
		}
		return analysis
	}

	home, _ := os.UserHomeDir()
	syntax.Walk(prog, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			for _, redir := range n.Redirs {
				if path, ok, ambiguous := s.externalPathFromWord(redir.Word, home, true); ok {
					addPath(path, "")
				} else if ambiguous {
					analysis.ambiguous = true
				}
			}
			return true
		case *syntax.CallExpr:
			for _, assign := range n.Assigns {
				if path, ok, ambiguous := s.externalPathFromAssign(assign, home); ok {
					addPath(path, "")
				} else if ambiguous {
					analysis.ambiguous = true
				}
			}
			cmdName, cmdIdx, ok := shellCallCommand(n.Args)
			if !ok {
				if len(n.Args) > 0 && wordHasDynamicParts(n.Args[0]) {
					analysis.ambiguous = true
				}
				return false
			}
			if shellCommandNeedsPermissionReview(cmdName, n.Args[cmdIdx+1:]) {
				analysis.ambiguous = true
				return false
			}
			if contentCmds[cmdName] {
				return false
			}
			for i := cmdIdx + 1; i < len(n.Args); i++ {
				arg := n.Args[i]
				if path, ok, ambiguous := s.externalPathFromWord(arg, home, false); ok {
					addPath(path, "")
				} else if ambiguous {
					analysis.ambiguous = true
				}
			}
			return false
		}
		return true
	})

	return analysis
}

func (s *Sandbox) externalPathFromAssign(assign *syntax.Assign, home string) (string, bool, bool) {
	if assign == nil || assign.Value == nil {
		return "", false, false
	}
	if path, ok, ambiguous := s.externalPathFromWord(assign.Value, home, false); ok {
		return path, true, false
	} else if ambiguous {
		return "", false, true
	}

	text := wordText(assign.Value)
	if text == "" {
		return "", false, false
	}
	if !filepath.IsAbs(text) {
		text = filepath.Join(s.projectDir, text)
	}
	resolved := resolvePathAllowMissing(text)
	if s.isExternalPath(resolved) || s.isDeniedDirectory(resolved) {
		return resolved, true, false
	}
	return "", false, false
}

func (s *Sandbox) externalPathFromWord(w *syntax.Word, home string, redirect bool) (string, bool, bool) {
	text := wordText(w)
	if text == "" {
		if wordHasDynamicParts(w) || wordHasPathEnvVar(w) {
			return "", false, true
		}
		return "", false, false
	}

	expanded := text
	if strings.HasPrefix(text, "~/") && home != "" {
		expanded = filepath.Join(home, text[2:])
	} else if text == "~" && home != "" {
		expanded = home
	}

	if filepath.IsAbs(expanded) {
		resolved := resolvePathAllowMissing(expanded)
		if safeDevPaths[resolved] {
			return "", false, false
		}
		if redirect {
			if !pathOrParentExists(resolved) {
				return "", false, false
			}
		} else if quoted := wordIsQuoted(w); quoted {
			if _, err := os.Stat(resolved); err != nil {
				return "", false, false
			}
		} else if !pathOrParentExists(resolved) {
			return "", false, false
		}
		if s.isExternalPath(resolved) {
			return resolved, true, false
		}
		return "", false, false
	}

	if containsDotDot(text) {
		resolved := resolvePathAllowMissing(filepath.Join(s.projectDir, text))
		if s.isExternalPath(resolved) {
			return resolved, true, false
		}
	}
	if path, ok := externalPathFromAssignment(text, home); ok {
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.projectDir, path)
		}
		resolved := resolvePathAllowMissing(path)
		if s.isExternalPath(resolved) || s.isDeniedDirectory(resolved) {
			return resolved, true, false
		}
	}
	if path, ok := externalPathFromFlagValue(text, home); ok {
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.projectDir, path)
		}
		resolved := resolvePathAllowMissing(path)
		if s.isExternalPath(resolved) || s.isDeniedDirectory(resolved) {
			return resolved, true, false
		}
	}

	return "", false, false
}

func externalPathFromAssignment(tok, home string) (string, bool) {
	idx := strings.IndexByte(tok, '=')
	if idx <= 0 || idx >= len(tok)-1 {
		return "", false
	}
	return externalPathLikeValue(tok[idx+1:], home)
}

func externalPathFromFlagValue(tok, home string) (string, bool) {
	if !strings.HasPrefix(tok, "-") {
		return "", false
	}
	idx := strings.IndexByte(tok, '=')
	if idx <= 0 || idx >= len(tok)-1 {
		return "", false
	}
	return externalPathLikeValue(tok[idx+1:], home)
}

func externalPathLikeValue(value, home string) (string, bool) {
	switch {
	case strings.HasPrefix(value, "~/") && home != "":
		return filepath.Join(home, value[2:]), true
	case value == "~" && home != "":
		return home, true
	case filepath.IsAbs(value):
		return value, true
	case strings.HasPrefix(value, "../"), strings.HasPrefix(value, "./"):
		return value, true
	default:
		return "", false
	}
}

func shellCallCommand(args []*syntax.Word) (string, int, bool) {
	if len(args) == 0 {
		return "", 0, false
	}
	i := 0
	cmd := wordText(args[i])
	if cmd == "" {
		return "", 0, false
	}
	if filepath.Base(cmd) == "env" {
		i++
		for i < len(args) && isShellEnvAssignment(wordText(args[i])) {
			i++
		}
		if i >= len(args) {
			return "", 0, false
		}
		cmd = wordText(args[i])
		if cmd == "" {
			return "", 0, false
		}
	}
	return filepath.Base(cmd), i, true
}

func isShellEnvAssignment(tok string) bool {
	if tok == "" {
		return false
	}
	idx := strings.IndexByte(tok, '=')
	if idx <= 0 {
		return false
	}
	for i := 0; i < idx; i++ {
		c := tok[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9' || i == 0) && c != '_' {
			return false
		}
	}
	return true
}

func shellCommandNeedsPermissionReview(cmd string, args []*syntax.Word) bool {
	switch cmd {
	case "eval", "source", ".":
		return true
	case "xargs":
		return true
	case "awk":
		return awkCommandNeedsPermissionReview(args)
	case "sed":
		return sedCommandNeedsPermissionReview(args)
	case "sh", "bash", "zsh":
		for _, arg := range args {
			switch wordText(arg) {
			case "-c", "-lc", "-cl", "-ic", "-ci":
				return true
			}
		}
	case "python", "python3", "node", "perl", "ruby", "php":
		for _, arg := range args {
			switch wordText(arg) {
			case "-c", "-e", "-r":
				return true
			}
		}
	}
	return false
}

func awkCommandNeedsPermissionReview(args []*syntax.Word) bool {
	for _, arg := range args {
		text := wordText(arg)
		if text == "" {
			continue
		}
		if strings.Contains(text, "system(") {
			return true
		}
		if awkPathScriptPattern.MatchString(text) {
			return true
		}
	}
	return false
}

func sedCommandNeedsPermissionReview(args []*syntax.Word) bool {
	for _, arg := range args {
		text := wordText(arg)
		if text == "" {
			continue
		}
		if sedPathScriptPattern.MatchString(text) || sedExecScriptPattern.MatchString(text) {
			return true
		}
	}
	return false
}

func wordHasDynamicParts(w *syntax.Word) bool {
	if w == nil || len(w.Parts) == 0 {
		return false
	}
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit, *syntax.SglQuoted:
			continue
		case *syntax.DblQuoted:
			for _, dp := range p.Parts {
				if _, ok := dp.(*syntax.Lit); !ok {
					return true
				}
			}
		default:
			return true
		}
	}
	return false
}

func augmentPermissionDisplay(argsJSON string, analysis bashPathAnalysis) string {
	if len(analysis.externalPaths) == 0 && len(analysis.displayLines) == 0 && !analysis.ambiguous {
		return argsJSON
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON
	}
	command, _ := m["command"].(string)
	if len(analysis.externalPaths) > 0 {
		paths := make([]string, len(analysis.externalPaths))
		copy(paths, analysis.externalPaths)
		m["permission_paths"] = paths
	}
	switch {
	case command != "" && len(analysis.displayLines) > 0:
		m["permission_display"] = command + "\n\n" + strings.Join(analysis.displayLines, "\n")
	case command != "":
		m["permission_display"] = command
	case len(analysis.displayLines) > 0:
		m["permission_display"] = strings.Join(analysis.displayLines, "\n")
	case analysis.ambiguous:
		m["permission_display"] = "dynamic shell command"
	}
	out, err := json.Marshal(m)
	if err != nil {
		return argsJSON
	}
	return string(out)
}
