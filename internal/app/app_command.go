package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type runtimeFactory func(Config) (*Runtime, error)

func runCommand(ctx context.Context, in io.Reader, out io.Writer, args []string, getenv func(string) string, getwd func() (string, error), buildRuntime runtimeFactory) error {
	workspaceRoot, err := getwd()
	if err != nil {
		return err
	}
	input, err := ParseCommandInput(args, workspaceRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.WorkspaceRoot) != "" {
		workspaceRoot = input.WorkspaceRoot
	}
	if input.UserText == "" && !input.Resume {
		return ErrUserTextRequired
	}

	config, err := LoadRuntimeConfig(getenv)
	if err != nil {
		return err
	}
	runtime, err := buildRuntime(config)
	if err != nil {
		return err
	}
	defer func() {
		_ = runtime.Close()
	}()

	reader := bufio.NewReader(in)
	pendingTrust, err := runtime.EvaluateStartupTrust(ctx, workspaceRoot)
	if err != nil {
		return err
	}
	if pendingTrust.Pending() {
		decision, proceed, err := PromptStartupTrust(reader, out, pendingTrust)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		if err := runtime.ResolveStartupTrust(ctx, decision); err != nil {
			return err
		}
	}
	sessionID := ""
	if input.Resume {
		session, err := runtime.OpenWorkspaceSession(ctx, workspaceRoot, input.AdditionalWorkspaceRoots, true)
		if err != nil {
			return err
		}
		sessionID = session.SessionID
		pending, ok, err := resumePendingTurn(ctx, input, runtime, session.SessionID)
		if err != nil {
			return err
		}
		if ok {
			return continueSessionTurn(ctx, in, out, runtime, input, pending)
		}
	}
	if input.UserText == "" {
		return ErrUserTextRequired
	}
	if strings.TrimSpace(sessionID) == "" {
		session, err := runtime.OpenWorkspaceSession(ctx, workspaceRoot, input.AdditionalWorkspaceRoots, false)
		if err != nil {
			return err
		}
		sessionID = session.SessionID
	}
	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     NewTurnID(),
		UserText:   input.UserText,
		WorkflowID: input.WorkflowID,
		SkillIDs:   append([]string(nil), input.SkillIDs...),
	})
	if err != nil {
		return err
	}

	return continueSessionTurn(ctx, in, out, runtime, input, result)
}

func continueSessionTurn(ctx context.Context, in io.Reader, out io.Writer, runtime *Runtime, input CommandInput, result RunSessionResult) error {
	reader := bufio.NewReader(in)
	writtenText := ""

	for {
		if err := writeCLIText(out, unwrittenAssistantText(writtenText, result.AssistantText)); err != nil {
			return err
		}
		writtenText = result.AssistantText

		if result.Status == TurnRunStatusFailed {
			return writeCLIText(out, result.Error)
		}

		if result.Status != TurnRunStatusPending || (result.PendingPermission == nil && result.PendingExecution == nil && result.PendingQuestion == nil) {
			return nil
		}
		if result.PendingQuestion != nil {
			answer, err := promptQuestion(reader, out, *result.PendingQuestion)
			if err != nil {
				return err
			}
			result, err = runtime.AnswerSessionQuestion(ctx, AnswerSessionQuestionInput{
				SessionID: result.SessionID,
				TurnID:    result.TurnID,
				RequestID: result.PendingRequestID,
				Answer:    answer,
				UserText:  result.UserText,
				SkillIDs:  append([]string(nil), input.SkillIDs...),
			})
			if err != nil {
				return err
			}
			continue
		}

		resolveInput := ResolveSessionTurnInput{
			SessionID:           result.SessionID,
			TurnID:              result.TurnID,
			PermissionRequestID: result.PendingRequestID,
			UserText:            result.UserText,
			SkillIDs:            append([]string(nil), input.SkillIDs...),
		}
		if result.PendingExecution != nil {
			decision, err := promptExecutionApproval(reader, out, *result.PendingExecution)
			if err != nil {
				return err
			}
			resolveInput.ExecutionDecision = decision
		} else {
			decision, scope, grantPath, recursive, err := promptPermission(reader, out, *result.PendingPermission)
			if err != nil {
				return err
			}
			resolveInput.Decision = decision
			resolveInput.Scope = scope
			resolveInput.GrantPath = grantPath
			resolveInput.Recursive = recursive
		}

		var err error
		result, err = runtime.ResolveSessionTurn(ctx, resolveInput)
		if err != nil {
			return err
		}
	}
}

