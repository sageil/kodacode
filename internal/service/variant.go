package service

// VariantConfig holds the thinking parameters for a variant level.
type VariantConfig struct {
	Budget int    // token budget (Anthropic, Google)
	Effort string // effort level (OpenAI: "low"/"medium"/"high", Google: "LOW"/"MEDIUM"/"HIGH")
}

// Variants maps effort level names to thinking parameters.
// "adaptive" is not in the map — it signals the OAuth middleware to use
// adaptive thinking (model decides depth). Budget 0 + not-found = adaptive.
var Variants = map[string]VariantConfig{
	"low":  {Budget: 3000, Effort: "low"},
	"high": {Budget: 10000, Effort: "medium"},
	"max":  {Budget: 32000, Effort: "high"},
}

// VariantNames is the ordered list of variant names for cycling.
// "adaptive" lets the model decide thinking depth (Anthropic OAuth default).
var VariantNames = []string{"adaptive", "low", "high", "max"}

// VariantBudget returns the thinking token budget for a variant name.
// Returns (0, false) if the variant is empty or unknown.
func VariantBudget(variant string) (int, bool) {
	if variant == "" {
		return 0, false
	}
	v, ok := Variants[variant]
	return v.Budget, ok
}

// VariantEffort returns the reasoning effort level for a variant name.
// Returns ("", false) if the variant is empty or unknown.
func VariantEffort(variant string) (string, bool) {
	if variant == "" {
		return "", false
	}
	v, ok := Variants[variant]
	return v.Effort, ok
}

// AutoReduceReasoningBudget returns a reduced reasoning budget for tool-dispatch
// turns (step > 1). When the model is routing tool results rather than deep-thinking,
// a low budget (3K tokens) is sufficient. Returns nil if no reduction is needed.
func AutoReduceReasoningBudget(step int, agentBudget *int) *int {
	if step <= 1 {
		return agentBudget // first turn always gets full budget
	}
	low := Variants["low"].Budget
	if agentBudget == nil || *agentBudget <= low {
		return agentBudget // already at or below low budget
	}
	return &low
}

// ContextAwareReasoningBudget scales the reasoning budget down when context
// usage exceeds 70%. Uses the same linear scale as tool output adaptive limits:
// 100% at ≤70% usage, 25% at ≥90% usage.
func ContextAwareReasoningBudget(budget *int, contextUsage float64) *int {
	if budget == nil || *budget == 0 || contextUsage < 0.7 {
		return budget
	}
	scale := 1.0 - (contextUsage-0.7)/0.3*0.75
	if scale < 0.25 {
		scale = 0.25
	}
	reduced := max(int(float64(*budget)*scale), Variants["low"].Budget)
	return &reduced
}
