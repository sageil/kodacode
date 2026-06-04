package tui

import (
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

type themeItem struct {
	Name        string
	DisplayName string
}

type agentItem struct {
	ID          string
	Description string
}

type workflowItem struct {
	ID          string
	Description string
	None        bool
}

type skillItem struct {
	ID          string
	Description string
	WhenToUse   string
	Source      string
}

type modelItem struct {
	Ref          provider.ModelRef
	ProviderName string
	ModelName    string
	Capacity     provider.ModelCapacity
	CostInput    float64
	CostOutput   float64
	Reasoning    bool
	ToolCalls    bool
	Vision       bool
	Exact        bool
}

type providerPreset struct {
	ID             string
	Name           string
	BaseURL        string
	APIKeyLabel    string
	APIKeyOptional bool
	Native         bool
	Custom         bool
}

type connectDialogEntry struct {
	preset    providerPreset
	connected bool
	baseURL   string
}

var connectProviderPresets = []providerPreset{
	{ID: "openai", Name: "OpenAI", BaseURL: provider.DefaultOpenAIBaseURL(), APIKeyLabel: "API key", Native: true},
	{ID: "anthropic", Name: "Anthropic", BaseURL: "https://api.anthropic.com", APIKeyLabel: "API key", Native: true},
	{ID: "google", Name: "Google", BaseURL: "https://generativelanguage.googleapis.com", APIKeyLabel: "API key", Native: true},
	{ID: "nvidia", Name: "NVIDIA", BaseURL: "https://integrate.api.nvidia.com/v1", APIKeyLabel: "API key", Native: true},
	{ID: "github-copilot", Name: "GitHub Copilot", BaseURL: "https://api.githubcopilot.com", APIKeyLabel: "Copilot token", Native: true},
	{ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com", APIKeyLabel: "API key"},
	{ID: "qwencloud", Name: "QwenCloud", BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", APIKeyLabel: "API key"},
	{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", APIKeyLabel: "API key"},
	{ID: "togetherai", Name: "Together AI", BaseURL: "https://api.together.xyz/v1", APIKeyLabel: "API key"},
	{ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", APIKeyLabel: "API key"},
	{ID: "fireworks-ai", Name: "Fireworks AI", BaseURL: "https://api.fireworks.ai/inference/v1", APIKeyLabel: "API key"},
	{ID: "mistral", Name: "Mistral", BaseURL: "https://api.mistral.ai/v1", APIKeyLabel: "API key"},
	{ID: "cerebras", Name: "Cerebras", BaseURL: "https://api.cerebras.ai/v1", APIKeyLabel: "API key"},
	{ID: "deepinfra", Name: "Deep Infra", BaseURL: "https://api.deepinfra.com/v1/openai", APIKeyLabel: "API key"},
	{ID: "moonshotai", Name: "Moonshot AI (Kimi)", BaseURL: "https://api.moonshot.ai/v1", APIKeyLabel: "API key"},
	{ID: "venice", Name: "Venice AI", BaseURL: "https://api.venice.ai/api/v1", APIKeyLabel: "API key"},
	{ID: "zai-coding-plan", Name: "Z.AI", BaseURL: "https://api.z.ai/api/coding/paas/v4", APIKeyLabel: "API key"},
	{ID: "ollama-cloud", Name: "Ollama Cloud", BaseURL: "https://ollama.com/v1", APIKeyLabel: "API key"},
	{ID: "ollama", Name: "Ollama", BaseURL: "http://localhost:11434/v1", APIKeyLabel: "API key (optional)", APIKeyOptional: true},
	{ID: "lmstudio", Name: "LM Studio", BaseURL: "http://localhost:1234/v1", APIKeyLabel: "API key (optional)", APIKeyOptional: true},
	{ID: "llamacpp", Name: "llama.cpp", BaseURL: "http://localhost:8080/v1", APIKeyLabel: "API key (optional)", APIKeyOptional: true},
	{ID: "custom", Name: "Custom Compatible", APIKeyLabel: "API key", Custom: true},
}

func buildThemeItems(names []string) []themeItem {
	items := make([]themeItem, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		items = append(items, themeItem{Name: name, DisplayName: name})
	}
	return items
}

func buildAgentItems(agents []app.AvailableAgent) []agentItem {
	items := make([]agentItem, 0, len(agents))
	for _, agent := range agents {
		id := strings.TrimSpace(agent.ID)
		if id == "" {
			continue
		}
		items = append(items, agentItem{
			ID:          id,
			Description: strings.TrimSpace(agent.Description),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func buildWorkflowItems(workflows []app.AvailableWorkflow) []workflowItem {
	items := make([]workflowItem, 0, len(workflows)+1)
	items = append(items, workflowItem{
		ID:          "",
		Description: "Run without a workflow",
		None:        true,
	})
	for _, workflow := range workflows {
		id := strings.TrimSpace(workflow.ID)
		if id == "" {
			continue
		}
		items = append(items, workflowItem{
			ID:          id,
			Description: strings.TrimSpace(workflow.Description),
		})
	}
	sort.Slice(items[1:], func(i, j int) bool { return items[i+1].ID < items[j+1].ID })
	return items
}

func buildSkillItems(skills []app.AvailableSkill) []skillItem {
	items := make([]skillItem, 0, len(skills))
	for _, available := range skills {
		id := strings.TrimSpace(available.ID)
		if id == "" {
			continue
		}
		items = append(items, skillItem{
			ID:          id,
			Description: strings.TrimSpace(available.Description),
			WhenToUse:   strings.TrimSpace(available.WhenToUse),
			Source:      strings.TrimSpace(available.Source),
		})
	}
	return items
}

func buildModelItems(state app.DialogState, selected provider.ModelRef) []modelItem {
	seen := map[string]struct{}{}
	dynamicByProvider := make(map[string][]modelItem)
	for _, available := range state.AvailableModels {
		providerID := strings.TrimSpace(available.Ref.ProviderID)
		if providerID == "" {
			continue
		}
		dynamicByProvider[providerID] = append(dynamicByProvider[providerID], modelItem{
			Ref:          available.Ref,
			ProviderName: available.ProviderName,
			ModelName:    available.ModelName,
			Capacity:     available.Capacity,
			CostInput:    available.CostInput,
			CostOutput:   available.CostOutput,
			Reasoning:    available.Reasoning,
			ToolCalls:    available.ToolCalls,
			Vision:       available.Vision,
		})
	}
	var items []modelItem
	for _, providerState := range state.ConnectedProviders {
		providerID := strings.TrimSpace(providerState.ProviderID)
		for _, item := range dynamicByProvider[providerID] {
			key := item.Ref.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ProviderName == items[j].ProviderName {
			return items[i].ModelName < items[j].ModelName
		}
		return items[i].ProviderName < items[j].ProviderName
	})
	if strings.TrimSpace(selected.ProviderID) == "" || strings.TrimSpace(selected.ModelID) == "" {
		selected = state.ModelRoute.Primary
	}
	if ref := selected; strings.TrimSpace(ref.ProviderID) != "" && strings.TrimSpace(ref.ModelID) != "" {
		if _, ok := seen[ref.String()]; !ok {
			items = append([]modelItem{{
				Ref:          ref,
				ProviderName: providerDisplayName(ref.ProviderID),
				ModelName:    ref.ModelID,
				Exact:        true,
			}}, items...)
		}
	}
	return items
}

func providerPresetByID(providerID string) (providerPreset, bool) {
	for _, preset := range connectProviderPresets {
		if preset.ID == providerID {
			return preset, true
		}
	}
	return providerPreset{}, false
}

func connectedProviderMap(state app.DialogState) map[string]app.ConnectedProvider {
	connected := make(map[string]app.ConnectedProvider, len(state.ConnectedProviders))
	for _, providerState := range state.ConnectedProviders {
		connected[providerState.ProviderID] = providerState
	}
	return connected
}

func providerDisplayName(providerID string) string {
	if preset, ok := providerPresetByID(providerID); ok {
		return preset.Name
	}
	return providerID
}

func buildConnectEntries(state app.DialogState) []connectDialogEntry {
	connected := connectedProviderMap(state)
	entries := make([]connectDialogEntry, 0, len(connectProviderPresets)+len(connected))
	customPreset, _ := providerPresetByID("custom")
	for _, preset := range connectProviderPresets {
		entry := connectDialogEntry{preset: preset}
		if providerState, ok := connected[preset.ID]; ok {
			entry.connected = true
			entry.baseURL = providerState.BaseURL
		}
		entries = append(entries, entry)
	}
	for providerID, providerState := range connected {
		if _, known := providerPresetByID(providerID); known {
			continue
		}
		entries = append(entries, connectDialogEntry{
			preset: providerPreset{
				ID:             providerID,
				Name:           providerID,
				BaseURL:        providerState.BaseURL,
				APIKeyLabel:    customPreset.APIKeyLabel,
				APIKeyOptional: customPreset.APIKeyOptional,
			},
			connected: true,
			baseURL:   providerState.BaseURL,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].connected != entries[j].connected {
			return entries[i].connected
		}
		return entries[i].preset.Name < entries[j].preset.Name
	})
	return entries
}

func pickFirstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
