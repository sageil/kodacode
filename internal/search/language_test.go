package search

import "testing"

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		reported string
		want     string
	}{
		{name: "reported language wins", path: "src/app.unknown", reported: "Go", want: "go"},
		{name: "typescript by extension", path: "src/app.ts", want: "typescript"},
		{name: "tsx by extension", path: "src/app.tsx", want: "typescript"},
		{name: "javascript by extension", path: "src/app.js", want: "javascript"},
		{name: "php by extension", path: "src/app.php", want: "php"},
		{name: "ruby by extension", path: "src/app.rb", want: "ruby"},
		{name: "lua by extension", path: "src/app.lua", want: "lua"},
		{name: "zig by extension", path: "src/app.zig", want: "zig"},
		{name: "swift by extension", path: "src/app.swift", want: "swift"},
		{name: "kotlin by extension", path: "src/app.kt", want: "kotlin"},
		{name: "c by extension", path: "src/app.c", want: "c"},
		{name: "csharp normalized", path: "src/app.cs", want: "csharp"},
		{name: "cpp normalized", path: "src/app.cpp", want: "cpp"},
		{name: "unknown extension", path: "README.md", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectLanguage(tt.path, tt.reported); got != tt.want {
				t.Fatalf("DetectLanguage(%q, %q) = %q, want %q", tt.path, tt.reported, got, tt.want)
			}
		})
	}
}
