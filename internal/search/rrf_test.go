package search

import "testing"

func TestMergeRRF_BothLists(t *testing.T) {
	fts := []SearchResult{
		{FilePath: "a.go", Name: "Foo", Kind: "function"},
		{FilePath: "b.go", Name: "Bar", Kind: "function"},
	}
	vec := []SearchResult{
		{FilePath: "b.go", Name: "Bar", Kind: "function"},
		{FilePath: "c.go", Name: "Baz", Kind: "type"},
	}

	results := MergeRRF(fts, vec, 10)

	if len(results) != 3 {
		t.Fatalf("len = %d, want 3", len(results))
	}

	// Bar appears in both lists → highest RRF score → should be first.
	if results[0].Name != "Bar" {
		t.Errorf("results[0].Name = %q, want Bar (appeared in both)", results[0].Name)
	}
	if results[0].Source != "both" {
		t.Errorf("results[0].Source = %q, want both", results[0].Source)
	}

	// Check sources for single-list items.
	sources := make(map[string]string)
	for _, r := range results {
		sources[r.Name] = r.Source
	}
	if sources["Foo"] != "fts" {
		t.Errorf("Foo source = %q, want fts", sources["Foo"])
	}
	if sources["Baz"] != "vector" {
		t.Errorf("Baz source = %q, want vector", sources["Baz"])
	}
}

func TestMergeRRF_EmptyVector(t *testing.T) {
	fts := []SearchResult{
		{FilePath: "a.go", Name: "Foo"},
	}
	results := MergeRRF(fts, nil, 10)
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Source != "fts" {
		t.Errorf("source = %q, want fts", results[0].Source)
	}
}

func TestMergeRRF_EmptyFTS(t *testing.T) {
	vec := []SearchResult{
		{FilePath: "a.go", Name: "Foo"},
	}
	results := MergeRRF(nil, vec, 10)
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Source != "vector" {
		t.Errorf("source = %q, want vector", results[0].Source)
	}
}

func TestMergeRRF_Limit(t *testing.T) {
	fts := []SearchResult{
		{FilePath: "a.go", Name: "A"},
		{FilePath: "b.go", Name: "B"},
		{FilePath: "c.go", Name: "C"},
	}
	results := MergeRRF(fts, nil, 2)
	if len(results) != 2 {
		t.Errorf("len = %d, want 2", len(results))
	}
}

func TestMergeRRF_ScoreOrdering(t *testing.T) {
	fts := []SearchResult{
		{FilePath: "a.go", Name: "First"},
		{FilePath: "b.go", Name: "Second"},
		{FilePath: "c.go", Name: "Third"},
	}
	results := MergeRRF(fts, nil, 10)

	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted by score descending: [%d]=%f > [%d]=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}
