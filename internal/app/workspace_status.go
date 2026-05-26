package app

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	searchsvc "github.com/sageil/kodacode/internal/search"
)

const workspaceStatusGitTimeout = 3 * time.Second
const workspaceStatusMaxGitChangedFileEntries = 20

type WorkspaceStatus struct {
	Git    *WorkspaceGitStatus
	LSP    *WorkspaceLSPStatus
	Search *WorkspaceSearchStatus
}

type WorkspaceGitStatus struct {
	Branch       string
	ChangedFiles int
	Changed      []WorkspaceGitChangedFile
}

type WorkspaceGitChangedFile struct {
	Path   string
	Status string
}

type WorkspaceLSPStatus struct {
	ActiveServers []string
}

type WorkspaceSearchStatus struct {
	Configured        bool
	Tracking          bool
	Model             string
	PrewarmEmbeddings bool
	TrackedFiles      int
	IndexedFiles      int
	IndexedChunks     int
	PendingFiles      int
	LastRefreshAt     time.Time
	LastWarmupAt      time.Time
	LastWarmupError   string
}

func (r *Runtime) WorkspaceStatus(ctx context.Context, sessionID string) (WorkspaceStatus, error) {
	if strings.TrimSpace(sessionID) == "" {
		return WorkspaceStatus{}, ErrSessionIDRequired
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	status, err := loadWorkspaceStatus(ctx, state.WorkspaceRoot, r.Search)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	if r != nil && r.CodeIntel != nil {
		status.LSP = r.CodeIntel.WorkspaceServerStatus(state.WorkspaceRoot, state.AdditionalWorkspaceRoots)
	}
	return status, nil
}

func loadWorkspaceStatus(ctx context.Context, workspaceRoot string, search *searchsvc.Service) (WorkspaceStatus, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return WorkspaceStatus{}, nil
	}
	gitStatus, err := detectWorkspaceGitStatus(ctx, workspaceRoot)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	return WorkspaceStatus{
		Git:    gitStatus,
		Search: workspaceSearchStatus(search, workspaceRoot),
	}, nil
}

func detectWorkspaceGitStatus(ctx context.Context, workspaceRoot string) (*WorkspaceGitStatus, error) {
	statusCtx, cancel := context.WithTimeout(ctx, workspaceStatusGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(statusCtx, "git", "status", "--porcelain=v1", "--branch", "--untracked-files=all", "--", ".")
	cmd.Dir = workspaceRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	if err != nil {
		if strings.Contains(output, "not a git repository") {
			return nil, nil
		}
		return nil, err
	}
	return parseWorkspaceGitStatus(output), nil
}

func parseWorkspaceGitStatus(raw string) *WorkspaceGitStatus {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n")), "\n")
	if len(lines) == 0 {
		return nil
	}

	status := &WorkspaceGitStatus{}
	if head := strings.TrimSpace(lines[0]); strings.HasPrefix(head, "## ") {
		status.Branch = normalizeGitBranchLabel(strings.TrimSpace(strings.TrimPrefix(head, "## ")))
		lines = lines[1:]
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		status.ChangedFiles++
		if len(status.Changed) < workspaceStatusMaxGitChangedFileEntries {
			if changed, ok := parseWorkspaceGitChangedFile(line); ok {
				status.Changed = append(status.Changed, changed)
			}
		}
	}
	if status.Branch == "" && status.ChangedFiles == 0 {
		return nil
	}
	return status
}

func parseWorkspaceGitChangedFile(line string) (WorkspaceGitChangedFile, bool) {
	line = strings.TrimRight(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
	if strings.TrimSpace(line) == "" {
		return WorkspaceGitChangedFile{}, false
	}
	statusCode := strings.TrimSpace(line[:min(len(line), 2)])
	if statusCode == "" {
		statusCode = "changed"
	}
	path := ""
	if len(line) > 3 {
		path = strings.TrimSpace(line[3:])
	} else if len(line) > 2 {
		path = strings.TrimSpace(line[2:])
	}
	if path == "" {
		return WorkspaceGitChangedFile{}, false
	}
	if idx := strings.LastIndex(path, " -> "); idx >= 0 {
		path = strings.TrimSpace(path[idx+4:])
	}
	path = strings.ReplaceAll(path, "\n", " ")
	path = strings.TrimSpace(path)
	if path == "" {
		return WorkspaceGitChangedFile{}, false
	}
	return WorkspaceGitChangedFile{
		Path:   path,
		Status: statusCode,
	}, true
}

func normalizeGitBranchLabel(label string) string {
	label = strings.TrimSpace(label)
	switch {
	case label == "":
		return ""
	case strings.HasPrefix(label, "No commits yet on "):
		return strings.TrimSpace(strings.TrimPrefix(label, "No commits yet on "))
	case strings.HasPrefix(label, "HEAD (no branch)"):
		return "detached"
	}
	if idx := strings.Index(label, "..."); idx >= 0 {
		label = strings.TrimSpace(label[:idx])
	}
	if label == "" {
		return ""
	}
	return label
}

func workspaceSearchStatus(service *searchsvc.Service, workspaceRoot string) *WorkspaceSearchStatus {
	if service == nil {
		return nil
	}
	status := service.WorkspaceStatus(workspaceRoot)
	return &WorkspaceSearchStatus{
		Configured:        status.Configured,
		Tracking:          status.Tracking,
		Model:             status.Model.String(),
		PrewarmEmbeddings: status.PrewarmEmbeddings,
		TrackedFiles:      status.TrackedFiles,
		IndexedFiles:      status.IndexedFiles,
		IndexedChunks:     status.IndexedChunks,
		PendingFiles:      status.PendingFiles,
		LastRefreshAt:     status.LastRefreshAt,
		LastWarmupAt:      status.LastWarmupAt,
		LastWarmupError:   status.LastWarmupError,
	}
}
