package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAttachment_SizeLimit(t *testing.T) {
	dir := t.TempDir()

	// Create a file that's 5 bytes.
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should pass with a 10-byte limit.
	att, err := ValidateAttachment(small, 10)
	if err != nil {
		t.Fatalf("expected no error for small file, got: %v", err)
	}
	if att.Name != "small.txt" {
		t.Errorf("Name = %q, want %q", att.Name, "small.txt")
	}

	// Should fail with a 3-byte limit.
	_, err = ValidateAttachment(small, 3)
	if err == nil {
		t.Fatal("expected error for file exceeding max size")
	}
}

func TestValidateAttachment_DefaultSize(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Zero maxSize should use the 20 MB default, so a 1-byte file passes.
	if _, err := ValidateAttachment(f, 0); err != nil {
		t.Fatalf("unexpected error with default size: %v", err)
	}
}

func TestValidateAttachment_Directory(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateAttachment(dir, 0)
	if err == nil {
		t.Fatal("expected error when attaching a directory")
	}
}

func TestValidateAttachment_NotFound(t *testing.T) {
	_, err := ValidateAttachment("/nonexistent/file.txt", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFormatAttachmentLabel(t *testing.T) {
	tests := []struct {
		name string
		att  Attachment
		want string
	}{
		{
			name: "file",
			att:  Attachment{Name: "notes.txt", MimeType: "text/plain"},
			want: "[File notes.txt #1]",
		},
		{
			name: "image",
			att:  Attachment{Name: "image.png", MimeType: "image/png"},
			want: "[Image image.png #2]",
		},
		{
			name: "pdf",
			att:  Attachment{Name: "mypdf.pdf", MimeType: "application/pdf"},
			want: "[pdf mypdf.pdf #3]",
		},
	}
	for i, tc := range tests {
		if got := formatAttachmentLabel(i+1, tc.att); got != tc.want {
			t.Fatalf("%s: formatAttachmentLabel() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
