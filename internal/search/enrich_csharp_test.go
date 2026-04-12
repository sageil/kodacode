package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testCSharpFile = `namespace KodaCode.Search;

public class SessionService : BaseService, IBuildable
{
    public SessionService(IRepo repo)
    {
    }

    [Trace]
    protected async Task<bool> BuildPayloadAsync(string input)
    {
        return true;
    }

    public static SessionService FromInput<T>(T input) => new((IRepo)input);
}

public interface IBuildable
{
    Task<bool> BuildPayloadAsync(string input);
}
`

func TestCSharpSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.cs")
	if err := os.WriteFile(path, []byte(testCSharpFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := csharpSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "csharp", Line: 3},
		{FilePath: path, Name: "BuildPayloadAsync", Kind: "function", Language: "csharp", Line: 9},
		{FilePath: path, Name: "FromInput", Kind: "function", Language: "csharp", Line: 14},
		{FilePath: path, Name: "IBuildable", Kind: "interface", Language: "csharp", Line: 17},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "public class SessionService : BaseService, IBuildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "protected async Task<bool> BuildPayloadAsync(string input)" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("factory parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "public static SessionService FromInput<T>(T input)" {
		t.Fatalf("factory signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "public interface IBuildable" {
		t.Fatalf("interface signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesCSharpSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.cs")
	if err := os.WriteFile(path, []byte(testCSharpFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "csharp", []Symbol{
		{FilePath: path, Name: "BuildPayloadAsync", Kind: "function", Language: "csharp", Line: 9},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("csharp signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("csharp parent = %q, want SessionService", enriched[0].Parent)
	}
}
