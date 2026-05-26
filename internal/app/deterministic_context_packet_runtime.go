package app

import (
	"context"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

const deterministicContextDiagnosticsTimeout = 2 * time.Second

type deterministicContextPacketRuntimeInput struct {
	WorkspaceRoot             string
	AdditionalWorkspaceRoots  []string
	UserText                  string
	Route                     provider.ModelRoute
	IsFirstTurn               bool
	IsContinuation            bool
	PreviousSentSectionBodies map[string]string
}

func (r *Runtime) deterministicContextPacketFragment(ctx context.Context, input deterministicContextPacketRuntimeInput) (prompt.Fragment, bool) {
	if r == nil || len(r.Config.ContextPacket.EnabledSections) == 0 {
		return prompt.Fragment{}, false
	}
	if input.IsContinuation {
		return prompt.Fragment{}, false
	}
	status, err := loadWorkspaceStatus(ctx, input.WorkspaceRoot, r.Search)
	if err != nil {
		if logger := r.log("runtime"); logger != nil {
			logger.Debug("deterministic context packet status load failed",
				"workspace_root", input.WorkspaceRoot,
				"error", err.Error(),
			)
		}
		return prompt.Fragment{}, false
	}
	diagnosticPaths, diagnosticCandidates := deterministicContextPacketDiagnosticCandidatePaths(input.WorkspaceRoot, status)
	diagnostics, diagnosticsEnabled := r.deterministicContextPacketDiagnostics(ctx, deterministicContextPacketDiagnosticsInput{
		WorkspaceRoot:            input.WorkspaceRoot,
		AdditionalWorkspaceRoots: input.AdditionalWorkspaceRoots,
		UserText:                 input.UserText,
		CandidatePaths:           diagnosticPaths,
		CandidateFiles:           diagnosticCandidates,
	})
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot: input.WorkspaceRoot,
		Status:        status,
		Diagnostics:   diagnostics,
		DiagnosticCandidateFiles: func() int {
			if diagnosticsEnabled {
				return diagnosticCandidates
			}
			return 0
		}(),
		DiagnosticOmittedFiles: max(diagnosticCandidates-len(diagnosticPaths), 0),
	})
	sections = selectDeterministicContextPacketSections(deterministicContextPacketSectionPolicyInput{
		Sections:                  sections,
		UserText:                  input.UserText,
		IsFirstTurn:               input.IsFirstTurn,
		PreviousSentSectionBodies: input.PreviousSentSectionBodies,
	})
	packet := buildDeterministicContextPacketForRequest(deterministicContextPacketRequestInput{
		Request:         provider.Request{Model: input.Route.Primary},
		Models:          r.ModelCatalog,
		EnabledSections: append([]string(nil), r.Config.ContextPacket.EnabledSections...),
		Sections:        sections,
	})
	if strings.TrimSpace(packet.Content) == "" {
		return prompt.Fragment{}, false
	}
	return deterministicContextPacketFragment(packet)
}

type deterministicContextPacketDiagnosticsInput struct {
	WorkspaceRoot            string
	AdditionalWorkspaceRoots []string
	UserText                 string
	CandidatePaths           []string
	CandidateFiles           int
}

type deterministicContextPacketDiagnosticsProvider interface {
	DiagnosticsForFiles(context.Context, string, []string, []string, time.Duration) ([]tool.CodeIntelFileDiagnostics, error)
}

func (r *Runtime) deterministicContextPacketDiagnostics(ctx context.Context, input deterministicContextPacketDiagnosticsInput) ([]tool.CodeIntelFileDiagnostics, bool) {
	if r == nil || !deterministicContextPacketEnabledSections(r.Config.ContextPacket.EnabledSections)[deterministicContextPacketSectionDiagnostics] {
		return nil, false
	}
	diagnosticsProvider := r.ContextPacketDiagnostics
	if diagnosticsProvider == nil {
		diagnosticsProvider = r.CodeIntel
	}
	if diagnosticsProvider == nil {
		return nil, false
	}
	if !deterministicContextPacketUserRequestsDiagnostics(input.UserText) || len(input.CandidatePaths) == 0 || input.CandidateFiles == 0 {
		return nil, false
	}
	diagnosticsCtx, cancel := context.WithTimeout(ctx, deterministicContextDiagnosticsTimeout)
	defer cancel()
	diagnostics, err := diagnosticsProvider.DiagnosticsForFiles(diagnosticsCtx, input.WorkspaceRoot, input.AdditionalWorkspaceRoots, input.CandidatePaths, deterministicContextDiagnosticsTimeout)
	if err != nil {
		if logger := r.log("runtime"); logger != nil {
			logger.Debug("deterministic context diagnostics omitted",
				"workspace_root", input.WorkspaceRoot,
				"error", err.Error(),
			)
		}
		return nil, false
	}
	return diagnostics, true
}

type deterministicContextPacketSectionPolicyInput struct {
	Sections                  []deterministicContextPacketSectionInput
	UserText                  string
	IsFirstTurn               bool
	PreviousSentSectionBodies map[string]string
}

