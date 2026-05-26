package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var (
	ErrWriteRestoreTurnNotFound = errors.New("write restore turn not found")
	ErrWriteRestoreTurnRunning  = errors.New("write restore turn is still running")
	ErrWriteRestoreUnavailable  = errors.New("write restore is not available")
	ErrWriteRestoreConflict     = errors.New("write restore conflict")
)

type RestoreSessionTurnWritesInput struct {
	SessionID    string
	SourceTurnID string
}

type RestoreSessionTurnWritesResult struct {
	SourceTurnID string
	Paths        []string
}

type writeRestoreStep struct {
	CallID        string
	Path          string
	AfterContent  string
	BeforeContent string
	BeforeExisted bool
	BeforeMode    os.FileMode
}

type writeRestoreFileState struct {
	Exists  bool
	IsDir   bool
	Content string
}

func (r *Runtime) RestoreSessionTurnWrites(ctx context.Context, input RestoreSessionTurnWritesInput) (RestoreSessionTurnWritesResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RestoreSessionTurnWritesResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.SourceTurnID) == "" {
		return RestoreSessionTurnWritesResult{}, ErrTurnIDRequired
	}

	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RestoreSessionTurnWritesResult{}, err
	}
	steps, err := buildWriteRestoreSteps(ctx, state, r.Sessions.blobs, input.SourceTurnID)
	if err != nil {
		return RestoreSessionTurnWritesResult{}, err
	}
	if err := validateWriteRestoreSteps(steps); err != nil {
		return RestoreSessionTurnWritesResult{}, err
	}
	if err := applyWriteRestoreSteps(steps); err != nil {
		return RestoreSessionTurnWritesResult{}, err
	}

	restores := make([]events.WorkspaceWriteRestoreItem, 0, len(steps))
	paths := make([]string, 0, len(steps))
	for _, step := range steps {
		restores = append(restores, events.WorkspaceWriteRestoreItem{
			CallID:        step.CallID,
			Path:          step.Path,
			ExistedBefore: step.BeforeExisted,
		})
		paths = appendUniqueValues(paths, []string{step.Path})
	}
	if _, err := r.Sessions.RecordWorkspaceWriteRestore(ctx, RecordWorkspaceWriteRestoreInput{
		SessionID:    input.SessionID,
		SourceTurnID: input.SourceTurnID,
		Restores:     restores,
	}); err != nil {
		return RestoreSessionTurnWritesResult{}, err
	}

	if logger := r.log("runtime"); logger != nil {
		logger.Op("session turn writes restored",
			"session_id", input.SessionID,
			"source_turn_id", input.SourceTurnID,
			"restored_count", len(restores),
			"path_count", len(paths),
		)
	}
	return RestoreSessionTurnWritesResult{
		SourceTurnID: input.SourceTurnID,
		Paths:        paths,
	}, nil
}

func buildWriteRestoreSteps(ctx context.Context, state events.SessionState, blobs ToolResultBlobStore, sourceTurnID string) ([]writeRestoreStep, error) {
	turn := state.Turns[sourceTurnID]
	if turn == nil {
		return nil, ErrWriteRestoreTurnNotFound
	}
	if turn.Status == events.TurnStatusRunning {
		return nil, ErrWriteRestoreTurnRunning
	}

	steps := make([]writeRestoreStep, 0, len(turn.ToolCallOrder))
	for i := len(turn.ToolCallOrder) - 1; i >= 0; i-- {
		call := turn.ToolCalls[turn.ToolCallOrder[i]]
		if call == nil || !call.Completed || !call.Succeeded || call.WriteMutation == nil {
			continue
		}
		beforeContent, err := loadWriteMutationBeforeContent(ctx, blobs, call.WriteMutation)
		if err != nil {
			return nil, err
		}
		afterContent, ok, err := restoreAfterContentForCall(call.ToolName, call.Input, beforeContent)
		if err != nil {
			return nil, fmt.Errorf("%w: restore metadata missing text mutation content for %s", ErrWriteRestoreUnavailable, call.CallID)
		}
		if !ok {
			continue
		}
		steps = append(steps, writeRestoreStep{
			CallID:        call.CallID,
			Path:          call.WriteMutation.Path,
			AfterContent:  afterContent,
			BeforeContent: beforeContent,
			BeforeExisted: call.WriteMutation.Existed,
			BeforeMode:    os.FileMode(call.WriteMutation.Mode),
		})
	}
	if len(steps) == 0 {
		return nil, ErrWriteRestoreUnavailable
	}
	return steps, nil
}

