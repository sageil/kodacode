package sessiontitle

import "testing"

func TestNormalizeStripsFormattingAndPunctuation(t *testing.T) {
	got := Normalize(`**"Optimizing Your Project’s Performance"**`)
	if got != "Optimizing Your Projects Performance" {
		t.Fatalf("Normalize() = %q", got)
	}
}

func TestNormalizeKeepsLettersAndSpacesOnly(t *testing.T) {
	got := Normalize(`title: Cache TTLs & DB Query Tuning v2`)
	if got != "Cache TTLs DB Query Tuning v" {
		t.Fatalf("Normalize() = %q", got)
	}
}

func TestNormalizeStripsWrappedTitlePrefix(t *testing.T) {
	got := Normalize(`**Title: Fast Cache Review**`)
	if got != "Fast Cache Review" {
		t.Fatalf("Normalize() = %q", got)
	}
}
