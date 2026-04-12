package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testCFile = `typedef struct SessionService {
    int id;
} SessionService;

struct BuildConfig {
    int strict;
};

static int build_payload(const SessionService *svc, const char *input) {
    return svc != NULL && input != NULL;
}

int check_permission(const char *perm);
`

func TestCSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.c")
	if err := os.WriteFile(path, []byte(testCFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := cSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "c", Line: 1},
		{FilePath: path, Name: "BuildConfig", Kind: "type", Language: "c", Line: 5},
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "c", Line: 9},
		{FilePath: path, Name: "check_permission", Kind: "function", Language: "c", Line: 13},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "typedef struct SessionService" {
		t.Fatalf("typedef signature = %q", enriched[0].Signature)
	}
	if enriched[1].Signature != "struct BuildConfig" {
		t.Fatalf("struct signature = %q", enriched[1].Signature)
	}
	if enriched[2].Signature != "static int build_payload(const SessionService *svc, const char *input)" {
		t.Fatalf("function signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "int check_permission(const char *perm)" {
		t.Fatalf("declaration signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesCSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.c")
	if err := os.WriteFile(path, []byte(testCFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "c", []Symbol{
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "c", Line: 9},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("c signature missing after registry enrichment")
	}
}
