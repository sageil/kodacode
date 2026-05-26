package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkRenderReadTextFileLargeRange(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "notes.txt")
	lines := make([]string, 0, 10000)
	for i := 1; i <= 10000; i++ {
		lines = append(lines, fmt.Sprintf("line %05d %s", i, strings.Repeat("x", 32)))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		b.Fatalf("WriteFile() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := renderReadTextFile(path, "notes.txt", 50, 250); err != nil {
			b.Fatalf("renderReadTextFile() error = %v", err)
		}
	}
}