func selectDeterministicContextPacketSections(input deterministicContextPacketSectionPolicyInput) []deterministicContextPacketSectionInput {
	if len(input.Sections) == 0 {
		return nil
	}
	if input.IsFirstTurn {
		return append([]deterministicContextPacketSectionInput(nil), input.Sections...)
	}
	userRequestedWorkspaceState := deterministicContextPacketUserRequestsWorkspaceState(input.UserText)
	userRequestedDiagnostics := deterministicContextPacketUserRequestsDiagnostics(input.UserText)
	selected := make([]deterministicContextPacketSectionInput, 0, len(input.Sections))
	for _, section := range input.Sections {
		key := normalizeDeterministicContextPacketKey(section.Key)
		content := strings.TrimSpace(section.Content)
		if key == "" || content == "" {
			continue
		}
		previous, hadPrevious := input.PreviousSentSectionBodies[key]
		switch key {
		case deterministicContextPacketSectionRepo:
			if !hadPrevious || strings.TrimSpace(previous) != content {
				selected = append(selected, section)
			}
		case deterministicContextPacketSectionGit, deterministicContextPacketSectionGitDirtySummary:
			if userRequestedWorkspaceState || !hadPrevious || strings.TrimSpace(previous) != content {
				selected = append(selected, section)
			}
		case deterministicContextPacketSectionDiagnostics:
			if userRequestedDiagnostics || !hadPrevious || strings.TrimSpace(previous) != content {
				selected = append(selected, section)
			}
		default:
			if userRequestedWorkspaceState || !hadPrevious || strings.TrimSpace(previous) != content {
				selected = append(selected, section)
			}
		}
	}
	return selected
}

func deterministicContextPacketUserRequestsWorkspaceState(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{
		"git",
		"status",
		"dirty",
		"worktree",
		"working tree",
		"changed",
		"changes",
		"modified",
		"staged",
		"unstaged",
		"untracked",
		"branch",
		"commit",
		"diff",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func deterministicContextPacketUserRequestsDiagnostics(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{
		"diagnostic",
		"diagnostics",
		"error",
		"errors",
		"warning",
		"warnings",
		"fix",
		"failing",
		"failure",
		"broken",
		"compile",
		"compiler",
		"typecheck",
		"type check",
		"lint",
		"test",
		"tests",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func lastSentDeterministicContextPacketSections(state events.SessionState, enabledSections []string) map[string]string {
	sections := make(map[string]string)
	enabled := deterministicContextPacketEnabledSections(enabledSections)
	if len(enabled) == 0 {
		return sections
	}
	for index := len(state.TurnOrder) - 1; index >= 0; index-- {
		turnID := state.TurnOrder[index]
		turn := state.Turns[turnID]
		if turn == nil || turn.Prompt == nil || !turnSentDeterministicContextPacket(turn) {
			continue
		}
		for key, content := range extractDeterministicContextPacketSections(turn.Prompt.Instructions) {
			if enabled[key] && sections[key] == "" {
				sections[key] = content
			}
		}
		for key, content := range extractDeterministicContextPacketSections(turn.Prompt.CacheablePrefix) {
			if enabled[key] && sections[key] == "" {
				sections[key] = content
			}
		}
		for key, content := range extractDeterministicContextPacketSections(turn.Prompt.DynamicSuffix) {
			if enabled[key] && sections[key] == "" {
				sections[key] = content
			}
		}
		if len(sections) == len(enabled) {
			return sections
		}
	}
	return sections
}

func turnSentDeterministicContextPacket(turn *events.TurnState) bool {
	for _, attempt := range turn.ProviderAttempts {
		if attempt.DeterministicContextTokens > 0 {
			return true
		}
	}
	return false
}

func extractDeterministicContextPacketSections(text string) map[string]string {
	sections := make(map[string]string)
	for {
		start := strings.Index(text, deterministicContextPacketStartTag)
		if start < 0 {
			return sections
		}
		text = text[start+len(deterministicContextPacketStartTag):]
		end := strings.Index(text, deterministicContextPacketEndTag)
		if end < 0 {
			return sections
		}
		packet := text[:end]
		extractDeterministicContextPacketSectionBodies(packet, sections)
		text = text[end+len(deterministicContextPacketEndTag):]
	}
}

func extractDeterministicContextPacketSectionBodies(packet string, sections map[string]string) {
	for {
		start := strings.Index(packet, "<section ")
		if start < 0 {
			return
		}
		packet = packet[start:]
		tagEnd := strings.Index(packet, ">")
		if tagEnd < 0 {
			return
		}
		tag := packet[:tagEnd+1]
		key := normalizeDeterministicContextPacketKey(extractDeterministicContextPacketAttribute(tag, "key"))
		packet = packet[tagEnd+1:]
		closeIndex := strings.Index(packet, "</section>")
		if closeIndex < 0 {
			return
		}
		if key != "" {
			sections[key] = strings.TrimSpace(packet[:closeIndex])
		}
		packet = packet[closeIndex+len("</section>"):]
	}
}

func extractDeterministicContextPacketAttribute(tag, name string) string {
	prefix := name + `="`
	start := strings.Index(tag, prefix)
	if start < 0 {
		return ""
	}
	valueStart := start + len(prefix)
	valueEnd := strings.Index(tag[valueStart:], `"`)
	if valueEnd < 0 {
		return ""
	}
	return tag[valueStart : valueStart+valueEnd]
}
