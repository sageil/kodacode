// Package snapshot manages per-turn git snapshots for session time travel.
package snapshot

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Snapshot represents a single point-in-time capture of the working tree.
type Snapshot struct {
	TurnIndex  int       `json:"turn_index"`
	CommitHash string    `json:"commit_hash"`
	Summary    string    `json:"summary"`
	Files      []string  `json:"files"`
	CreatedAt  time.Time `json:"created_at"`
}

// Service manages per-session git shadow branches for time-travel snapshots.
type Service struct {
	projectDir string
}

func New(projectDir string) *Service {
	return &Service{projectDir: projectDir}
}

func (s *Service) branchName(sessionID string) string {
	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return "kodacode/" + prefix
}

func (s *Service) indexFile(sessionID string) string {
	prefix := sessionID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return filepath.Join(os.TempDir(), "kodacode-snapshot-"+prefix+".idx")
}

// IsGitRepo returns true if projectDir is inside a git work tree.
func (s *Service) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = s.projectDir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// Create commits the current working tree state to the shadow branch.
// Returns nil if not a git repo or no files changed.
func (s *Service) Create(sessionID string, turnIndex int, summary string) error {
	if !s.IsGitRepo() {
		return nil
	}

	idxFile := s.indexFile(sessionID)
	defer func() { _ = os.Remove(idxFile) }()

	branch := s.branchName(sessionID)
	env := append(os.Environ(), "GIT_INDEX_FILE="+idxFile)

	// Check if HEAD points to a valid commit (false for repos with no commits).
	_, headErr := s.gitOutput("rev-parse", "--verify", "HEAD")
	hasHead := headErr == nil

	if hasHead {
		if err := s.gitEnv(env, "read-tree", "HEAD"); err != nil {
			return fmt.Errorf("read-tree: %w", err)
		}
	}

	// Stage all working tree changes into the temp index.
	if err := s.gitEnv(env, "add", "-A"); err != nil {
		return fmt.Errorf("add: %w", err)
	}

	treeHash, err := s.gitOutputEnv(env, "write-tree")
	if err != nil {
		return fmt.Errorf("write-tree: %w", err)
	}

	if hasHead {
		headTree, _ := s.gitOutput("rev-parse", "HEAD^{tree}")
		if treeHash == headTree {
			return nil
		}
	}

	// Determine parent: tip of shadow branch if it exists, else HEAD.
	var parentArgs []string
	if parent, err := s.gitOutput("rev-parse", "--verify", "refs/heads/"+branch); err == nil {
		parentArgs = []string{"-p", parent}
	} else if hasHead {
		parent, _ := s.gitOutput("rev-parse", "HEAD")
		if parent != "" {
			parentArgs = []string{"-p", parent}
		}
	}

	msg := fmt.Sprintf("turn %d: %s", turnIndex, summary)
	args := append([]string{"commit-tree", treeHash}, parentArgs...)
	args = append(args, "-m", msg)
	commitHash, err := s.gitOutput(args...)
	if err != nil {
		return fmt.Errorf("commit-tree: %w", err)
	}

	if err := s.git("update-ref", "refs/heads/"+branch, commitHash); err != nil {
		return fmt.Errorf("update-ref: %w", err)
	}

	log.Printf("snapshot: created turn %d on %s (%s)", turnIndex, branch, commitHash[:8])
	return nil
}

