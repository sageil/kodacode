package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/tool"
)

const (
	deterministicContextPacketSectionRepo            = "repo"
	deterministicContextPacketSectionGit             = "git"
	deterministicContextPacketSectionGitDirtySummary = "git_dirty_summary"
	deterministicContextPacketSectionDiagnostics     = "diagnostics"
	deterministicContextPacketMaxDiagnosticFiles     = 3
	deterministicContextPacketMaxDiagnostics         = 8
	deterministicContextPacketMaxDiagnosticMessage   = 160
)

type deterministicContextPacketWorkspaceInput struct {
	WorkspaceRoot            string
	Status                   WorkspaceStatus
	Diagnostics              []tool.CodeIntelFileDiagnostics
	DiagnosticCandidateFiles int
	DiagnosticOmittedFiles   int
}

func deterministicContextPacketWorkspaceSections(input deterministicContextPacketWorkspaceInput) []deterministicContextPacketSectionInput {
	sections := make([]deterministicContextPacketSectionInput, 0, 4)
	if section, ok := deterministicContextPacketRepoSection(input); ok {
		sections = append(sections, section)
	}
	if section, ok := deterministicContextPacketGitSection(input.Status.Git); ok {
		sections = append(sections, section)
	}
	if section, ok := deterministicContextPacketGitDirtySummarySection(input.Status.Git); ok {
		sections = append(sections, section)
	}
	if section, ok := deterministicContextPacketDiagnosticsSection(input); ok {
		sections = append(sections, section)
	}
	return sections
}

func deterministicContextPacketRepoSection(input deterministicContextPacketWorkspaceInput) (deterministicContextPacketSectionInput, bool) {
	root := strings.TrimSpace(input.WorkspaceRoot)
	if root == "" && input.Status.Git == nil {
		return deterministicContextPacketSectionInput{}, false
	}
	name := strings.TrimSpace(filepath.Base(filepath.Clean(root)))
	if root == "" || name == "." || name == string(filepath.Separator) {
		name = "workspace"
	}
	lines := []string{
		"name: " + name,
	}
	if input.Status.Git != nil {
		lines = append(lines, "git: detected")
	} else {
		lines = append(lines, "git: not_detected")
	}
	return deterministicContextPacketSectionInput{
		Key:       deterministicContextPacketSectionRepo,
		Label:     "Repository Summary",
		Source:    "workspace metadata",
		Freshness: "current",
		Content:   strings.Join(lines, "\n"),
	}, true
}

func deterministicContextPacketGitSection(status *WorkspaceGitStatus) (deterministicContextPacketSectionInput, bool) {
	if status == nil {
		return deterministicContextPacketSectionInput{}, false
	}
	lines := make([]string, 0, 2)
	if branch := strings.TrimSpace(status.Branch); branch != "" {
		lines = append(lines, "branch: "+branch)
	}
	lines = append(lines, fmt.Sprintf("changed_files: %d", max(status.ChangedFiles, 0)))
	return deterministicContextPacketSectionInput{
		Key:       deterministicContextPacketSectionGit,
		Label:     "Git Summary",
		Source:    "git status --porcelain=v1 --branch",
		Freshness: "current",
		Content:   strings.Join(lines, "\n"),
	}, true
}

func deterministicContextPacketGitDirtySummarySection(status *WorkspaceGitStatus) (deterministicContextPacketSectionInput, bool) {
	if status == nil || status.ChangedFiles <= 0 || len(status.Changed) == 0 {
		return deterministicContextPacketSectionInput{}, false
	}
	lines := []string{
		fmt.Sprintf("total_changed_files: %d", max(status.ChangedFiles, 0)),
		fmt.Sprintf("listed_changed_files: %d", len(status.Changed)),
	}
	if omitted := status.ChangedFiles - len(status.Changed); omitted > 0 {
		lines = append(lines, fmt.Sprintf("omitted_changed_files: %d", omitted))
	}
	lines = append(lines, "files:")
	for _, changed := range status.Changed {
		path := strings.TrimSpace(changed.Path)
		if path == "" {
			continue
		}
		statusCode := strings.TrimSpace(changed.Status)
		if statusCode == "" {
			statusCode = "changed"
		}
		lines = append(lines, fmt.Sprintf("- %s %s", statusCode, path))
	}
	if len(lines) == 3 {
		return deterministicContextPacketSectionInput{}, false
	}
	return deterministicContextPacketSectionInput{
		Key:       deterministicContextPacketSectionGitDirtySummary,
		Label:     "Dirty Git Summary",
		Source:    "git status --porcelain=v1 --branch --untracked-files=all",
		Freshness: "current",
		Content:   strings.Join(lines, "\n"),
	}, true
}

