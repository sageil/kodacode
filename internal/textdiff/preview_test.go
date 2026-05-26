package textdiff

import "testing"

func TestBuildPreviewTracksAnchorsAcrossSkippedContext(t *testing.T) {
	before := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\n"
	after := "line 1\nline 2\nline 3\nline 4 updated\nline 5\nline 6\nline 7\n"

	preview := BuildPreview(before, after, 1)
	if preview.OldStartLine != 3 || preview.NewStartLine != 3 {
		t.Fatalf("preview start = %d,%d", preview.OldStartLine, preview.NewStartLine)
	}
	if len(preview.Ops) != 4 {
		t.Fatalf("preview ops = %#v", preview.Ops)
	}
	if preview.Ops[0].Kind != OpContext || preview.Ops[0].Text != "line 3" {
		t.Fatalf("preview ops[0] = %#v", preview.Ops[0])
	}
	if preview.Ops[1].Kind != OpDelete || preview.Ops[1].Text != "line 4" {
		t.Fatalf("preview ops[1] = %#v", preview.Ops[1])
	}
	if preview.Ops[2].Kind != OpInsert || preview.Ops[2].Text != "line 4 updated" {
		t.Fatalf("preview ops[2] = %#v", preview.Ops[2])
	}
	if preview.Ops[3].Kind != OpContext || preview.Ops[3].Text != "line 5" {
		t.Fatalf("preview ops[3] = %#v", preview.Ops[3])
	}
}

func TestBuildPreviewAddsSkipMarkerBetweenHunks(t *testing.T) {
	before := "a\nb\nc\nd\ne\nf\ng\nh\ni\n"
	after := "a\nB\nc\nd\ne\nf\nG\nh\ni\n"

	preview := BuildPreview(before, after, 1)
	if len(preview.Ops) == 0 {
		t.Fatal("preview ops = nil")
	}
	foundSkip := false
	for _, op := range preview.Ops {
		if op.Kind == OpSkip {
			foundSkip = true
			break
		}
	}
	if !foundSkip {
		t.Fatalf("preview ops = %#v, want skip marker", preview.Ops)
	}
	added, removed := LineStats(preview)
	if added != 2 || removed != 2 {
		t.Fatalf("line stats = +%d -%d", added, removed)
	}
}

func TestBuildPreviewReturnsEmptyPreviewWithoutChanges(t *testing.T) {
	preview := BuildPreview("same\n", "same\n", 2)
	if preview.OldStartLine != 0 || preview.NewStartLine != 0 || len(preview.Ops) != 0 {
		t.Fatalf("preview = %#v", preview)
	}
	if HasChanges(preview) {
		t.Fatalf("HasChanges(%#v) = true, want false", preview)
	}
}
