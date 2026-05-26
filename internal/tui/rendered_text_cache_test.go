package tui

import "testing"

func TestRenderedTextCacheReusesSingleEntry(t *testing.T) {
	cache := newRenderedTextCache()
	calls := 0

	first := cache.renderedFor(11, func() string {
		calls++
		return "first"
	})
	second := cache.renderedFor(11, func() string {
		calls++
		return "second"
	})

	if calls != 1 {
		t.Fatalf("render calls = %d, want 1 cache hit", calls)
	}
	if first != "first" || second != "first" {
		t.Fatalf("cache returned (%q, %q), want repeated first value", first, second)
	}
}

func TestRenderedTextCacheReplacesPreviousEntry(t *testing.T) {
	cache := newRenderedTextCache()
	cache.renderedFor(11, func() string { return "first" })

	calls := 0
	value := cache.renderedFor(22, func() string {
		calls++
		return "second"
	})

	if calls != 1 {
		t.Fatalf("render calls = %d, want 1 cache miss", calls)
	}
	if value != "second" {
		t.Fatalf("cache value = %q, want %q", value, "second")
	}
	if cache.key != 22 || !cache.valid || cache.value != "second" {
		t.Fatalf("cache stored (%d, %t, %q), want (%d, %t, %q)", cache.key, cache.valid, cache.value, 22, true, "second")
	}
}
