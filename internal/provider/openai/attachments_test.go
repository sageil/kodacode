package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestConvertMessageToResponseInput_EncodesImagesAndFiles(t *testing.T) {
	items := convertMessageToResponseInput(provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: "see files"},
			provider.FilePart{Path: "/tmp/diagram.png", MimeType: "image/png", URL: "data:image/png;base64,cG5n"},
			provider.FilePart{Path: "/tmp/spec.pdf", MimeType: "application/pdf", URL: "data:application/pdf;base64,cGRm"},
		},
	})

	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"type":"input_image"`) {
		t.Fatalf("responses input = %s, want input_image part", got)
	}
	if !strings.Contains(got, `"type":"input_file"`) {
		t.Fatalf("responses input = %s, want input_file part", got)
	}
	if !strings.Contains(got, `"filename":"spec.pdf"`) {
		t.Fatalf("responses input = %s, want filename for file part", got)
	}
}

func TestConvertUserMessages_UsesFileContentParts(t *testing.T) {
	msgs := convertUserMessages(provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: "inspect this"},
			provider.FilePart{Path: "/tmp/spec.pdf", MimeType: "application/pdf", URL: "data:application/pdf;base64,cGRm"},
		},
	})

	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"type":"file"`) {
		t.Fatalf("chat completions input = %s, want file content part", got)
	}
	if strings.Contains(got, "[File:") {
		t.Fatalf("chat completions input still contains text fallback: %s", got)
	}
}
