# KodaCode

An AI-powered CLI assistant with a modern terminal UI. KodaCode runs in your terminal, understands your codebase, executes tools, and works autonomously to complete software engineering tasks — from bug fixes to multi-file refactors.

## How It Works

You describe what you want. KodaCode reads your code, runs commands, edits files, and iterates until the task is done. It operates inside a sandbox confined to your project directory, asks permission before destructive actions, and tracks costs so there are no surprises.

## Key Features

- **Multi-provider**: OpenAI, Anthropic, Google, and any OpenAI-compatible endpoint — switch mid-session
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

## License

[AGPL-3.0](LICENSE)