func parseWriteRestoreCallArguments(raw string) (writeToolArguments, error) {
	args, err := parseWriteToolArguments(json.RawMessage(raw))
	if err != nil {
		return writeToolArguments{}, err
	}
	if !args.HasContent {
		return writeToolArguments{}, errors.New("content is required")
	}
	return args, nil
}

func restoreAfterContentForCall(toolName, rawInput, beforeContent string) (string, bool, error) {
	switch strings.TrimSpace(toolName) {
	case "write":
		args, err := parseWriteRestoreCallArguments(rawInput)
		if err != nil {
			return "", false, err
		}
		return args.Content, true, nil
	default:
		return "", false, nil
	}
}

func loadWriteMutationBeforeContent(ctx context.Context, blobs ToolResultBlobStore, mutation *events.WriteMutation) (string, error) {
	if mutation == nil {
		return "", ErrWriteRestoreUnavailable
	}
	if !mutation.BeforeTruncated {
		return mutation.Before, nil
	}
	if mutation.BeforeBlob == nil || strings.TrimSpace(mutation.BeforeBlob.Ref) == "" || blobs == nil {
		return "", fmt.Errorf("%w: previous file content is unavailable for %s", ErrWriteRestoreUnavailable, mutation.Path)
	}
	before, err := blobs.Load(ctx, mutation.BeforeBlob.Ref)
	if err != nil {
		return "", err
	}
	return before, nil
}

func validateWriteRestoreSteps(steps []writeRestoreStep) error {
	simulated := make(map[string]writeRestoreFileState, len(steps))
	for _, step := range steps {
		state, ok := simulated[step.Path]
		if !ok {
			current, err := readWriteRestoreFileState(step.Path)
			if err != nil {
				return err
			}
			state = current
		}
		if err := validateWriteRestoreStepState(step, state); err != nil {
			return err
		}
		simulated[step.Path] = writeRestoreFileState{
			Exists:  step.BeforeExisted,
			Content: step.BeforeContent,
		}
	}
	return nil
}

func validateWriteRestoreStepState(step writeRestoreStep, state writeRestoreFileState) error {
	switch {
	case !state.Exists:
		return fmt.Errorf("%w: %s no longer exists at the state written by %s", ErrWriteRestoreConflict, step.Path, step.CallID)
	case state.IsDir:
		return fmt.Errorf("%w: %s is now a directory", ErrWriteRestoreConflict, step.Path)
	case state.Content != step.AfterContent:
		return fmt.Errorf("%w: %s no longer matches the write output from %s", ErrWriteRestoreConflict, step.Path, step.CallID)
	default:
		return nil
	}
}

func readWriteRestoreFileState(path string) (writeRestoreFileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeRestoreFileState{}, nil
		}
		return writeRestoreFileState{}, err
	}
	if info.IsDir() {
		return writeRestoreFileState{Exists: true, IsDir: true}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return writeRestoreFileState{}, err
	}
	return writeRestoreFileState{
		Exists:  true,
		Content: string(data),
	}, nil
}

func applyWriteRestoreSteps(steps []writeRestoreStep) error {
	for _, step := range steps {
		if err := applyWriteRestoreStep(step); err != nil {
			return err
		}
	}
	return nil
}

func applyWriteRestoreStep(step writeRestoreStep) error {
	if !step.BeforeExisted {
		if err := os.Remove(step.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(step.Path), 0o755); err != nil {
		return err
	}
	mode := step.BeforeMode
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(step.Path, []byte(step.BeforeContent), mode); err != nil {
		return err
	}
	return os.Chmod(step.Path, mode.Perm())
}
