package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// AnswerResponse is the response type for permission questions.
type AnswerResponse string

const (
	AnswerOnce   AnswerResponse = "once"
	AnswerAlways AnswerResponse = "always"
	AnswerReject AnswerResponse = "reject"
)

// pendingQuestion holds state for an unanswered permission question.
type pendingQuestion struct {
	ch        chan error
	sessionID string
	tool      string
	patterns  []string
}

type pendingUserQuestion struct {
	ch        chan string
	sessionID string
}

// AskPermission returns an AskPermissionFunc that wires tool permission requests
// through the SSE question/answer flow. Used by chain construction.
// Denials are recorded by path. Subsequent tool calls to the same path (or
// children) are auto-denied without prompting.
func (s *SessionService) AskPermission() AskPermissionFunc {
	return func(ctx context.Context, sessionID, toolName, input string) func(string, string) error {
		parentSessionID := tool.TaskSessionIDFromContext(ctx)
		approved := false
		denySessionID := sessionID
		if parentSessionID != "" {
			denySessionID = parentSessionID
		}
		return func(tn, inp string) error {
			if approved {
				return nil
			}
			projectDir := ""
			if s != nil && s.runtime != nil {
				projectDir = s.runtime.projectDir
			}
			pathSubjects := permissionPathSubjectsFromInput(inp, projectDir)
			for _, pathSubject := range pathSubjects {
				if s.state.permissions.IsPathDenied(sessionID, parentSessionID, pathSubject) {
					return tool.ErrDenied
				}
			}

			approvalPatterns := s.permissionApprovalPatterns(tn, inp, projectDir)
			if s.state.permissions.IsApproved(sessionID, parentSessionID, tn, approvalPatterns) {
				approved = true
				return nil
			}

			questionID := fmt.Sprintf("q-%s-%d", sessionID, time.Now().UnixNano())
			err := s.askPermission(ctx, sessionID, questionID, tn, inp, s.permissionSuggestedPattern(tn, inp, projectDir), approvalPatterns)
			if err == tool.ErrDenied {
				for _, pathSubject := range pathSubjects {
					s.state.permissions.DenyPath(denySessionID, pathSubject)
				}
			}
			if err == nil {
				approved = true
			}
			return err
		}
	}
}

