package prompt

import (
	"context"
	"testing"
)

func TestShaperShapeUsesGenericStructuredInstructions(t *testing.T) {
	shaper := NewShaper()

	got, err := shaper.Shape(context.Background(), Compiled{
		Document:              "full",
		StablePrefix:          "stable",
		DynamicSuffix:         "dynamic",
		ProviderDocument:      "provider full",
		ProviderStablePrefix:  "provider stable",
		ProviderDynamicSuffix: "provider dynamic",
	})
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	if got.Shape != ViewShapeGeneric {
		t.Fatalf("shape = %q, want %q", got.Shape, ViewShapeGeneric)
	}
	if got.Instructions != "provider full" {
		t.Fatalf("instructions = %q", got.Instructions)
	}
	if got.CacheablePrefix != "provider stable" {
		t.Fatalf("cacheable prefix = %q", got.CacheablePrefix)
	}
	if got.DynamicSuffix != "provider dynamic" {
		t.Fatalf("dynamic suffix = %q", got.DynamicSuffix)
	}
}
