package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestDeterministicContextPacketWorkspaceSectionsBuildsBoundedRepoAndGitSections(t *testing.T) {
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot: "/workspace/kodacode",
		Status: WorkspaceStatus{
			Git: &WorkspaceGitStatus{
				Branch:       "main",
				ChangedFiles: 3,
			},
		},
	})

	if len(sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(sections))
	}
	if sections[0].Key != deterministicContextPacketSectionRepo {
		t.Fatalf("first section key = %q, want repo", sections[0].Key)
	}
	if sections[1].Key != deterministicContextPacketSectionGit {
		t.Fatalf("second section key = %q, want git", sections[1].Key)
	}
	if !strings.Contains(sections[0].Content, "name: kodacode") {
		t.Fatalf("repo section content = %q", sections[0].Content)
	}
	if !strings.Contains(sections[1].Content, "branch: main") || !strings.Contains(sections[1].Content, "changed_files: 3") {
		t.Fatalf("git section content = %q", sections[1].Content)
	}
	if strings.Contains(sections[1].Content, "diff --git") || strings.Contains(sections[1].Content, "@@") {
		t.Fatalf("git section should not contain patch content: %q", sections[1].Content)
	}
}

func TestDeterministicContextPacketWorkspaceSectionsOmitGitWhenUnavailable(t *testing.T) {
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot: "/workspace/kodacode",
	})

	if len(sections) != 1 {
		t.Fatalf("sections = %d, want only repo", len(sections))
	}
	if sections[0].Key != deterministicContextPacketSectionRepo {
		t.Fatalf("section key = %q, want repo", sections[0].Key)
	}
	if !strings.Contains(sections[0].Content, "git: not_detected") {
		t.Fatalf("repo section content = %q", sections[0].Content)
	}
}

func TestBuildDeterministicContextPacketIncludesConfiguredWorkspaceSectionsOnly(t *testing.T) {
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot: "/workspace/kodacode",
		Status: WorkspaceStatus{
			Git: &WorkspaceGitStatus{
				Branch:       "main",
				ChangedFiles: 1,
				Changed: []WorkspaceGitChangedFile{{
					Path:   "README.md",
					Status: "??",
				}},
			},
		},
	})
	packet := buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: 20000,
		EnabledSections: []string{
			deterministicContextPacketSectionGit,
		},
		Sections: sections,
	})

	if len(packet.Sections) != 1 {
		t.Fatalf("packet sections = %d, want 1", len(packet.Sections))
	}
	if packet.Sections[0].Key != deterministicContextPacketSectionGit {
		t.Fatalf("packet section key = %q, want git", packet.Sections[0].Key)
	}
	if strings.Contains(packet.Content, "Repository Summary") {
		t.Fatalf("packet content included disabled repo section: %q", packet.Content)
	}
	if strings.Contains(packet.Content, "Dirty Git Summary") {
		t.Fatalf("packet content included disabled dirty summary section: %q", packet.Content)
	}
}

func TestDeterministicContextPacketWorkspaceSectionsBuildsBoundedDirtyGitSummary(t *testing.T) {
	changed := make([]WorkspaceGitChangedFile, 0, workspaceStatusMaxGitChangedFileEntries)
	for i := range workspaceStatusMaxGitChangedFileEntries {
		changed = append(changed, WorkspaceGitChangedFile{
			Path:   "file-" + string(rune('a'+i%26)) + ".go",
			Status: "M",
		})
	}
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot: "/workspace/kodacode",
		Status: WorkspaceStatus{
			Git: &WorkspaceGitStatus{
				Branch:       "main",
				ChangedFiles: workspaceStatusMaxGitChangedFileEntries + 2,
				Changed:      changed,
			},
		},
	})

	var section deterministicContextPacketSectionInput
	for _, candidate := range sections {
		if candidate.Key == deterministicContextPacketSectionGitDirtySummary {
			section = candidate
			break
		}
	}
	if section.Key == "" {
		t.Fatalf("dirty summary section missing: %#v", sections)
	}
	for _, want := range []string{
		"total_changed_files: 22",
		"listed_changed_files: 20",
		"omitted_changed_files: 2",
		"- M file-a.go",
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("dirty summary missing %q:\n%s", want, section.Content)
		}
	}
	if strings.Contains(section.Content, "diff --git") || strings.Contains(section.Content, "@@") {
		t.Fatalf("dirty summary should not contain patch content: %q", section.Content)
	}
}

func TestDeterministicContextPacketWorkspaceSectionsBuildsBoundedDiagnosticsSummary(t *testing.T) {
	diagnostics := make([]tool.CodeIntelDiagnostic, 0, deterministicContextPacketMaxDiagnostics+2)
	for i := range deterministicContextPacketMaxDiagnostics + 2 {
		diagnostics = append(diagnostics, tool.CodeIntelDiagnostic{
			Line:      i + 1,
			Character: i,
			Severity:  "error",
			Message:   "diagnostic message " + string(rune('a'+i%26)),
			Source:    "gopls",
		})
	}
	sections := deterministicContextPacketWorkspaceSections(deterministicContextPacketWorkspaceInput{
		WorkspaceRoot:            "/workspace/kodacode",
		DiagnosticCandidateFiles: deterministicContextPacketMaxDiagnosticFiles + 2,
		DiagnosticOmittedFiles:   2,
		Diagnostics: []tool.CodeIntelFileDiagnostics{
			{
				Path:        "/workspace/kodacode/internal/app/main.go",
				Diagnostics: diagnostics,
			},
		},
	})

	var section deterministicContextPacketSectionInput
	for _, candidate := range sections {
		if candidate.Key == deterministicContextPacketSectionDiagnostics {
			section = candidate
			break
		}
	}
	if section.Key == "" {
		t.Fatalf("diagnostics section missing: %#v", sections)
	}
	for _, want := range []string{
		"candidate_files: 5",
		"checked_files: 1",
		"omitted_files: 2",
		"diagnostics_found: 10",
		"omitted_diagnostics: 2",
		"- internal/app/main.go:1:0 [error] diagnostic message a (gopls)",
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("diagnostics summary missing %q:\n%s", want, section.Content)
		}
	}
	if strings.Contains(section.Content, "diagnostic message i") {
		t.Fatalf("diagnostics summary included diagnostics past the cap:\n%s", section.Content)
	}
}
