package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func (tl *turnLoop) maybeHandlePlannerApproval(executions []toolExecution, retrySent *bool) (bool, error) {
	planText := ""
	for _, ex := range executions {
		if ex.call.Name != "subagent" || ex.errStr != nil {
			continue
		}
		if plannerAgentIDFromArgs(ex.call.Arguments) != "planner" {
			continue
		}
		planText = strings.TrimSpace(ex.output)
	}
	if planText == "" {
		return false, nil
	}

	var tasks []*tool.Task
	if tl.taskStore != nil {
		tasks = tl.taskStore.GetTasks(tl.req.SessionID)
	}

	if len(tasks) == 0 {
		if retrySent != nil && !*retrySent {
			*retrySent = true
			setWorkflowRuntimeDirective(tl.req, "[SYSTEM: The planner returned prose only and created no tasks. Call the planner again and make it return a concrete task list before asking for approval.]")
			log.Printf("workflow: planner returned no tasks, retry injected")
			return true, nil
		}
		return false, fmt.Errorf("planner returned no tasks")
	}

	planText = adoptPlannerPlanText(planText, tasks)

	log.Printf("workflow: planner completed, publishing plan approval prompt")
	tl.persistAssistantMessage(planText, nil, nil)
	tl.req.Messages = append(tl.req.Messages, provider.Message{
		Role: "assistant",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: planText},
		},
	})
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "assistant_message",
			Data: SSEAssistantMessageData{Content: planText},
		})
	}

	if tl.askUser == nil {
		setWorkflowRuntimeDirective(tl.req, "The plan above is ready for approval. Approval UI is unavailable in this environment. Ask the user whether to proceed before executing anything.")
		return false, nil
	}

	answer, err := tl.askPlanApproval()
	if err != nil {
		return false, err
	}
	decision := classifyPlanApprovalSelection(answer)
	if err := tl.maybeSaveApprovedPlan(planText, decision, answer); err != nil {
		return false, err
	}
	marker := encodePlanApprovalDecision(decision, answer)
	tl.persistTextMessage("user", marker, true)
	tl.req.Messages = append(tl.req.Messages, provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: marker},
		},
	})
	notePlanApprovalDecision(tl.req, decision, answer)
	tl.activateApprovedPlanTask(decision)
	return false, nil
}

func adoptPlannerPlanText(raw string, tasks []*tool.Task) string {
	if len(tasks) == 0 {
		return strings.TrimSpace(raw)
	}
	return renderStructuredPlan(extractPlannerSummary(raw), tasks)
}

func extractPlannerSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	paragraphs := strings.Split(raw, "\n\n")
	for _, paragraph := range paragraphs {
		p := strings.TrimSpace(paragraph)
		if p == "" {
			continue
		}
		if strings.Contains(p, "\n") {
			p = strings.TrimSpace(strings.SplitN(p, "\n", 2)[0])
		}
		p = strings.TrimSpace(strings.TrimPrefix(p, "---"))
		if p == "" {
			continue
		}

		lower := strings.ToLower(p)
		switch {
		case strings.HasPrefix(p, "#"),
			strings.HasPrefix(lower, "## "),
			strings.HasPrefix(lower, "tasks:"),
			strings.HasPrefix(lower, "task 1"),
			strings.HasPrefix(lower, "part 1"),
			strings.HasPrefix(lower, "acceptance criteria"):
			return ""
		case strings.HasPrefix(lower, "let me "),
			strings.HasPrefix(lower, "i'll "),
			strings.HasPrefix(lower, "i will "),
			strings.HasPrefix(lower, "now "),
			strings.HasPrefix(lower, "here's the implementation plan"),
			strings.HasPrefix(lower, "here is the implementation plan"):
			continue
		default:
			return p
		}
	}
	return ""
}

func (tl *turnLoop) maybeHandleDirectPlannerApproval(planText string) error {
	if tl.req.Ephemeral || tl.req.AgentID != "planner" {
		return nil
	}
	if strings.TrimSpace(planText) == "" || tl.askUser == nil {
		return nil
	}
	answer, err := tl.askPlanApproval()
	if err != nil {
		return err
	}
	decision := classifyPlanApprovalSelection(answer)
	if err := tl.maybeSaveApprovedPlan(planText, decision, answer); err != nil {
		return err
	}
	marker := encodePlanApprovalDecision(decision, answer)
	tl.persistTextMessage("user", marker, true)
	tl.req.Messages = append(tl.req.Messages, provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: marker},
		},
	})
	notePlanApprovalDecision(tl.req, decision, answer)
	tl.activateApprovedPlanTask(decision)
	return nil
}

func (tl *turnLoop) askPlanApproval() (string, error) {
	return tl.askUser(tl.ctx, tl.req.SessionID)(planApprovalQuestionText, planApprovalOptions, false, planApprovalPurpose)
}

func (tl *turnLoop) activateApprovedPlanTask(decision planApprovalDecision) {
	if decision != planApprovalApproved || tl.req == nil || !isEngineerWorkflowAgent(tl.req.AgentID) || tl.taskStore == nil {
		return
	}
	active, activated, err := tl.taskStore.EnsureActiveTask(tl.ctx, tl.req.SessionID)
	if err != nil || !activated || active == nil {
		return
	}
	if tl.publish != nil {
		tl.publish(tl.req.SessionID, SSEEvent{
			Type: "task_sync",
			Data: SSETaskSyncData{ActiveTaskID: active.ID},
		})
	}
}

func (tl *turnLoop) maybeSaveApprovedPlan(planText string, decision planApprovalDecision, answer string) error {
	if decision != planApprovalApproved || normalizePlanApprovalAnswer(answer) != planApprovalSaveOption {
		return nil
	}
	if tl.req == nil || !isEngineerWorkflowAgent(tl.req.AgentID) || tl.sndbx == nil {
		return nil
	}
	projectDir := strings.TrimSpace(tl.sndbx.ProjectDir())
	if projectDir == "" {
		return nil
	}
	planPath, err := nextApprovedPlanPath(projectDir, planFileSlug(initialUserText(tl.req.Messages)), time.Now())
	if err != nil {
		return fmt.Errorf("save approved plan: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		return fmt.Errorf("save approved plan: %w", err)
	}
	if err := os.WriteFile(planPath, []byte(renderApprovedPlanMarkdown(planText)), 0o644); err != nil {
		return fmt.Errorf("save approved plan: %w", err)
	}
	return nil
}

func renderApprovedPlanMarkdown(planText string) string {
	planText = strings.TrimSpace(planText)
	if planText == "" {
		return "# Approved Plan\n"
	}
	if strings.HasPrefix(planText, "#") {
		return planText + "\n"
	}
	return "# Approved Plan\n\n" + planText + "\n"
}

func nextApprovedPlanPath(projectDir, slug string, now time.Time) (string, error) {
	base := fmt.Sprintf("%s-%s", now.Format("2006-01-02"), slug)
	dir := filepath.Join(projectDir, "docs", "kodacode", "plans")
	for part := 1; part <= 99; part++ {
		path := filepath.Join(dir, fmt.Sprintf("%s-part%d.md", base, part))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if os.IsNotExist(err) {
			return path, nil
		} else {
			return "", err
		}
	}
	return "", fmt.Errorf("no free plan filename for %s", base)
}

func planFileSlug(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(text, "-")
	text = strings.Trim(text, "-")
	if text == "" {
		return "plan"
	}
	if len(text) > 48 {
		text = strings.Trim(text[:48], "-")
	}
	if text == "" {
		return "plan"
	}
	return text
}
