package sandbox

import (
	"strings"
	"testing"
)

func TestAugmentPermissionDisplay_PrefersCommandForAmbiguousBash(t *testing.T) {
	t.Parallel()

	argsJSON := `{"command":"bash -c 'cat /etc/passwd'","description":"Check system file","purpose":"diagnostic"}`
	got := augmentPermissionDisplay(argsJSON, bashPathAnalysis{ambiguous: true})

	if !strings.Contains(got, `"permission_display":"bash -c 'cat /etc/passwd'"`) {
		t.Fatalf("permission display should show command, got %q", got)
	}
	if strings.Contains(got, "dynamic shell command") {
		t.Fatalf("permission display should not fall back to generic dynamic text, got %q", got)
	}
}

func TestAugmentPermissionDisplay_IncludesCommandAndExternalPathDetails(t *testing.T) {
	t.Parallel()

	argsJSON := `{"command":"bash -c 'cat /etc/passwd'","description":"Check system file","purpose":"diagnostic"}`
	got := augmentPermissionDisplay(argsJSON, bashPathAnalysis{
		externalPaths: []string{"/etc/passwd"},
		displayLines:  []string{"read: /etc/passwd"},
	})

	if !strings.Contains(got, `"permission_display":"bash -c 'cat /etc/passwd'\n\nread: /etc/passwd"`) {
		t.Fatalf("permission display should include command and path details, got %q", got)
	}
	if !strings.Contains(got, `"permission_paths":["/etc/passwd"]`) {
		t.Fatalf("permission paths should include resolved external path, got %q", got)
	}
}
