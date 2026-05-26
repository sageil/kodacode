package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
)

const (
	toolResultBlobInlineLimit = 4096
	toolResultBlobPreviewHead = 2048
	toolResultBlobPreviewTail = 1024
)

type ToolResultBlobStore interface {
	Save(context.Context, ToolResultBlobKey, string) (*events.ToolResultBlobRef, error)
	Load(context.Context, string) (string, error)
}

type ToolResultBlobKey struct {
	SessionID string
	TurnID    string
	CallID    string
	Stream    string
}

type toolResultPayload struct {
	Output          string
	Error           string
	OutputBlob      *events.ToolResultBlobRef
	ErrorBlob       *events.ToolResultBlobRef
	OutputTruncated bool
	ErrorTruncated  bool
}

func prepareToolResultPayload(ctx context.Context, store ToolResultBlobStore, sessionID, turnID, callID, _ string, output, errorText string) (toolResultPayload, error) {
	payload := toolResultPayload{Output: output, Error: errorText}
	if store == nil {
		return payload, nil
	}
	outputPreview, outputBlob, outputTruncated, err := maybeOffloadToolResultText(ctx, store, ToolResultBlobKey{
		SessionID: sessionID,
		TurnID:    turnID,
		CallID:    callID,
		Stream:    "output",
	}, output)
	if err != nil {
		return payload, nil
	}
	errorPreview, errorBlob, errorTruncated, err := maybeOffloadToolResultText(ctx, store, ToolResultBlobKey{
		SessionID: sessionID,
		TurnID:    turnID,
		CallID:    callID,
		Stream:    "error",
	}, errorText)
	if err != nil {
		return payload, nil
	}
	payload.Output = outputPreview
	payload.Error = errorPreview
	payload.OutputBlob = outputBlob
	payload.ErrorBlob = errorBlob
	payload.OutputTruncated = outputTruncated
	payload.ErrorTruncated = errorTruncated
	return payload, nil
}

func prepareTextMutationPayload(ctx context.Context, store ToolResultBlobStore, sessionID, turnID, callID string, mutation *events.WriteMutation, args textMutationArguments) (*events.WriteMutation, error) {
	if mutation == nil {
		return nil, nil
	}
	copyMutation := *mutation
	copyMutation.BeforeBlob = nil
	copyMutation.BeforeTruncated = false
	copyMutation.DiffPreview = buildTextMutationDiffPreview(mutation, args)
	if store == nil {
		return &copyMutation, nil
	}
	beforePreview, beforeBlob, beforeTruncated, err := maybeOffloadToolResultText(ctx, store, ToolResultBlobKey{
		SessionID: sessionID,
		TurnID:    turnID,
		CallID:    callID,
		Stream:    "write-before",
	}, mutation.Before)
	if err != nil {
		return &copyMutation, nil
	}
	copyMutation.Before = beforePreview
	copyMutation.BeforeBlob = beforeBlob
	copyMutation.BeforeTruncated = beforeTruncated
	return &copyMutation, nil
}

func prepareToolTextMutationPayloads(ctx context.Context, store ToolResultBlobStore, sessionID, turnID, callID string, mutations []toolTextMutationPayload) ([]events.WriteMutation, error) {
	if len(mutations) == 0 {
		return nil, nil
	}
	out := make([]events.WriteMutation, 0, len(mutations))
	for idx, mutation := range mutations {
		eventMutation := events.WriteMutation{
			Path:        mutation.Path,
			Existed:     mutation.Existed,
			Before:      mutation.Before,
			DiffPreview: buildMutationDiffPreview(mutation.Before, mutation.After),
			Mode:        mutation.Mode,
		}
		if store != nil {
			beforePreview, beforeBlob, beforeTruncated, err := maybeOffloadToolResultText(ctx, store, ToolResultBlobKey{
				SessionID: sessionID,
				TurnID:    turnID,
				CallID:    callID,
				Stream:    fmt.Sprintf("write-before-%d", idx+1),
			}, mutation.Before)
			if err == nil {
				eventMutation.Before = beforePreview
				eventMutation.BeforeBlob = beforeBlob
				eventMutation.BeforeTruncated = beforeTruncated
			}
		}
		out = append(out, eventMutation)
	}
	return out, nil
}

func buildTextMutationDiffPreview(mutation *events.WriteMutation, args textMutationArguments) *textdiff.Preview {
	if mutation == nil || !args.HasAfterContent {
		return nil
	}
	return buildMutationDiffPreview(mutation.Before, args.AfterContent)
}

func buildMutationDiffPreview(before, after string) *textdiff.Preview {
	preview := textdiff.BuildPreview(before, after, 2)
	return &preview
}

func maybeOffloadToolResultText(ctx context.Context, store ToolResultBlobStore, key ToolResultBlobKey, text string) (string, *events.ToolResultBlobRef, bool, error) {
	if strings.TrimSpace(text) == "" {
		return text, nil, false, nil
	}
	runes := []rune(text)
	if len(runes) <= toolResultBlobInlineLimit {
		return text, nil, false, nil
	}
	ref, err := store.Save(ctx, key, text)
	if err != nil {
		return text, nil, false, err
	}
	return toolResultPreview(key.Stream, text), ref, true, nil
}

func toolResultPreview(stream, text string) string {
	runes := []rune(text)
	if len(runes) <= toolResultBlobInlineLimit {
		return text
	}
	label := "output"
	if stream == "error" {
		label = "error"
	}
	head := min(toolResultBlobPreviewHead, len(runes))
	tail := min(toolResultBlobPreviewTail, max(len(runes)-head, 0))
	headText := string(runes[:head])
	tailText := ""
	if tail > 0 {
		tailText = string(runes[len(runes)-tail:])
	}
	return fmt.Sprintf("[%s truncated: %d chars total]\n%s\n...\n%s", label, len(runes), headText, tailText)
}
