package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testJavaFile = `public class SessionService extends BaseService implements Buildable {
    private final Repo repo;

    public SessionService(Repo repo) {
        this.repo = repo;
    }

    @Override
    protected boolean buildPayload(String input) {
        return true;
    }

    public static <T> SessionService fromInput(T input) {
        return new SessionService((Repo) input);
    }
}

interface Buildable {
    boolean buildPayload(String input);
}
`

func TestJavaSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.java")
	if err := os.WriteFile(path, []byte(testJavaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := javaSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "java", Line: 1},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "java", Line: 8},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "java", Line: 12},
		{FilePath: path, Name: "Buildable", Kind: "interface", Language: "java", Line: 17},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "public class SessionService extends BaseService implements Buildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "protected boolean buildPayload(String input)" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("factory parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "public static <T> SessionService fromInput(T input)" {
		t.Fatalf("factory signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "interface Buildable" {
		t.Fatalf("interface signature = %q", enriched[3].Signature)
	}
}

func TestStripJavaLine_SlashInString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"url in string", `String s = "http://example.com";`, `String s = ;`},
		{"real comment", `int x = 1; // todo`, `int x = 1; `},
		{"comment after string", `String s = "hello"; // note`, `String s = ; `},
		{"no comment", `int x = a + b;`, `int x = a + b;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripJavaLine(tt.input)
			if got != tt.want {
				t.Errorf("stripJavaLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJavaSnippetIgnoresStringBraces(t *testing.T) {
	dir := t.TempDir()
	src := `public class Example {
    public String format(String input) {
        return String.format("value={%s}", input);
    }
}
`
	path := filepath.Join(dir, "Example.java")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := javaSymbolEnricher{}
	enriched := enricher.Enrich(path, []Symbol{
		{FilePath: path, Name: "format", Kind: "function", Language: "java", Line: 2},
	})
	if !strings.Contains(enriched[0].Signature, "format") {
		t.Fatalf("signature truncated by string brace: %q", enriched[0].Signature)
	}
}

func TestAnalyzerRegistryEnrichesJavaSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.java")
	if err := os.WriteFile(path, []byte(testJavaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "java", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "java", Line: 8},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("java signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("java parent = %q, want SessionService", enriched[0].Parent)
	}
}
