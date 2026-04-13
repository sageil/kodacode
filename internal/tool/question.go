package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// questionOption is a single option with an optional semantic role.
type questionOption struct {
	Label string `json:"label"`
	Role  string `json:"role,omitempty"`
}

// questionArgs is the JSON shape the model sends when calling the question tool.
type questionArgs struct {
	Question string           `json:"question"`
	Options  []questionOption `json:"options"`
	Multiple bool             `json:"multiple"`
	Purpose  string           `json:"purpose,omitempty"`
}

// NewQuestionTool creates a tool that lets the model ask the user a question
// with selectable options. The tool blocks until the user responds.
func NewQuestionTool() *Tool {
	return &Tool{
		Name:        "question",
		ReadOnly:    true,
		Description: `Ask the user a short question with selectable options. Use this tool for every user-facing question that expects an answer. The question appears in a compact dialog — keep it to 1-2 sentences max. Put details, context, and explanations in your text output BEFORE calling this tool, not inside the question field. For open-ended questions, include an option such as "Other" or "Something else" so the user can type a custom answer. The user sees a selection dialog and picks one or more answers.`,
		Parameters: []byte(`{
			"type": "object",
			"properties": {
				"question": {
					"type": "string",
					"description": "A short question (1-2 sentences). Do NOT put details here. Write them as text output before calling this tool."
				},
				"options": {
					"type": "array",
					"items": {
						"oneOf": [
							{ "type": "string" },
							{
								"type": "object",
								"properties": {
									"label": { "type": "string" },
									"role": { "type": "string", "enum": ["approve", "reject"] }
								},
								"required": ["label"]
							}
						]
					},
					"description": "List of options. Each can be a string or an object with label and optional role (approve/reject)."
				},
				"multiple": {
					"type": "boolean",
					"description": "If true, the user can select multiple options. If false (default), the user selects exactly one."
				},
				"purpose": {
					"type": "string",
					"description": "Semantic purpose of the question. Use 'plan_approval' when asking the user to approve or reject a plan."
				}
			},
			"required": ["question", "options"]
		}`),
		Execute: executeQuestion,
	}
}

func executeQuestion(ctx context.Context, ectx ExecutionContext, args []byte) (*Result, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return ErrorResult(ErrCodeInvalidArgs, fmt.Sprintf("question: invalid arguments: %v", err), true), nil
	}
	var qa questionArgs
	if q, ok := raw["question"]; ok {
		_ = json.Unmarshal(q, &qa.Question)
	}
	if m, ok := raw["multiple"]; ok {
		_ = json.Unmarshal(m, &qa.Multiple)
	}
	if p, ok := raw["purpose"]; ok {
		_ = json.Unmarshal(p, &qa.Purpose)
	}
	if o, ok := raw["options"]; ok {
		qa.Options = parseQuestionOptions(o)
	}
	if qa.Question == "" {
		return ErrorResult(ErrCodeInvalidArgs,
			"question: question text is required. Put the question in the 'question' field and choices in the 'options' array.",
			true), nil
	}
	if len(qa.Options) == 0 {
		return ErrorResult(ErrCodeInvalidArgs,
			"question: at least one option is required in the 'options' array. "+
				"For open-ended questions, include an option such as 'Other' so the user can type a custom answer.",
			true), nil
	}

	if ectx.AskUser == nil {
		log.Printf("question tool: AskUser is nil for session %s", ectx.SessionID)
		return ErrorResult(ErrCodeUnavailable, "question: user interaction is not available in this environment", false), nil
	}

	labels := make([]string, len(qa.Options))
	for i, o := range qa.Options {
		labels[i] = o.Label
	}

	log.Printf("question tool: asking %q with %d options purpose=%q (session=%s)", qa.Question, len(qa.Options), qa.Purpose, ectx.SessionID)
	answer, err := ectx.AskUser(qa.Question, labels, qa.Multiple, qa.Purpose)
	if err != nil {
		log.Printf("question tool: AskUser error for session %s: %v", ectx.SessionID, err)
		return nil, err
	}

	if answer == "" {
		log.Printf("question tool: user cancelled question for session %s", ectx.SessionID)
		return ErrorResult(ErrCodeCancelled, "The user cancelled the question without selecting an answer.", false), nil
	}

	selectedRole := ""
	for _, o := range qa.Options {
		if o.Label == answer {
			selectedRole = o.Role
			break
		}
	}

	display := qa.Question + "\n> " + answer
	meta := map[string]any{
		"question": qa.Question,
		"answer":   answer,
	}
	if qa.Purpose != "" {
		meta["purpose"] = qa.Purpose
	}
	if selectedRole != "" {
		meta["role"] = selectedRole
	}

	return &Result{
		Output:   display,
		Metadata: meta,
	}, nil
}

// parseQuestionOptions handles both string arrays and object arrays for backward compatibility.
func parseQuestionOptions(raw json.RawMessage) []questionOption {
	// Try object array first.
	var opts []questionOption
	if json.Unmarshal(raw, &opts) == nil && len(opts) > 0 && opts[0].Label != "" {
		return opts
	}
	// Try string array.
	var strs []string
	if json.Unmarshal(raw, &strs) == nil {
		out := make([]questionOption, len(strs))
		for i, s := range strs {
			out[i] = questionOption{Label: s}
		}
		return out
	}
	// Try comma-separated string.
	var single string
	if json.Unmarshal(raw, &single) == nil && single != "" {
		var out []questionOption
		for s := range strings.SplitSeq(single, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, questionOption{Label: s})
			}
		}
		return out
	}
	return nil
}

// FormatQuestionAnswer formats a question and its selected answers for display.
func FormatQuestionAnswer(question string, answers []string) string {
	if len(answers) == 0 {
		return question + "\n> (no selection)"
	}
	var sb strings.Builder
	sb.WriteString(question)
	sb.WriteString("\n")
	for _, a := range answers {
		sb.WriteString("> ")
		sb.WriteString(a)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
