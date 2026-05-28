package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderMarkdownBlockOnSurfaceSyntaxHighlightsWithThemeFallback(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	rendered := strings.Join(renderMarkdownBlockOnSurface(model, "```go\nfunc main() {\n    println(\"hello\")\n}\n```", 80, tonePanelAlt), "\n")
	if !strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Primary)) {
		t.Fatalf("rendered code block missing theme keyword color\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Success)) {
		t.Fatalf("rendered code block missing theme string color\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, backgroundANSI(toneValue(model.theme, tonePanelAlt))) {
		t.Fatalf("rendered code block missing requested surface background\nrendered:\n%s", rendered)
	}

	stripped := ansi.Strip(rendered)
	for _, want := range []string{"func main() {", "println(\"hello\")"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("rendered code block missing %q\nrendered:\n%s", want, stripped)
		}
	}
}

func TestRenderMarkdownBlockOnSurfaceFramesFencedCodeBlocks(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	rendered := strings.Join(renderMarkdownBlockOnSurface(model, "```go\nfunc main() {}\n```", 40, tonePanelAlt), "\n")
	stripped := ansi.Strip(rendered)
	for _, want := range []string{"┌ go", "│ func main() {}", "└"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("rendered framed code block missing %q\nrendered:\n%s", want, stripped)
		}
	}
	for _, line := range strings.Split(stripped, "\n") {
		if got := ansi.StringWidth(line); got > 40 {
			t.Fatalf("rendered code block line width = %d, want <= 40\n%s", got, stripped)
		}
	}
}

func TestRenderMarkdownBlockOnSurfaceUsesThemeSyntaxStyle(t *testing.T) {
	tokyoTheme := theme.StaticDefault()
	tokyoTheme.SyntaxStyle = "tokyonight-night"

	roseTheme := theme.StaticDefault()
	roseTheme.SyntaxStyle = "rose-pine-moon"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tokyoModel := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &tokyoTheme,
	})
	roseModel := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &roseTheme,
	})

	source := "```json\n{\"name\":\"kodacode\",\"count\":2}\n```"
	tokyoRendered := strings.Join(renderMarkdownBlockOnSurface(tokyoModel, source, 80, ""), "\n")
	roseRendered := strings.Join(renderMarkdownBlockOnSurface(roseModel, source, 80, ""), "\n")
	if tokyoRendered == roseRendered {
		t.Fatalf("different syntax styles rendered identical ANSI output\nrendered:\n%s", tokyoRendered)
	}

	tokyoStripped := ansi.Strip(tokyoRendered)
	roseStripped := ansi.Strip(roseRendered)
	if tokyoStripped != roseStripped {
		t.Fatalf("syntax style swap changed visible content\n tokyo: %q\n rose:  %q", tokyoStripped, roseStripped)
	}
}

func TestRenderMarkdownBlockOnSurfacePreservesLiteralBackgroundColor(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	literalBG := "#223344"
	rendered := strings.Join(renderMarkdownBlockOnSurface(model, "- `src/cache.ts`", 80, literalBG), "\n")
	r, g, b := parseHex(literalBG)
	if !strings.Contains(rendered, fmt.Sprintf("48;2;%d;%d;%d", r, g, b)) {
		t.Fatalf("rendered markdown missing requested literal background\nrendered:\n%s", rendered)
	}
	sr, sg, sb := parseHex(shellBackgroundColor)
	if strings.Contains(rendered, fmt.Sprintf("48;2;%d;%d;%d", sr, sg, sb)) {
		t.Fatalf("rendered markdown leaked shell fallback background\nrendered:\n%s", rendered)
	}
}
