// Package tool provides the tool types, execution context, and truncation
// helpers used by the session service to dispatch AI tool calls.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/skills"
)

// flexUnmarshal unmarshals JSON into dst (must be a pointer to a struct),
// tolerating type mismatches that weaker models produce. It handles:
//   - string-wrapped numbers: "100" → int/float64
//   - string-wrapped booleans: "true" → bool
//   - string-wrapped arrays: "[{...}]" → []T
//   - single object where array expected: {...} → []T (wrapped in a slice)
//
// Strict unmarshal is tried first. Well-formed input takes the fast path.
func flexUnmarshal(data []byte, dst any) error {
	if err := json.Unmarshal(data, dst); err == nil {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}

	v := reflect.ValueOf(dst).Elem()
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		rawVal, ok := raw[name]
		if !ok {
			continue
		}

		fv := v.Field(i)
		// Try direct unmarshal first.
		ptr := reflect.New(field.Type)
		if json.Unmarshal(rawVal, ptr.Interface()) == nil {
			fv.Set(ptr.Elem())
			continue
		}

		// Coerce from string.
		var s string
		if json.Unmarshal(rawVal, &s) != nil {
			// If it is not a string, try object-to-slice coercion for slice fields.
			if field.Type.Kind() == reflect.Slice {
				elem := reflect.New(field.Type.Elem())
				if json.Unmarshal(rawVal, elem.Interface()) == nil {
					sl := reflect.MakeSlice(field.Type, 1, 1)
					sl.Index(0).Set(elem.Elem())
					fv.Set(sl)
				}
			}
			continue
		}

		// Resolve the target kind, dereferencing pointers.
		targetType := field.Type
		isPtr := targetType.Kind() == reflect.Pointer
		if isPtr {
			targetType = targetType.Elem()
		}

		var coerced reflect.Value
		switch targetType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				coerced = reflect.New(targetType).Elem()
				coerced.SetInt(n)
			}
		case reflect.Float32, reflect.Float64:
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				coerced = reflect.New(targetType).Elem()
				coerced.SetFloat(f)
			}
		case reflect.Bool:
			if b, err := strconv.ParseBool(s); err == nil {
				coerced = reflect.New(targetType).Elem()
				coerced.SetBool(b)
			}
		case reflect.Slice:
			// Try parsing string content as JSON array.
			sl := reflect.New(field.Type)
			if json.Unmarshal([]byte(s), sl.Interface()) == nil {
				fv.Set(sl.Elem())
				continue
			}
		}

		if coerced.IsValid() {
			if isPtr {
				p := reflect.New(targetType)
				p.Elem().Set(coerced)
				fv.Set(p)
			} else {
				fv.Set(coerced)
			}
		}
	}
	return nil
}

// MaxLines is the maximum number of lines before truncation.
const MaxLines = 200

// MaxBytes is the maximum number of bytes before truncation.
const MaxBytes = 16 * 1024

// ErrDenied is returned by ExecutionContext.Ask when the user denies a tool call.
var ErrDenied = errors.New("tool call denied")

type TaskSessionKey struct{}

// EphemeralKey is the context key marking a session as ephemeral (subagent).
// Tools use this to apply tighter limits.
type EphemeralKey struct{}

// TaskSessionIDFromContext returns the parent task session ID from ctx, or "".
func TaskSessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(TaskSessionKey{}).(string); ok {
		return v
	}
	return ""
}

// contextUsageKey is the context key for passing context window usage to tools.
type contextUsageKey struct{}

type skillPolicyKey struct{}

// WithContextUsage stores the context window usage fraction in ctx.
func WithContextUsage(ctx context.Context, usage float64) context.Context {
	return context.WithValue(ctx, contextUsageKey{}, usage)
}

// ContextUsageFromContext retrieves the context window usage fraction from ctx.
// Returns 0 if not set.
func ContextUsageFromContext(ctx context.Context) float64 {
	if v, ok := ctx.Value(contextUsageKey{}).(float64); ok {
		return v
	}
	return 0
}

