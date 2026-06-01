package googlereasoning

import (
	"reflect"
	"testing"
)

func TestSupportsThinking(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want bool
	}{
		{name: "gemini 3", ref: Model{ModelID: "gemini-3-pro"}, want: true},
		{name: "gemini 25", ref: Model{ModelID: " gemini-2.5-flash "}, want: true},
		{name: "gemini 20 unsupported", ref: Model{ModelID: "gemini-2.0-flash"}},
		{name: "gemini 1 unsupported", ref: Model{ModelID: "gemini-1.5-pro"}},
		{name: "non gemini", ref: Model{ModelID: "text-bison"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsThinking(tt.ref); got != tt.want {
				t.Fatalf("SupportsThinking() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportedVariants(t *testing.T) {
	tests := []struct {
		name string
		ref  Model
		want []string
	}{
		{name: "gemini 3 pro", ref: Model{ModelID: "gemini-3-pro"}, want: []string{"low", "medium", "high"}},
		{name: "gemini 3 flash", ref: Model{ModelID: "gemini-3-flash-lite"}, want: []string{"minimal", "low", "medium", "high"}},
		{name: "gemini 25 pro", ref: Model{ModelID: "gemini-2.5-pro"}, want: []string{"-1"}},
		{name: "gemini 25 flash", ref: Model{ModelID: "gemini-2.5-flash"}, want: []string{"0", "-1"}},
		{name: "gemini 25 flash lite", ref: Model{ModelID: "gemini-2.5-flash-lite"}, want: []string{"0", "-1"}},
		{name: "unsupported", ref: Model{ModelID: "gemini-2.0-flash"}, want: nil},
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
		{name: "gemini 3 level", ref: Model{ModelID: "gemini-3-pro"}, requested: " HIGH ", want: "high"},
		{name: "gemini 3 unsupported level", ref: Model{ModelID: "gemini-3-pro"}, requested: "minimal"},
		{name: "gemini 25 pro dynamic budget", ref: Model{ModelID: "gemini-2.5-pro"}, requested: "128", want: "128"},
		{name: "gemini 25 pro rejects zero", ref: Model{ModelID: "gemini-2.5-pro"}, requested: "0"},
		{name: "gemini 25 flash accepts zero", ref: Model{ModelID: "gemini-2.5-flash"}, requested: "0", want: "0"},
		{name: "gemini 25 flash lite min budget", ref: Model{ModelID: "gemini-2.5-flash-lite"}, requested: "511"},
		{name: "gemini 25 disables budget", ref: Model{ModelID: "gemini-2.5-flash"}, requested: "-1", want: "-1"},
		{name: "unsupported model", ref: Model{ModelID: "gemini-2.0-flash"}, requested: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveVariant(tt.ref, tt.requested); got != tt.want {
				t.Fatalf("EffectiveVariant() = %q, want %q", got, tt.want)
			}
		})
	}
}
