package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

func replayToolResultText(ctx context.Context, blobs ToolResultBlobStore, toolName, output string, outputBlob *events.ToolResultBlobRef, errorText string, errorBlob *events.ToolResultBlobRef, succeeded bool) (string, string) {
	if blobs == nil {
		return normalizeToolResultText(toolName, output, errorText, succeeded)
	}
	if outputBlob != nil && strings.TrimSpace(outputBlob.Ref) != "" && outputBlob.Bytes > 0 {
		if loaded, err := blobs.Load(ctx, outputBlob.Ref); err == nil {
			output = loaded
		}
	}
	if errorBlob != nil && strings.TrimSpace(errorBlob.Ref) != "" && errorBlob.Bytes > 0 {
		if loaded, err := blobs.Load(ctx, errorBlob.Ref); err == nil {
			errorText = loaded
		}
	}
	return normalizeToolResultText(toolName, output, errorText, succeeded)
}

func replayTemporaryGrants(request events.PermissionRequestedPayload, call replayedToolCall) []workspace.Grant {
	if strings.TrimSpace(request.Path) == "" {
		return nil
	}
	return []workspace.Grant{{
		Path:      request.Path,
		Recursive: false,
	}}
}

func replayExecutionTemporaryGrants(request events.PermissionRequestedPayload, call replayedToolCall) []workspace.Grant {
	if call.Execution == nil || strings.TrimSpace(request.ExecutionID) == "" || call.Execution.ExecutionID != request.ExecutionID {
		if strings.TrimSpace(request.Path) == "" {
			return nil
		}
		return []workspace.Grant{{Path: request.Path, Recursive: false}}
	}
	if strings.TrimSpace(call.Execution.WorkingDirectory) == "" {
		return nil
	}
	return []workspace.Grant{{Path: call.Execution.WorkingDirectory, Recursive: false}}
}

func appendReplayTemporaryGrants(existing, extra []workspace.Grant) []workspace.Grant {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	out := append([]workspace.Grant(nil), existing...)
	for _, grant := range existing {
		key := grant.Path + "\x00" + strconv.FormatBool(grant.Recursive)
		seen[key] = struct{}{}
	}
	for _, grant := range extra {
		if strings.TrimSpace(grant.Path) == "" {
			continue
		}
		key := grant.Path + "\x00" + strconv.FormatBool(grant.Recursive)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, grant)
	}
	return out
}

func findPendingToolCall(order []string, calls map[string]replayedToolCall, completed map[string]bool, requestSequence int64, requestToolCallID string) (*replayedToolCall, error) {
	if strings.TrimSpace(requestToolCallID) != "" {
		call, ok := calls[requestToolCallID]
		if !ok || call.DeclaredSequence > requestSequence || completed[requestToolCallID] {
			return nil, ErrPendingToolCallNotFound
		}
		copyCall := call
		return &copyCall, nil
	}

	var pending *replayedToolCall
	for _, callID := range order {
		call, ok := calls[callID]
		if !ok || call.DeclaredSequence > requestSequence || completed[callID] {
			continue
		}
		if pending != nil {
			return nil, ErrMultiplePendingToolCalls
		}
		copyCall := call
		pending = &copyCall
	}
	if pending == nil {
		return nil, ErrPendingToolCallNotFound
	}
	return pending, nil
}
