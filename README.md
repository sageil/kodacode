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


## Turns AI from "suggestion generator" into "work executor"

## How It Works
The built‑in `engineer` agent is designed for broad or high‑impact work.
Instead of editing immediately, it deliberately slows down to reduce risk:

- Explores the repository to understand context
- Produces a structured execution plan
- Asks for approval before making changes
- Executes work in verified steps
- Reviews results against explicit acceptance criteria
## Agents ##
- builder Default, project path sandboxed agent for development work
- advisor Read-only research and advisory agent
- [More Agents](https://kodacode.dev/features/agents/)
## Features ##

- [Sandbox by default](https://kodacode.dev/features/sandbox/)
- [Model Routing](https://kodacode.dev/features/model-routing/)
- [Semantic Code Search](https://kodacode.dev/features/search/)
- [Cost Tracking](https://kodacode.dev/features/cost-tracking/)
- [Project Memory](https://kodacode.dev/features/memory/)
- [Context Management](https://kodacode.dev/features/context/)


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

See [CREDITS.md](CREDITS.md) for the full list of open source projects KodaCode is built on.

## License

[AGPL-3.0](LICENSE)
