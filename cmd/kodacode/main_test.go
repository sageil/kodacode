package main

import "testing"

func TestShouldPrintBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "long flag", args: []string{"--version"}, want: true},
		{name: "command", args: []string{"version"}, want: true},
		{name: "no args", args: nil, want: false},
		{name: "version with prompt text", args: []string{"--version", "inspect"}, want: false},
		{name: "short flag unsupported", args: []string{"-v"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPrintBuildInfo(tt.args); got != tt.want {
				t.Fatalf("shouldPrintBuildInfo(%#v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