// List returns all snapshots for a session by walking the shadow branch log.
func (s *Service) List(sessionID string) ([]Snapshot, error) {
	branch := s.branchName(sessionID)
	// Format: hash<TAB>subject<TAB>ISO date
	out, err := s.gitOutput("log", "--format=%H\t%s\t%aI", "refs/heads/"+branch)
	if err != nil {
		return nil, nil // branch doesn't exist
	}

	var snapshots []Snapshot
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		hash, subject, dateStr := parts[0], parts[1], parts[2]

		// Parse "turn N: summary" from subject.
		turnIndex := 0
		summary := subject
		if strings.HasPrefix(subject, "turn ") {
			if colonIdx := strings.Index(subject, ": "); colonIdx > 0 {
				if n, err := strconv.Atoi(subject[5:colonIdx]); err == nil {
					turnIndex = n
					summary = subject[colonIdx+2:]
				}
			}
		}

		createdAt, _ := time.Parse(time.RFC3339, dateStr)

		// Get changed files for this commit.
		filesOut, _ := s.gitOutput("diff-tree", "--no-commit-id", "--name-only", "-r", hash)
		var files []string
		if filesOut != "" {
			files = strings.Split(strings.TrimSpace(filesOut), "\n")
		}

		snapshots = append(snapshots, Snapshot{
			TurnIndex:  turnIndex,
			CommitHash: hash,
			Summary:    summary,
			Files:      files,
			CreatedAt:  createdAt,
		})
	}
	return snapshots, nil
}

// Restore fully restores the working tree to the state captured at turnIndex.
// It creates a safety snapshot first (and refuses to proceed if that fails),
// then restores tracked files from the snapshot and removes any tracked files
// that were added after the snapshot.
func (s *Service) Restore(sessionID string, turnIndex int) error {
	snapshots, err := s.List(sessionID)
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	var target *Snapshot
	for i := range snapshots {
		if snapshots[i].TurnIndex == turnIndex {
			target = &snapshots[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("snapshot for turn %d not found", turnIndex)
	}

	// Safety snapshot before restoring — fail closed if it can't be created.
	if err := s.Create(sessionID, -1, "pre-restore safety snapshot"); err != nil {
		return fmt.Errorf("safety snapshot failed (refusing to restore): %w", err)
	}

	// Step 1: Restore all tracked files from the snapshot commit.
	if err := s.git("checkout", target.CommitHash, "--", "."); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	// Step 2: Remove tracked files that exist now but didn't exist in the
	// snapshot. This prevents a hybrid state where newer files survive the
	// restore. Only tracked files are removed — untracked files (build
	// artifacts, .env, etc.) are left alone.
	snapshotFiles, err := s.gitOutput("ls-tree", "-r", "--name-only", target.CommitHash)
	if err != nil {
		return fmt.Errorf("ls-tree: %w", err)
	}
	currentFiles, err := s.gitOutput("ls-files")
	if err != nil {
		return fmt.Errorf("ls-files: %w", err)
	}

	snapshotSet := make(map[string]bool)
	if snapshotFiles != "" {
		for f := range strings.SplitSeq(snapshotFiles, "\n") {
			if f != "" {
				snapshotSet[f] = true
			}
		}
	}

	var extras []string
	if currentFiles != "" {
		for f := range strings.SplitSeq(currentFiles, "\n") {
			if f != "" && !snapshotSet[f] {
				extras = append(extras, f)
			}
		}
	}

	// Batch git rm calls to avoid exceeding ARG_MAX on large repos.
	const batchSize = 500
	for i := 0; i < len(extras); i += batchSize {
		end := min(i+batchSize, len(extras))
		args := append([]string{"rm", "-f", "--quiet", "--"}, extras[i:end]...)
		if err := s.git(args...); err != nil {
			return fmt.Errorf("remove extra files: %w", err)
		}
	}

	log.Printf("snapshot: restored turn %d (%s), removed %d extra files", turnIndex, target.CommitHash[:8], len(extras))
	return nil
}

// Cleanup deletes the shadow branch for a session.
func (s *Service) Cleanup(sessionID string) error {
	branch := s.branchName(sessionID)
	// Ignore error — branch may not exist.
	_ = s.git("branch", "-D", branch)
	log.Printf("snapshot: cleaned up branch %s", branch)
	return nil
}

func (s *Service) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *Service) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) gitEnv(env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.projectDir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (s *Service) gitOutputEnv(env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.projectDir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
