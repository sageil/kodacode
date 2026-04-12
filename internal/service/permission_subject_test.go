package service

import "testing"

func TestPermissionPathSubject_NormalizesProjectAndExternalPaths(t *testing.T) {
	root := t.TempDir()

	if got := permissionPathSubject("docs/readme.md", root); got != "project:docs/readme.md" {
		t.Fatalf("relative project path subject = %q, want %q", got, "project:docs/readme.md")
	}

	if got := permissionPathSubject("/tmp/kodacode-review.txt", root); got != "external:/tmp/kodacode-review.txt" {
		t.Fatalf("external path subject = %q, want %q", got, "external:/tmp/kodacode-review.txt")
	}
}

func TestBashPermissionSubject_NormalizesCommandAndWorkdir(t *testing.T) {
	root := t.TempDir()
	input := `{"command":"  echo   'hello world'  ","workdir":"subdir"}`

	got := bashPermissionSubject(input, root)
	want := "echo 'hello world' [workdir=project:subdir]"
	if got != want {
		t.Fatalf("bashPermissionSubject() = %q, want %q", got, want)
	}
}

func TestWebFetchPermissionSubject_UsesOrigin(t *testing.T) {
	input := `{"url":"https://docs.example.com/guide?lang=en#intro"}`
	got := webFetchPermissionSubject(input)
	want := "https://docs.example.com"
	if got != want {
		t.Fatalf("webFetchPermissionSubject() = %q, want %q", got, want)
	}
}

func TestSessionPermissionBroker_PathDeniesApplyToChildren(t *testing.T) {
	broker := newSessionPermissionBroker()
	broker.DenyPath("s1", "project:docs")

	if !broker.IsPathDenied("s1", "", "project:docs/readme.md") {
		t.Fatal("expected child path under denied project directory to be denied")
	}
	if broker.IsPathDenied("s1", "", "project:src/main.go") {
		t.Fatal("unexpected deny outside denied subtree")
	}
}
