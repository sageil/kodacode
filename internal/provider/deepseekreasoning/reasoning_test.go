package deepseekreasoning

import (
	"reflect"
	"testing"
)

func TestSupportedVariants(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want []string
	}{
		{name: "v4 pro", ref: Model{ModelID: "deepseek-v4-pro"}, want: []string{"high", "xhigh"}},
		{name: "v4 flash", ref: Model{ModelID: " deepseek-v4-flash "}, want: []string{"high", "xhigh"}},
		{name: "reasoner", ref: Model{ModelID: "deepseek-reasoner"}, want: []string{"high", "xhigh"}},
		{name: "unsupported", ref: Model{ModelID: "deepseek-chat"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportedVariants(tt.ref); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SupportedVariants() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEffectiveVariant(t *testing.T) {
	tests := []struct {
		name      string
		ref       Model
		requested string
		want      string
	}{
		{name: "empty remains empty", ref: Model{ModelID: "deepseek-reasoner"}, requested: ""},
		{name: "low maps to high", ref: Model{ModelID: "deepseek-reasoner"}, requested: " low ", want: "high"},
		{name: "medium maps to high", ref: Model{ModelID: "deepseek-reasoner"}, requested: "medium", want: "high"},
		{name: "high remains high", ref: Model{ModelID: "deepseek-reasoner"}, requested: "high", want: "high"},
		{name: "xhigh remains xhigh", ref: Model{ModelID: "deepseek-reasoner"}, requested: "xhigh", want: "xhigh"},
		{name: "unsupported variant", ref: Model{ModelID: "deepseek-reasoner"}, requested: "minimal"},
		{name: "unsupported model", ref: Model{ModelID: "deepseek-chat"}, requested: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveVariant(tt.ref, tt.requested); got != tt.want {
				t.Fatalf("EffectiveVariant() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatCompletionsEffortForVariant(t *testing.T) {
	tests := []struct {
		name            string
		ref             Model
		variant         string
		wantEffort      string
		wantOK          bool
		wantUnsupported bool
	}{
		{name: "empty", ref: Model{ModelID: "deepseek-reasoner"}, variant: ""},
		{name: "medium maps to high", ref: Model{ModelID: "deepseek-reasoner"}, variant: " MEDIUM ", wantEffort: "high", wantOK: true},
		{name: "xhigh maps to max", ref: Model{ModelID: "deepseek-reasoner"}, variant: "xhigh", wantEffort: "max", wantOK: true},
		{name: "unsupported variant", ref: Model{ModelID: "deepseek-reasoner"}, variant: "minimal", wantUnsupported: true},
		{name: "unsupported model", ref: Model{ModelID: "deepseek-chat"}, variant: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEffort, gotOK, gotUnsupported := ChatCompletionsEffortForVariant(tt.ref, tt.variant)
			if gotEffort != tt.wantEffort || gotOK != tt.wantOK || gotUnsupported != tt.wantUnsupported {
				t.Fatalf("ChatCompletionsEffortForVariant() = (%q, %v, %v), want (%q, %v, %v)", gotEffort, gotOK, gotUnsupported, tt.wantEffort, tt.wantOK, tt.wantUnsupported)
			}
		})
	}
}
