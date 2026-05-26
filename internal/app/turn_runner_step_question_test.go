package app

import (
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestHandleStepQuestionToolPreviewCommitsPlannerSavePlanQuestion(t *testing.T) {
	commits := 0
	discards := 0
	err := handleStepQuestionToolPreview(
		tool.QuestionToolName,
		`{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`,
		func() error {
			commits++
			return nil
		},
		func() error {
			discards++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("handleStepQuestionToolPreview() error = %v", err)
	}
	if commits != 1 || discards != 0 {
		t.Fatalf("commits=%d discards=%d, want commit only", commits, discards)
	}
}

func TestHandleStepQuestionToolPreviewDiscardsNormalQuestion(t *testing.T) {
	commits := 0
	discards := 0
	err := handleStepQuestionToolPreview(
		tool.QuestionToolName,
		`{"question":"Proceed?"}`,
		func() error {
			commits++
			return nil
		},
		func() error {
			discards++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("handleStepQuestionToolPreview() error = %v", err)
	}
	if commits != 0 || discards != 1 {
		t.Fatalf("commits=%d discards=%d, want discard only", commits, discards)
	}
}

func TestHandleStepQuestionToolPreviewIgnoresNonQuestion(t *testing.T) {
	commits := 0
	discards := 0
	err := handleStepQuestionToolPreview(
		tool.ReadToolName,
		`{"paths":["app.go"]}`,
		func() error {
			commits++
			return nil
		},
		func() error {
			discards++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("handleStepQuestionToolPreview() error = %v", err)
	}
	if commits != 0 || discards != 0 {
		t.Fatalf("commits=%d discards=%d, want no action", commits, discards)
	}
}

func TestHandleStepQuestionToolPreviewReturnsCallbackError(t *testing.T) {
	want := errors.New("commit failed")
	err := handleStepQuestionToolPreview(
		tool.QuestionToolName,
		`{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`,
		func() error {
			return want
		},
		func() error {
			return nil
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
