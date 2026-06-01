package qwenreasoning

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
		{name: "qwen3", ref: Model{ModelID: "qwen3-coder"}, want: []string{"none", "high"}},
		{name: "namespaced qwen3", ref: Model{ModelID: " qwen/qwen3-coder "}, want: []string{"none", "high"}},
		{name: "qwen plus", ref: Model{ModelID: "qwen-plus"}, want: []string{"none", "high"}},
		{name: "qwq", ref: Model{ModelID: "qwq-32b"}, want: []string{"none", "high"}},
		{name: "unsupported", ref: Model{ModelID: "qwen2.5-coder"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportedVariants(tt.ref); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SupportedVariants() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
