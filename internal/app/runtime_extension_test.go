package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

type extensionTestTool struct {
	definition tool.Definition
}

func (t extensionTestTool) Definition() tool.Definition {
	return t.definition
}

func (t extensionTestTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	return tool.Result{Output: "ok"}, nil
}

func TestRuntimeExtensionSurfaceCollectsToolsAndMetadata(t *testing.T) {
	hook := &recordingPrecomputeHook{calls: make(chan RuntimePrecomputeHint, 1)}

	surface, err := buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
			Effects: []tool.ExecutionEffect{tool.ExecutionEffectRead},
		}},
		PrecomputeHooks: []RuntimeExtensionPrecomputeHookRegistration{{
			ID:   "metadata_refresh",
			Hook: hook,
		}},
		ContextContributions: []RuntimeExtensionContextContribution{{
			ID:          "metadata_context",
			Description: "Metadata summaries",
		}},
	}})
	if err != nil {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v", err)
	}
	if len(surface.Tools) != 1 || surface.Tools[0].Definition().Name != "metadata_scan" {
		t.Fatalf("surface tools = %#v", surface.Tools)
	}
	if got := surface.ToolEffects["metadata_scan"]; len(got) != 1 || got[0] != tool.ExecutionEffectRead {
		t.Fatalf("surface tool effects = %#v", surface.ToolEffects)
	}
	if len(surface.PrecomputeHooks) != 1 || surface.PrecomputeHooks[0] != hook {
		t.Fatalf("surface precompute hooks = %#v", surface.PrecomputeHooks)
	}
	if len(surface.ContextContributions) != 1 || surface.ContextContributions[0].ID != "metadata_context" {
		t.Fatalf("surface context contributions = %#v", surface.ContextContributions)
	}
}

func TestRegisterRuntimeExtensionAddsRegisteredSurface(t *testing.T) {
	resetRuntimeExtensionsForTest(t)
	effects := []tool.ExecutionEffect{tool.ExecutionEffectRead}

	RegisterRuntimeExtension(RuntimeExtensionRegistration{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
			Effects: effects,
		}},
	})
	effects[0] = tool.ExecutionEffectWrite

	surface, err := buildRuntimeExtensionSurface(registeredRuntimeExtensions())
	if err != nil {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v", err)
	}
	if len(surface.Tools) != 1 || surface.Tools[0].Definition().Name != "metadata_scan" {
		t.Fatalf("surface tools = %#v", surface.Tools)
	}
	if got := surface.ToolEffects["metadata_scan"]; len(got) != 1 || got[0] != tool.ExecutionEffectRead {
		t.Fatalf("surface tool effects = %#v, want cloned read effect", surface.ToolEffects)
	}
}

func TestBuildRuntimeToolsIncludesExtensionTools(t *testing.T) {
	extensionTool := extensionTestTool{definition: tool.Definition{
		Name:        "metadata_scan",
		Description: "Scan metadata",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}

	tools, err := buildRuntimeTools(nil, []tool.Tool{extensionTool})
	if err != nil {
		t.Fatalf("buildRuntimeTools() error = %v", err)
	}
	if !runtimeToolNamesInclude(tools, "metadata_scan") {
		t.Fatalf("runtime tools missing extension tool: %v", runtimeToolNames(tools))
	}
}

func TestBuildRuntimeToolsRejectsExtensionToolOverride(t *testing.T) {
	extensionTool := extensionTestTool{definition: tool.Definition{
		Name:        tool.ReadToolName,
		Description: "Override read",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}

	_, err := buildRuntimeTools(nil, []tool.Tool{extensionTool})
	if !errors.Is(err, ErrRuntimeExtensionToolDuplicate) {
		t.Fatalf("buildRuntimeTools() error = %v, want ErrRuntimeExtensionToolDuplicate", err)
	}
}

func TestRuntimeExtensionToolRequiresDeclaredEffects(t *testing.T) {
	_, err := buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		}},
	}})
	if !errors.Is(err, ErrRuntimeExtensionToolEffectMissing) {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v, want ErrRuntimeExtensionToolEffectMissing", err)
	}
}

func TestRuntimePrecomputeHooksIncludesExtensionHooks(t *testing.T) {
	hook := &recordingPrecomputeHook{calls: make(chan RuntimePrecomputeHint, 1)}
	runtime := &Runtime{extensionPrecomputeHooks: []RuntimePrecomputeHook{hook}}

	hooks := runtime.runtimePrecomputeHooks()
	if len(hooks) != 1 || hooks[0] != hook {
		t.Fatalf("runtimePrecomputeHooks() = %#v, want extension hook", hooks)
	}
}

func runtimeToolNamesInclude(tools []tool.Tool, target string) bool {
	for _, tl := range tools {
		if tl.Definition().Name == target {
			return true
		}
	}
	return false
}

func runtimeToolNames(tools []tool.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Definition().Name)
	}
	return names
}

func resetRuntimeExtensionsForTest(t *testing.T) {
	t.Helper()
	runtimeExtensionsMu.Lock()
	previous := runtimeExtensions
	runtimeExtensions = map[string]RuntimeExtensionRegistration{}
	runtimeExtensionsMu.Unlock()
	t.Cleanup(func() {
		runtimeExtensionsMu.Lock()
		runtimeExtensions = previous
		runtimeExtensionsMu.Unlock()
	})
}
