<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/logo-banner.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/logo-banner.svg">
  <img alt="KodaCode" src="assets/logo-banner.svg" width="800">
</picture>

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sageil/kodacode)](https://github.com/sageil/kodacode/releases)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-FBB040?logo=homebrew&logoColor=white)](https://github.com/sageil/homebrew-tap)
[![Docs](https://img.shields.io/badge/Docs-kodacode.dev-8B5CF6)](https://kodacode.dev)

</div>

## How It Works

You describe what you want. KodaCode reads your code, runs commands, edits files, and iterates until the task is done. It operates inside a sandbox confined to your project directory, asks permission before destructive actions, and tracks costs so there are no surprises.

## Key Features

- **Multi-provider**: OpenAI, Anthropic, Google, and 15+ OpenAI-compatible providers (Groq, DeepSeek, Mistral, Ollama, and more) — switch mid-session
- **20+ built-in tools**: File ops, shell, code search, LSP actions, symbol rename, git, and more
- **Agent system**: Specialized agents (explorer, planner, reviewer, refactor) that the model delegates to automatically
- **Sandboxed execution**: Every tool call is confined to your project — path escapes and external access require explicit permission
- **Session management**: Conversation history, context compaction, and time-travel snapshots
- **Background tasks**: Run long commands in the background; results are delivered back to the model automatically
- **Cost tracking**: Live per-session cost display with configurable budget caps
- **Extensible**: MCP servers, custom agents, overridable prompts, and custom themes

## Install

```bash
# Homebrew
brew tap sageil/tap && brew install --cask kodacode

# Quick install
curl -fsSL https://raw.githubusercontent.com/sageil/kodacode/main/install.sh | sh

# Go
go install github.com/sageil/kodacode/v1/cmd/kodacode@latest
```

## Quick Start

```bash
# Optional: authenticate with a provider
kodacode login openai

# Start
kodacode
```

Type your message and press Enter. KodaCode handles the rest.

## Documentation

Full documentation, configuration reference, and guides are available at **[kodacode.dev](https://kodacode.dev)**.

## Credits

KodaCode is built on these open source projects:

| Package | License |
|---------|---------|
| [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss) | MIT |
| [OpenAI Go SDK](https://github.com/openai/openai-go) | Apache-2.0 |
| [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go) | MIT |
| [Google GenAI](https://github.com/googleapis/go-genai) | Apache-2.0 |
| [goquery](https://github.com/PuerkitoBio/goquery) | BSD-3-Clause |
| [Chroma](https://github.com/alecthomas/chroma) | MIT |
| [Goldmark](https://github.com/yuin/goldmark) | MIT |
| [Echo](https://github.com/labstack/echo) | MIT |
| [doublestar](https://github.com/bmatcuk/doublestar) | MIT |
| [fsnotify](https://github.com/fsnotify/fsnotify) | BSD-3-Clause |
| [modernc/sqlite](https://gitlab.com/nicholasgasior/modernc-sqlite) | BSD-3-Clause |
| [mvdan/sh](https://github.com/mvdan/sh) | BSD-3-Clause |
| [yaml.v3](https://github.com/go-yaml/yaml) | MIT |
| [Astro](https://astro.build) + [Starlight](https://starlight.astro.build) | MIT |

[Full credits](https://kodacode.dev/reference/credits/)

## License

[AGPL-3.0](LICENSE)
