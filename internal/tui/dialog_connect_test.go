package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestBuildConnectEntriesIncludesEveryUnknownConnectedProvider(t *testing.T) {
	entries := buildConnectEntries(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{
			{ProviderID: "togetherai", BaseURL: "https://api.together.xyz/v1"},
			{ProviderID: "custom-one", BaseURL: "http://custom-one.invalid/v1"},
			{ProviderID: "custom-two", BaseURL: "http://custom-two.invalid/v1"},
		},
	})

	if !entryByProviderID(entries, "togetherai").connected {
		t.Fatal("togetherai should be marked connected")
	}
	if !entryByProviderID(entries, "custom-one").connected {
		t.Fatal("custom-one should be present as a distinct connected provider")
	}
	if !entryByProviderID(entries, "custom-two").connected {
		t.Fatal("custom-two should be present as a distinct connected provider")
	}
	if !entryByProviderID(entries, "custom").preset.Custom {
		t.Fatal("custom add-new entry should still be present")
	}
}

func TestBuildConnectEntriesIncludesNVIDIABuiltInPreset(t *testing.T) {
	entries := buildConnectEntries(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{
			{ProviderID: "nvidia", BaseURL: "https://integrate.api.nvidia.com/v1"},
		},
	})

	entry := entryByProviderID(entries, "nvidia")
	if entry.preset.Name != "NVIDIA" {
		t.Fatalf("preset name = %q", entry.preset.Name)
	}
	if !entry.preset.Native {
		t.Fatal("nvidia should be a native preset")
	}
	if !entry.connected {
		t.Fatal("nvidia should be marked connected")
	}
	if entry.baseURL != "https://integrate.api.nvidia.com/v1" {
		t.Fatalf("base url = %q", entry.baseURL)
	}
}

func TestBuildConnectEntriesIncludesQwenCloudPreset(t *testing.T) {
	entries := buildConnectEntries(app.DialogState{})

	entry := entryByProviderID(entries, "qwencloud")
	if entry.preset.Name != "QwenCloud" {
		t.Fatalf("preset name = %q", entry.preset.Name)
	}
	if entry.preset.BaseURL != "https://dashscope-intl.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("base url = %q", entry.preset.BaseURL)
	}
}

func TestBuildConnectEntriesIncludesOpenAIDefaultBaseURL(t *testing.T) {
	entries := buildConnectEntries(app.DialogState{})
	entry := entryByProviderID(entries, "openai")
	if entry.preset.BaseURL != provider.DefaultOpenAIBaseURL() {
		t.Fatalf("preset base url = %q", entry.preset.BaseURL)
	}
}

func TestConnectDialogOpenAISaveNormalizesOAuthURLForAPIKey(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	dialog := newConnectDialog([]connectDialogEntry{{
		preset: providerPreset{
			ID:      "openai",
			Name:    "OpenAI",
			BaseURL: provider.DefaultOpenAIBaseURL(),
		},
		connected: true,
		baseURL:   provider.DefaultOpenAIOAuthBaseURL(),
	}}, &defaultTheme)
	dialog.currentConnect = dialog.connectItems[0]
	dialog.baseURL.SetValue(provider.DefaultOpenAIOAuthBaseURL())
	dialog.apiKey.SetValue("next-key")

	_, cmd := dialog.activateConnectSave()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	closed, ok := msg.(dialogClosedMsg)
	if !ok {
		t.Fatalf("msg = %#v", msg)
	}
	result, ok := closed.result.(connectDialogResult)
	if !ok || result.Save == nil {
		t.Fatalf("result = %#v", closed.result)
	}
	if result.Save.BaseURL != provider.DefaultOpenAIBaseURL() {
		t.Fatalf("saved base url = %q, want %q", result.Save.BaseURL, provider.DefaultOpenAIBaseURL())
	}
}

func entryByProviderID(entries []connectDialogEntry, providerID string) connectDialogEntry {
	for _, entry := range entries {
		if entry.preset.ID == providerID {
			return entry
		}
	}
	return connectDialogEntry{}
}
