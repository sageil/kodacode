package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

const deterministicContextPacketMaxTokens = 2000

type deterministicContextPacketSectionInput struct {
	Key       string
	Label     string
	Source    string
	Freshness string
	Content   string
}

type deterministicContextPacketInput struct {
	ResolvedInputLimitTokens int
	InputLimitSource         string
	EnabledSections          []string
	Sections                 []deterministicContextPacketSectionInput
}

type deterministicContextPacketRequestInput struct {
	Request         provider.Request
	Models          modelCatalog
	EnabledSections []string
	Sections        []deterministicContextPacketSectionInput
}

type deterministicContextPacket struct {
	Content          string
	InputLimitTokens int
	InputLimitSource string
	TokenBudget      int
	Tokens           int
	Sections         []deterministicContextPacketSection
	Omitted          []deterministicContextPacketOmission
}

type deterministicContextPacketSection struct {
	Key       string
	Label     string
	Source    string
	Freshness string
	Content   string
	Tokens    int
	Bytes     int
}

type deterministicContextPacketOmission struct {
	Key       string
	Label     string
	Source    string
	Freshness string
	Reason    string
	Tokens    int
	Bytes     int
}

func buildDeterministicContextPacket(input deterministicContextPacketInput) deterministicContextPacket {
	packet := deterministicContextPacket{
		InputLimitTokens: max(input.ResolvedInputLimitTokens, 0),
		InputLimitSource: strings.TrimSpace(input.InputLimitSource),
		TokenBudget:      deterministicContextPacketTokenBudget(input.ResolvedInputLimitTokens),
	}
	if packet.TokenBudget <= 0 || len(input.EnabledSections) == 0 {
		return packet
	}

	enabled := deterministicContextPacketEnabledSections(input.EnabledSections)
	for _, sectionInput := range input.Sections {
		section, ok := normalizeDeterministicContextPacketSection(sectionInput)
		if !ok || !enabled[section.Key] {
			continue
		}
		candidateSections := append(slices.Clone(packet.Sections), section)
		candidateContent := renderDeterministicContextPacket(candidateSections)
		candidateTokens := provider.EstimateTextTokens(candidateContent)
		section.Tokens = provider.EstimateTextTokens(renderDeterministicContextPacketSection(section))
		if candidateTokens > packet.TokenBudget {
			packet.Omitted = append(packet.Omitted, deterministicContextPacketOmission{
				Key:       section.Key,
				Label:     section.Label,
				Source:    section.Source,
				Freshness: section.Freshness,
				Reason:    "token_budget",
				Tokens:    section.Tokens,
				Bytes:     len(section.Content),
			})
			continue
		}
		section.Bytes = len(section.Content)
		packet.Sections = append(packet.Sections, section)
		packet.Content = candidateContent
		packet.Tokens = candidateTokens
	}
	return packet
}

func buildDeterministicContextPacketForRequest(input deterministicContextPacketRequestInput) deterministicContextPacket {
	budget, ok := resolveModelInputBudgetForRequest(input.Request, input.Models)
	if !ok {
		return buildDeterministicContextPacket(deterministicContextPacketInput{
			EnabledSections: input.EnabledSections,
			Sections:        input.Sections,
		})
	}
	return buildDeterministicContextPacket(deterministicContextPacketInput{
		ResolvedInputLimitTokens: budget.InputLimitTokens,
		InputLimitSource:         budget.Source,
		EnabledSections:          input.EnabledSections,
		Sections:                 input.Sections,
	})
}

func deterministicContextPacketTokenBudget(inputLimitTokens int) int {
	if inputLimitTokens <= 0 {
		return 0
	}
	return min(max(inputLimitTokens/20, 1), deterministicContextPacketMaxTokens)
}

func deterministicContextPacketFragment(packet deterministicContextPacket) (prompt.Fragment, bool) {
	if strings.TrimSpace(packet.Content) == "" {
		return prompt.Fragment{}, false
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "deterministic-context-packet",
		Key:       "deterministic_context_packet",
		Label:     "Deterministic Context Packet",
		Content:   packet.Content,
	}, true
}

func deterministicContextPacketEnabledSections(keys []string) map[string]bool {
	enabled := make(map[string]bool, len(keys))
	for _, key := range keys {
		key = normalizeDeterministicContextPacketKey(key)
		if key == "" {
			continue
		}
		enabled[key] = true
	}
	return enabled
}

func normalizeDeterministicContextPacketSection(input deterministicContextPacketSectionInput) (deterministicContextPacketSection, bool) {
	key := normalizeDeterministicContextPacketKey(input.Key)
	content := strings.TrimSpace(input.Content)
	if key == "" || content == "" {
		return deterministicContextPacketSection{}, false
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = key
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "runtime"
	}
	freshness := strings.TrimSpace(input.Freshness)
	if freshness == "" {
		freshness = "current"
	}
	return deterministicContextPacketSection{
		Key:       key,
		Label:     label,
		Source:    source,
		Freshness: freshness,
		Content:   content,
	}, true
}

func normalizeDeterministicContextPacketKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func renderDeterministicContextPacket(sections []deterministicContextPacketSection) string {
	if len(sections) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sections)+1)
	parts = append(parts, "<deterministic_context_packet>")
	for _, section := range sections {
		parts = append(parts, renderDeterministicContextPacketSection(section))
	}
	parts = append(parts, "</deterministic_context_packet>")
	return strings.Join(parts, "\n")
}

func renderDeterministicContextPacketSection(section deterministicContextPacketSection) string {
	return fmt.Sprintf(
		"<section key=%q label=%q source=%q freshness=%q>\n%s\n</section>",
		section.Key,
		section.Label,
		section.Source,
		section.Freshness,
		section.Content,
	)
}
