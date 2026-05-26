package provider

import "testing"

func TestCanonicalProviderIDUsesRegisteredAlias(t *testing.T) {
	RegisterProviderAlias("anthropic-alias-test", "anthropic")
	if got := CanonicalProviderID("anthropic-alias-test"); got != "anthropic" {
		t.Fatalf("CanonicalProviderID() = %q, want anthropic", got)
	}
}
