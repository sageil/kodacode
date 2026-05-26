package provider

import "strings"

const (
	ReasoningVariantMinimal = "minimal"
	ReasoningVariantNone    = "none"
	ReasoningVariantLow     = "low"
	ReasoningVariantMedium  = "medium"
	ReasoningVariantHigh    = "high"
	ReasoningVariantXHigh   = "xhigh"
	ReasoningVariantMax     = "max"
)

func ReasoningVariantNames() []string {
	return []string{
		ReasoningVariantMinimal,
		ReasoningVariantNone,
		ReasoningVariantLow,
		ReasoningVariantMedium,
		ReasoningVariantHigh,
		ReasoningVariantXHigh,
		ReasoningVariantMax,
	}
}

func NormalizeReasoningVariant(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", true
	case ReasoningVariantMinimal:
		return ReasoningVariantMinimal, true
	case ReasoningVariantNone:
		return ReasoningVariantNone, true
	case ReasoningVariantLow:
		return ReasoningVariantLow, true
	case ReasoningVariantMedium:
		return ReasoningVariantMedium, true
	case ReasoningVariantHigh:
		return ReasoningVariantHigh, true
	case ReasoningVariantXHigh:
		return ReasoningVariantXHigh, true
	case ReasoningVariantMax:
		return ReasoningVariantMax, true
	default:
		return "", false
	}
}
