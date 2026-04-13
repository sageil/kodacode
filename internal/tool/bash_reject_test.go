package tool

import "testing"

func TestRejectBashFileOp(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		blocked bool
	}{
		// Heredoc patterns should be blocked
		{"cat heredoc", "cat > file.ts << 'EOF'\ncontent\nEOF", true},
		{"cat heredoc to tmp", "cat > /tmp/file.ts << EOF", true},
		{"cat to tmp then cp", "cat > /tmp/fix.py << 'PYEOF'\nimport sys\nPYEOF", true},
		{"heredoc with dash", "cat <<- 'END'\nstuff\nEND", true},
		{"heredoc no cat", "bash <<'SCRIPT'\necho hi\nSCRIPT", true},
		{"heredoc typescript", "cat > src/index.ts << 'TYPESCRIPT'\ncontent\nTYPESCRIPT", true},

		// Non-heredoc operations should pass
		{"cat read only", "cat tsconfig.json", false},
		{"cat -n read", "cat -n src/index.ts", false},
		{"cat write redirect", "cat > src/index.ts", false},
		{"echo redirect", "echo 'data' > output.txt", false},
		{"printf redirect", "printf '%s' data > output.txt", false},
		{"sed in-place", "sed -i 's/old/new/' file.txt", false},
		{"npm install", "npm install express", false},
		{"go build", "go build ./...", false},
		{"tsc check", "npx tsc --noEmit 2>&1", false},
		{"vitest run", "npx vitest run 2>&1", false},
		{"grep search", "grep -rn 'TODO' src/", false},
		{"mkdir", "mkdir -p src/services", false},
		{"docker build", "docker build -t myapp .", false},
		{"git status", "git status", false},
		{"pnpm build", "pnpm build 2>&1", false},
		{"stderr redirect", "command 2>&1", false},
		{"dev null redirect", "echo test > /dev/null", false},
		{"command with exit code", "npx vitest run 2>&1; echo \"EXIT_CODE: $?\"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := rejectBashFileOp(tt.cmd)
			got := msg != ""
			if got != tt.blocked {
				t.Errorf("rejectBashFileOp(%q) blocked=%v, want %v (msg=%q)", tt.cmd, got, tt.blocked, msg)
			}
		})
	}
}
