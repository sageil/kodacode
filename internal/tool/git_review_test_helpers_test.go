package tool

import (
	"os/exec"
	"testing"
)

func ensureGitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	runGitTest(t, root, "init", "-q")
	runGitTest(t, root, "config", "user.email", "test@example.com")
	runGitTest(t, root, "config", "user.name", "Test User")
}

func gitCommitAll(t *testing.T, root, message string) {
	t.Helper()
	runGitTest(t, root, "add", ".")
	runGitTest(t, root, "commit", "-q", "-m", message)
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}
