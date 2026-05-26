package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const QuestionToolName = "question"

var (
	ErrQuestionRequired      = errors.New("question is required")
	ErrQuestionOptionsEmpty  = errors.New("options is required")
	ErrQuestionOptionInvalid = errors.New("options must not contain empty values")
)

type QuestionTool struct{}

type questionInput struct {
	Question string
	Options  []string
	Purpose  string
}

func NewQuestionTool() QuestionTool {
	return QuestionTool{}
}

func (QuestionTool) Definition() Definition {
	return Definition{
		Name:                QuestionToolName,
		Description:         "Ask the user a single-choice question and wait for a response before continuing the turn. Use this for yes/no confirmations or short finite choices when the next step depends on user input that cannot be inferred safely.",
		ProviderDescription: "Ask the user a single-choice question and wait for the answer. Use this for confirmations or short finite choices.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"question":{"type":"string","description":"The exact user-facing question to ask."},"options":{"type":"array","items":{"type":"string"},"minItems":1,"description":"Single-choice answer options. Keep them short and mutually exclusive."},"purpose":{"type":["string","null"],"description":"Optional short explanation for why the answer is needed. Use null or omit this field when no explanation is needed."}},"required":["question","options"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"question":"Which environment should I use?","options":["dev","staging","prod"],"purpose":"Need one environment before deploying."}`},
		RequiresWorkspace:   false,
	}
}

func (QuestionTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseQuestionInput(args)
	if err != nil {
		return Result{}, err
	}

	response, err := ectx.AskQuestion(QuestionRequest{
		Question: input.Question,
		Options:  append([]string(nil), input.Options...),
		Purpose:  input.Purpose,
	})
	if err != nil {
		return Result{}, err
	}
	if !response.Answered {
		return Result{PendingQuestionID: response.RequestID}, nil
	}

	output, err := json.Marshal(struct {
		Answer string `json:"answer"`
	}{
		Answer: response.Answer,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(output)}, nil
}

func (QuestionTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseQuestionInput(args)
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	key, err := json.Marshal(struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
		Purpose  string   `json:"purpose"`
	}{
		Question: input.Question,
		Options:  append([]string(nil), input.Options...),
		Purpose:  input.Purpose,
	})
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	return string(key), nil
}

func parseQuestionInput(args json.RawMessage) (_ questionInput, err error) {
	defer func() {
		err = normalizeToolInputError(QuestionToolName, err)
	}()
	var raw struct {
		Question *string         `json:"question"`
		Options  json.RawMessage `json:"options"`
		Purpose  *string         `json:"purpose"`
	}
	if err := DecodeArgs(QuestionToolName, args, &raw); err != nil {
		return questionInput{}, err
	}
	if raw.Question == nil || strings.TrimSpace(*raw.Question) == "" {
		return questionInput{}, ErrQuestionRequired
	}
	options, ok, err := decodeOptionalStringArrayArg(QuestionToolName, raw.Options, "options")
	if err != nil {
		return questionInput{}, err
	}
	if !ok || len(options) == 0 {
		return questionInput{}, ErrQuestionOptionsEmpty
	}
	normalized := make([]string, 0, len(options))
	for _, option := range options {
		trimmed := strings.TrimSpace(option)
		if trimmed == "" {
			return questionInput{}, ErrQuestionOptionInvalid
		}
		normalized = append(normalized, trimmed)
	}
	return questionInput{
		Question: strings.TrimSpace(*raw.Question),
		Options:  normalized,
		Purpose:  strings.TrimSpace(stringValue(raw.Purpose)),
	}, nil
}
