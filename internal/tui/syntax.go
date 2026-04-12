package tui

import (
	"bytes"
	"path/filepath"
	"strings"

	chroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".js", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".jsx":
		return "JavaScript"
	case ".json":
		return "JSON"
	case ".py":
		return "Python"
	case ".sh", ".bash", ".zsh":
		return "Bash"
	case ".md", ".mdx":
		return "Markdown"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css", ".scss", ".sass":
		return "CSS"
	case ".rs":
		return "Rust"
	case ".rb":
		return "Ruby"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".swift":
		return "Swift"
	case ".kt":
		return "Kotlin"
	case ".dockerfile":
		return "Dockerfile"
	case ".xml":
		return "XML"
	case ".proto":
		return "Protobuf"
	case ".php":
		return "PHP"
	case ".lua":
		return "Lua"
	case ".pl", ".pm":
		return "Perl"
	case ".r", ".R":
		return "R"
	case ".scala":
		return "Scala"
	case ".groovy":
		return "Groovy"
	case ".dart":
		return "Dart"
	case ".vue":
		return "Vue"
	case ".svelte":
		return "Svelte"
	case ".tf", ".hcl":
		return "Terraform"
	case ".nix":
		return "Nix"
	case ".elm":
		return "Elm"
	case ".hs":
		return "Haskell"
	case ".ml", ".mli":
		return "OCaml"
	case ".fs", ".fsx":
		return "F#"
	case ".ex", ".exs":
		return "Elixir"
	case ".erl":
		return "Erlang"
	case ".clj", ".cljs", ".cljc":
		return "Clojure"
	case ".jl":
		return "Julia"
	case ".s", ".asm":
		return "Assembly"
	case ".m", ".mm":
		return "Objective-C"
	case ".env", ".gitignore", ".dockerignore":
		return "plaintext"
	default:
		// Handle filename-only cases like "Dockerfile"
		base := strings.ToLower(filepath.Base(path))
		if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
			return "Dockerfile"
		}
		return ""
	}
}

func chromaStyle(th *theme.Theme) string {
	// 1. Explicit syntax_style from theme YAML.
	if th != nil && th.SyntaxStyle != "" {
		return th.SyntaxStyle
	}
	// 2. Known built-in theme names.
	name := ""
	if th != nil {
		name = th.Name
	}
	switch name {
	case "rose-pine-moon", "rose-pine":
		return "rose-pine"
	case "catppuccin", "catppuccin-mocha", "catppuccin-latte":
		return "catppuccin-mocha"
	case "light":
		return "github"
	}
	// 3. Detect from palette luminance.
	if th != nil && th.IsLight() {
		return "github"
	}
	return "dracula"
}

// syntaxHighlight applies chroma terminal syntax highlighting to content.
func syntaxHighlight(content, language string, th *theme.Theme) string {
	if language == "" || content == "" {
		return content
	}

	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(chromaStyle(th))
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		return content
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return content
	}
	return buf.String()
}
