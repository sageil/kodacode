package openai

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestAttachmentCapabilitiesNativeOpenAISupportsFiles(t *testing.T) {
	client := NewOpenAI("", "", nil)
	caps := client.AttachmentCapabilities(provider.Model{})
	if !caps.Images || !caps.PDFs || !caps.Text || !caps.Binary {
		t.Fatalf("native OpenAI caps = %+v, want full attachment support", caps)
	}
}

func TestAttachmentCapabilitiesCompatibleProviderAllowsSerializableAttachments(t *testing.T) {
	client := New("groq", "Groq", "", "https://example.com/v1", nil)
	caps := client.AttachmentCapabilities(provider.Model{})
	if !caps.Images || !caps.PDFs || !caps.Text || !caps.Binary {
		t.Fatalf("compatible provider caps = %+v, want optimistic attachment support", caps)
	}
}