// extractPathFromInput extracts a file or directory path from tool arguments JSON.
// Checks common field names used by path-targeting tools and bash commands.
func extractPathFromInput(input string) string {
	paths := extractPathsFromInput(input)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func extractPathsFromInput(input string) []string {
	var fields map[string]any
	if json.Unmarshal([]byte(input), &fields) != nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	addPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	if paths, ok := fields["permission_paths"].([]any); ok {
		for _, raw := range paths {
			if path, ok := raw.(string); ok {
				addPath(path)
			}
		}
	}
	for _, key := range []string{"filePath", "file_path", "path"} {
		if v, ok := fields[key].(string); ok {
			addPath(v)
		}
	}
	// For bash commands, extract absolute paths from the command string.
	// Uses ShellTokens to handle quoted paths with spaces.
	if cmd, ok := fields["command"].(string); ok {
		for _, tok := range sandbox.ShellTokens(cmd) {
			if filepath.IsAbs(tok) {
				addPath(tok)
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(tok, "~/") && home != "" {
				addPath(filepath.Join(home, tok[2:]))
			}
		}
	}
	return out
}

func permissionPathSubjectsFromInput(input, projectDir string) []string {
	rawPaths := extractPathsFromInput(input)
	var out []string
	seen := make(map[string]bool)
	for _, rawPath := range rawPaths {
		subject := permissionPathSubject(rawPath, projectDir)
		if subject == "" || seen[subject] {
			continue
		}
		seen[subject] = true
		out = append(out, subject)
	}
	return out
}

func (s *SessionService) permissionApprovalPatterns(toolName, input, projectDir string) []string {
	pathSubjects := permissionPathSubjectsFromInput(input, projectDir)
	if len(pathSubjects) > 0 {
		return pathSubjects
	}
	subject := s.permissionSubject(toolName, input)
	if subject == "" {
		return nil
	}
	return []string{subject}
}

func (s *SessionService) permissionSuggestedPattern(toolName, input, projectDir string) string {
	pathSubjects := permissionPathSubjectsFromInput(input, projectDir)
	switch len(pathSubjects) {
	case 0:
		return s.permissionSubject(toolName, input)
	case 1:
		return pathSubjects[0]
	default:
		return strings.Join(pathSubjects, "\n")
	}
}

// AskUser returns an AskUserFunc that wires user questions through the SSE
// question/answer flow. Used by chain construction.
func (s *SessionService) AskUser() AskUserFunc {
	return func(ctx context.Context, sessionID string) func(string, []string, bool, string) (string, error) {
		return func(question string, options []string, multiple bool, purpose string) (string, error) {
			questionID := fmt.Sprintf("uq-%s-%d", sessionID, time.Now().UnixNano())
			log.Printf("askUser: publishing user_question %s for session %s: %q (%d options) purpose=%q", questionID, sessionID, question, len(options), purpose)
			ch := make(chan string, 1)

			s.state.permissions.SetPendingUserQuestion(questionID, pendingUserQuestion{ch: ch, sessionID: sessionID})

			s.publish(sessionID, SSEEvent{
				Type: "user_question",
				Data: SSEUserQuestionData{
					QuestionID: questionID,
					Question:   question,
					Options:    options,
					Multiple:   multiple,
					Purpose:    purpose,
				},
			})

			defer func() {
				s.state.permissions.DeletePendingUserQuestion(questionID)
			}()

			select {
			case answer := <-ch:
				log.Printf("askUser: question %s answered: %q", questionID, answer)
				return answer, nil
			case <-ctx.Done():
				log.Printf("askUser: question %s cancelled (ctx done): %v", questionID, ctx.Err())
				return "", ctx.Err()
			}
		}
	}
}

// Answer resolves a pending permission question or user question.
// For permission questions:
//
//	response=AnswerOnce → unblocks with nil (proceed this time)
//	response=AnswerAlways → unblocks with nil + stores pattern in approved map
//	response=AnswerReject → unblocks with ErrDenied
//
// For user questions (from the question tool):
//
//	response is the raw answer string (selected option(s))
//
// Non-blocking: returns an error if the question is no longer waiting.
func (s *SessionService) Answer(ctx context.Context, sessionID, questionID string, response AnswerResponse) error {
	_, pqOK, _, uqOK, pendingPerms, pendingUsers := s.state.permissions.Lookup(questionID)

	if uqOK {
		log.Printf("answer: delivering user question %s response=%q for session %s", questionID, response, sessionID)
		if err := s.state.permissions.DeliverUserAnswer(questionID, response); err != nil {
			log.Printf("answer: user question %s already answered or expired", questionID)
			return err
		}
		return nil
	}

	if !pqOK {
		log.Printf("answer: question %s not found (session=%s, pending_pq=%d, pending_uq=%d)", questionID, sessionID, pendingPerms, pendingUsers)
		return fmt.Errorf("answer: question %q not found", questionID)
	}
	if err := s.state.permissions.DeliverPermissionAnswer(sessionID, questionID, response); err == nil {
		return nil
	}
	return fmt.Errorf("answer: question %q already answered or expired", questionID)
}

// askPermission returns an Ask function for ExecutionContext.
// It emits a "question" SSE event and blocks until Answer is called or ctx is cancelled.
// ctx cancellation prevents a goroutine leak when the SSE client disconnects.
func (s *SessionService) askPermission(ctx context.Context, sessionID, questionID, toolName, input, suggestedPattern string, approvalPatterns []string) error {
	ch := make(chan error, 1)
	s.state.permissions.SetPendingPermission(questionID, pendingQuestion{
		ch:        ch,
		sessionID: sessionID,
		tool:      toolName,
		patterns:  append([]string(nil), approvalPatterns...),
	})
	s.publish(sessionID, SSEEvent{
		Type: "question",
		Data: SSEQuestionData{
			QuestionID:       questionID,
			Tool:             toolName,
			Input:            input,
			SuggestedPattern: suggestedPattern,
		},
	})
	defer func() {
		s.state.permissions.DeletePendingPermission(questionID)
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