// WithSkillPolicy stores the resolved skill access policy in ctx.
func WithSkillPolicy(ctx context.Context, policy skills.AccessPolicy) context.Context {
	return context.WithValue(ctx, skillPolicyKey{}, policy)
}

// SkillPolicyFromContext retrieves the resolved skill access policy from ctx.
func SkillPolicyFromContext(ctx context.Context) skills.AccessPolicy {
	if v, ok := ctx.Value(skillPolicyKey{}).(skills.AccessPolicy); ok {
		return v
	}
	return skills.AccessPolicy{AllowAll: true}
}

type ExecutionContext struct {
	// SessionID identifies the session owning this tool call.
	SessionID string
	// ContextUsage is the fraction [0.0, 1.0] of the model's context window
	// currently consumed. Tools use this to adaptively reduce output limits
	// when context pressure is high, avoiding expensive compaction cycles.
	ContextUsage float64
	// TaskSessionID overrides the session ID used for task storage.
	// When set (e.g. by subagents), tasks are created on the parent session
	// so they persist after the subagent's ephemeral session is deleted.
	TaskSessionID string
	// IgnorePatterns is a list of glob patterns for directories/files that
	// tools should skip when scanning (e.g. "node_modules/**", ".git/**").
	IgnorePatterns []string
	// WorkDir is the working directory for filesystem operations.
	// If empty, tools default to os.Getwd().
	WorkDir string
	// Ephemeral is true for subagent sessions. Tools may use this to apply
	// tighter limits (e.g., lower default read line count).
	Ephemeral bool
	// Abort is closed when the session is cancelled.
	// Tools should select on this channel to support early termination.
	Abort <-chan struct{}
	// Ask checks permission before executing. It blocks until the user
	// grants or denies. Returns nil to proceed, ErrDenied to abort.
	// nil means all calls are allowed automatically.
	Ask func(toolName, input string) error
	// WriteOutput streams incremental output chunks to the caller.
	// nil means output is only returned in the final Result.
	WriteOutput func(chunk string)
	// AskUser presents a question with options to the user and blocks until
	// they respond. Returns the selected answer(s) as a string.
	// nil means user interaction is not available.
	AskUser func(question string, options []string, multiple bool, purpose string) (string, error)
	// SpawnSubagent creates a subagent session with the given agent ID and task,
	// runs the conversation to completion, and returns the final assistant response.
	// nil means subagent spawning is not available.
	SpawnSubagent func(ctx context.Context, agentID, task string) (string, error)
	// OnBackgroundDone is called when a background bash task completes.
	// nil means no notification is sent.
	OnBackgroundDone func(task *BashTask)
	// SkillDirs lists directories to search for skill subdirectories.
	// Each entry is scanned for <name>/SKILL.md files.
	SkillDirs []string
	// SkillPolicy is the resolved model/agent access policy for skills.
	SkillPolicy skills.AccessPolicy
}

// IsIgnored checks if a relative path matches any of the ignore patterns.
// For directory checks, pass the path with a trailing "/" (e.g. "node_modules/").
func (ectx ExecutionContext) IsIgnored(relPath string) bool {
	// Normalize: remove trailing slash for matching, but also check the
	// directory name against "dir/**" patterns.
	clean := strings.TrimSuffix(relPath, "/")
	for _, pattern := range ectx.IgnorePatterns {
		if matched, _ := doublestar.Match(pattern, relPath); matched {
			return true
		}
		if matched, _ := doublestar.Match(pattern, clean); matched {
			return true
		}
		// "node_modules/**" should skip the "node_modules" directory itself.
		dirPattern := strings.TrimSuffix(pattern, "/**")
		if dirPattern != pattern && clean == dirPattern {
			return true
		}
	}
	return false
}

// Error code constants for structured tool error classification.
const (
	ErrCodeInvalidArgs = "invalid_args"
	ErrCodeNotFound    = "not_found"
	ErrCodePermission  = "permission"
	ErrCodeTimeout     = "timeout"
	ErrCodeInternal    = "internal"
	ErrCodeConflict    = "conflict"
	ErrCodeUnavailable = "unavailable"
	ErrCodeCancelled   = "cancelled"
)