func deterministicContextPacketDiagnosticsSection(input deterministicContextPacketWorkspaceInput) (deterministicContextPacketSectionInput, bool) {
	if input.DiagnosticCandidateFiles <= 0 && len(input.Diagnostics) == 0 {
		return deterministicContextPacketSectionInput{}, false
	}
	lines := []string{
		fmt.Sprintf("candidate_files: %d", max(input.DiagnosticCandidateFiles, len(input.Diagnostics))),
		fmt.Sprintf("checked_files: %d", len(input.Diagnostics)),
	}
	if input.DiagnosticOmittedFiles > 0 {
		lines = append(lines, fmt.Sprintf("omitted_files: %d", input.DiagnosticOmittedFiles))
	}

	totalDiagnostics := 0
	omittedDiagnostics := 0
	fileLines := make([]string, 0, len(input.Diagnostics))
	for _, file := range input.Diagnostics {
		displayPath := deterministicContextPacketDisplayPath(input.WorkspaceRoot, file.Path)
		if displayPath == "" {
			continue
		}
		if message := strings.TrimSpace(file.Error); message != "" {
			fileLines = append(fileLines, fmt.Sprintf("- %s: unavailable: %s", displayPath, truncateContextPacketLine(message, deterministicContextPacketMaxDiagnosticMessage)))
			continue
		}
		fileDiagnostics := 0
		for _, diagnostic := range file.Diagnostics {
			if strings.TrimSpace(diagnostic.Message) == "" {
				continue
			}
			fileDiagnostics++
			totalDiagnostics++
			if totalDiagnostics > deterministicContextPacketMaxDiagnostics {
				omittedDiagnostics++
				continue
			}
			severity := strings.TrimSpace(diagnostic.Severity)
			if severity == "" {
				severity = "diagnostic"
			}
			source := strings.TrimSpace(diagnostic.Source)
			if source == "" {
				source = "lsp"
			}
			fileLines = append(fileLines, fmt.Sprintf("- %s:%d:%d [%s] %s (%s)",
				displayPath,
				max(diagnostic.Line, 0),
				max(diagnostic.Character, 0),
				severity,
				truncateContextPacketLine(diagnostic.Message, deterministicContextPacketMaxDiagnosticMessage),
				source,
			))
		}
		if fileDiagnostics == 0 {
			fileLines = append(fileLines, fmt.Sprintf("- %s: no diagnostics", displayPath))
		}
	}
	lines = append(lines, fmt.Sprintf("diagnostics_found: %d", totalDiagnostics))
	if omittedDiagnostics > 0 {
		lines = append(lines, fmt.Sprintf("omitted_diagnostics: %d", omittedDiagnostics))
	}
	if len(fileLines) > 0 {
		lines = append(lines, "files:")
		lines = append(lines, fileLines...)
	}
	return deterministicContextPacketSectionInput{
		Key:       deterministicContextPacketSectionDiagnostics,
		Label:     "Diagnostics Summary",
		Source:    "LSP diagnostics for bounded changed files",
		Freshness: "current",
		Content:   strings.Join(lines, "\n"),
	}, true
}

func deterministicContextPacketDiagnosticCandidatePaths(workspaceRoot string, status WorkspaceStatus) ([]string, int) {
	if status.Git == nil || len(status.Git.Changed) == 0 {
		return nil, 0
	}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return nil, 0
	}
	paths := make([]string, 0, min(len(status.Git.Changed), deterministicContextPacketMaxDiagnosticFiles))
	seen := map[string]bool{}
	candidates := 0
	for _, changed := range status.Git.Changed {
		path := strings.TrimSpace(changed.Path)
		if path == "" || deterministicContextPacketDeletedGitStatus(changed.Status) {
			continue
		}
		abs := filepath.Join(root, path)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		candidates++
		if len(paths) >= deterministicContextPacketMaxDiagnosticFiles || seen[abs] {
			continue
		}
		seen[abs] = true
		paths = append(paths, abs)
	}
	return paths, candidates
}

func deterministicContextPacketDeletedGitStatus(status string) bool {
	status = strings.TrimSpace(status)
	return strings.Contains(status, "D")
}

func deterministicContextPacketDisplayPath(workspaceRoot, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	return path
}

func truncateContextPacketLine(text string, maxLen int) string {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " "))
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return text[:maxLen]
	}
	return strings.TrimSpace(text[:maxLen-3]) + "..."
}