func promptPermission(reader *bufio.Reader, out io.Writer, pending events.PermissionRequestState) (events.PermissionDecision, events.PermissionScope, string, bool, error) {
	lines := []string{"Permission required:"}
	switch pending.Kind {
	case events.PermissionRequestKindExecution:
		lines = append(lines,
			"kind: execution",
			"dir: "+pending.WorkingDirectory,
		)
	case events.PermissionRequestKindNetwork:
		lines = append(lines,
			"kind: network",
			"target: "+pending.Path,
		)
	default:
		targetLabel := "path"
		if pending.Access == "workdir" {
			targetLabel = "dir"
		}
		lines = append(lines,
			"kind: path",
			"access: "+pending.Access,
			targetLabel+": "+pending.Path,
		)
	}
	lines = append(lines,
		"tool: "+pending.ToolName,
		"command: "+pending.Command,
		"1. Allow once",
		"2. Allow for session duration",
		"3. Deny",
		"Choice [3]: ",
	)
	if _, err := fmt.Fprint(out, strings.Join(lines, "\n")); err != nil {
		return "", "", "", false, err
	}

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", "", "", false, err
	}
	decision, scope, grantPath, recursive := permissionChoiceFromInput(strings.TrimSpace(line), pending.Kind, pending.Path)
	return decision, scope, grantPath, recursive, nil
}

func promptExecutionApproval(reader *bufio.Reader, out io.Writer, pending events.ExecutionApprovalState) (events.ExecutionApprovalDecision, error) {
	lines := []string{
		"Execution approval required:",
		"command: " + pending.Command,
		"dir: " + pending.WorkingDirectory,
	}
	if strings.TrimSpace(pending.Reason) != "" {
		lines = append(lines, "reason: "+pending.Reason)
	}
	if len(pending.PrefixRule) > 0 {
		lines = append(lines, "rule: "+strings.Join(pending.PrefixRule, " "))
	}
	if len(pending.NetworkTargets) > 0 {
		lines = append(lines, "network: "+strings.Join(displayExecutionNetworkTargets(pending.NetworkTargets), ", "))
	}
	lines = append(lines, executionApprovalPromptLines(pending)...)
	lines = append(lines, "Choice [3]: ")
	if _, err := fmt.Fprint(out, strings.Join(lines, "\n")); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return executionApprovalChoice(strings.TrimSpace(line), pending.AvailableDecisions), nil
}

func promptQuestion(reader *bufio.Reader, out io.Writer, pending events.QuestionRequestState) (string, error) {
	lines := []string{
		"Question:",
		pending.Question,
	}
	for idx, option := range pending.Options {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, option))
	}
	lines = append(lines, fmt.Sprintf("Choice [1-%d]: ", len(pending.Options)))
	if _, err := fmt.Fprint(out, strings.Join(lines, "\n")); err != nil {
		return "", err
	}

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		return pending.Options[0], nil
	}
	index, convErr := strconv.Atoi(choice)
	if convErr != nil || index < 1 || index > len(pending.Options) {
		return "", fmt.Errorf("invalid choice %q", choice)
	}
	return pending.Options[index-1], nil
}

func displayExecutionNetworkTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if strings.HasPrefix(trimmed, "command: ") {
			out = append(out, "rule "+strings.TrimPrefix(trimmed, "command: "))
			continue
		}
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func executionApprovalPromptLines(pending events.ExecutionApprovalState) []string {
	lines := []string{"1. Allow once", "2. Allow for session duration", "3. Deny"}
	nextChoice := 4
	for _, decision := range pending.AvailableDecisions {
		switch decision {
		case events.ExecutionApprovalDecisionApplyNetworkPolicy:
			lines = append(lines, fmt.Sprintf("%d. Allow with network enabled", nextChoice))
			nextChoice++
		case events.ExecutionApprovalDecisionAcceptWithExecPolicy:
			lines = append(lines, fmt.Sprintf("%d. Allow with execution policy amendment", nextChoice))
			nextChoice++
		}
	}
	return lines
}

func executionApprovalChoice(line string, available []events.ExecutionApprovalDecision) events.ExecutionApprovalDecision {
	switch line {
	case "1":
		return events.ExecutionApprovalDecisionAccept
	case "2":
		return events.ExecutionApprovalDecisionAcceptForSession
	case "4":
		for _, decision := range available {
			if decision == events.ExecutionApprovalDecisionApplyNetworkPolicy {
				return decision
			}
		}
		for _, decision := range available {
			if decision == events.ExecutionApprovalDecisionAcceptWithExecPolicy {
				return decision
			}
		}
	case "5":
		for _, decision := range available {
			if decision == events.ExecutionApprovalDecisionAcceptWithExecPolicy {
				return decision
			}
		}
	}
	return events.ExecutionApprovalDecisionDecline
}

func unwrittenAssistantText(writtenText, assistantText string) string {
	if writtenText == "" {
		return assistantText
	}
	if strings.HasPrefix(assistantText, writtenText) {
		return assistantText[len(writtenText):]
	}
	return assistantText
}

func writeCLIText(out io.Writer, text string) error {
	if text == "" {
		return nil
	}
	if _, err := io.WriteString(out, text); err != nil {
		return err
	}
	if strings.HasSuffix(text, "\n") {
		return nil
	}
	_, err := io.WriteString(out, "\n")
	return err
}
