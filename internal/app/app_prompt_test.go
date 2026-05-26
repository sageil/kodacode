package app

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestPromptPermissionReturnsSessionScopeForChoiceTwo(t *testing.T) {
	var out bytes.Buffer
	decision, scope, grantPath, recursive, err := promptPermission(
		bufio.NewReader(strings.NewReader("2\n")),
		&out,
		events.PermissionRequestState{
			Kind:     events.PermissionRequestKindPath,
			Access:   "read",
			Path:     "/tmp/outside.txt",
			ToolName: "read",
			Command:  `read {"paths":["/tmp/outside.txt"]}`,
		},
	)
	if err != nil {
		t.Fatalf("promptPermission() error = %v", err)
	}
	if decision != events.PermissionDecisionApproved || scope != events.PermissionScopeSession {
		t.Fatalf("decision/scope = %q/%q", decision, scope)
	}
	if grantPath != "/tmp/outside.txt" || recursive {
		t.Fatalf("grantPath/recursive = %q/%v", grantPath, recursive)
	}
}