// Result is the output of a tool execution.
type Result struct {
	Title     string         `json:"title"`
	Output    string         `json:"output"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	ErrorCode string         `json:"error_code,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
}

// ErrorResult creates a Result representing a classified tool error.
// Tools return (ErrorResult(...), nil) for expected errors like missing
// arguments or not-found resources. Go errors are reserved for truly
// unexpected failures.
func ErrorResult(code, message string, retryable bool) *Result {
	return &Result{Output: message, ErrorCode: code, Retryable: retryable}
}

// Tool is a single function the AI model can call.
type Tool struct {
	Name        string
	Description string
	Parameters  []byte
	PromptHints provider.ToolPromptHints
	ReadOnly    bool // true if this tool never modifies files or state
	IsMCP       bool // true for tools provided by MCP servers
	Execute     func(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error)
}

// TruncateResult is the output of Truncate.
type TruncateResult struct {
	// Content is the (possibly truncated) text.
	Content string
	// Truncated reports whether the original text exceeded MaxLines or MaxBytes.
	Truncated bool
}

// EffectiveMaxLines returns MaxLines scaled down when context usage exceeds 50%.
// Linear scale: 100% at ≤50% usage, 25% at ≥80% usage.
func EffectiveMaxLines(contextUsage float64) int {
	if contextUsage < 0.5 {
		return MaxLines
	}
	scale := 1.0 - (contextUsage-0.5)/0.3*0.75
	if scale < 0.25 {
		scale = 0.25
	}
	n := max(int(float64(MaxLines)*scale), 50)
	return n
}

// EffectiveMaxBytes returns MaxBytes scaled down when context usage exceeds 50%.
func EffectiveMaxBytes(contextUsage float64) int {
	if contextUsage < 0.5 {
		return MaxBytes
	}
	scale := 1.0 - (contextUsage-0.5)/0.3*0.75
	if scale < 0.25 {
		scale = 0.25
	}
	n := max(int(float64(MaxBytes)*scale), 4096)
	return n
}

// TruncateWithBudget truncates output using adaptive limits based on context usage.
// When contextUsage is 0 (unknown), falls back to the static MaxLines/MaxBytes.
func TruncateWithBudget(text, direction string, contextUsage float64) TruncateResult {
	return truncateWithLimits(text, direction, EffectiveMaxLines(contextUsage), EffectiveMaxBytes(contextUsage))
}

// Truncate truncates output that exceeds MaxLines or MaxBytes.
// When truncated, the full output is saved to a temp file.
// direction is "head" (keep first) or "tail" (keep last).
// Truncate truncates using the static MaxLines/MaxBytes limits.
func Truncate(text, direction string) TruncateResult {
	return truncateWithLimits(text, direction, MaxLines, MaxBytes)
}

func truncateWithLimits(text, direction string, maxLines, maxBytes int) TruncateResult {
	if direction == "" {
		direction = "head"
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines && len(text) <= maxBytes {
		return TruncateResult{Content: text}
	}
	totalLines := len(lines)
	var truncLines []string
	if len(lines) > maxLines {
		if direction == "tail" {
			truncLines = lines[len(lines)-maxLines:]
		} else {
			truncLines = lines[:maxLines]
		}
	} else {
		truncLines = lines
	}
	content := strings.Join(truncLines, "\n")
	if len(content) > maxBytes {
		if direction == "tail" {
			s := content[len(content)-maxBytes:]
			// Advance past any partial rune at the start.
			for len(s) > 0 {
				r, _ := utf8.DecodeRuneInString(s)
				if r != utf8.RuneError {
					break
				}
				s = s[1:]
			}
			content = s
		} else {
			s := content[:maxBytes]
			// Walk back to avoid splitting a multi-byte rune.
			for len(s) > 0 {
				if utf8.ValidString(s) {
					break
				}
				s = s[:len(s)-1]
			}
			content = s
		}
	}
	hint := fmt.Sprintf("\n\n[output truncated — showing %d of %d lines]", len(truncLines), totalLines)
	return TruncateResult{
		Content:   content + hint,
		Truncated: true,
	}
}
