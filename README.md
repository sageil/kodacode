# KodaCode

Turns AI coding from *suggestion generator* into *work executor*.

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sageil/kodacode)](https://github.com/sageil/kodacode/releases)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-FBB040?logo=homebrew&logoColor=white)](https://github.com/sageil/homebrew-tap)
[![Docs](https://img.shields.io/badge/Docs-kodacode.dev-8B5CF6)](https://kodacode.dev)

</div>

KodaCode is an open-source, terminal-native coding agent built for trustworthy
software delivery. It keeps orchestration, permissions, tool execution, prompt
assembly, and replayable session state in runtime code and stored events
instead of hidden prompt behavior.

## Install

```bash
# Homebrew
brew tap sageil/tap && brew install --cask kodacode

# Quick install
curl -fsSL https://raw.githubusercontent.com/sageil/kodacode/main/install.sh | sh

# Go
go install github.com/sageil/kodacode/cmd/kodacode@latest
```

See the [installation guide](https://kodacode.dev/getting-started/installation/)
for setup details.

## Quick Start

Run KodaCode in the repository you want to work in:

```bash
kodacode
```

Inside the TUI:

1. Use `/connect` to configure a provider.
2. Use `/model` to choose a model route, such as `openai/gpt-5`.
3. Use `/init` to create workspace instructions.
4. Ask for one concrete task.

For a full first-session walkthrough, see the
[Quick Start](https://kodacode.dev/getting-started/quick-start/). For provider
IDs and route behavior, see [Model Routing](https://kodacode.dev/features/model-routing/).

## How It Works

KodaCode roots a session at your current workspace. Relative file access starts
there, while external paths, network access, and sensitive commands go through
explicit runtime-owned approval gates.

Use `builder` when the task is clear and contained. Use `engineer` when the task
is broad, risky, architectural, or needs an approved plan before edits.

Example prompt:

```text
Build a calculator that estimates monthly payment, total interest, and affordability range.

Acceptance criteria:

- Inputs: home price, down payment, interest rate, amortization years, property tax, insurance, condo fees, gross monthly income, monthly debts.
- Outputs: loan amount, estimated monthly principal + interest, total monthly housing cost, debt-to-income ratio, and affordability status.
- Use standard fixed-rate mortgage amortization math.
- Validate invalid inputs clearly.
- Add focused tests for payment calculation, DTI calculation, and edge cases.
- Do not use fake values or hardcoded backend state.
- Run the relevant test suite before finishing.
```

For a plan-first flow, select the `engineer` agent in the TUI and ask:

```text
Before editing, inspect the app structure, propose a short implementation plan,
and wait for approval before making code changes.
```

## Core Features

- [Sandbox & Permissions](https://kodacode.dev/features/sandbox/)
- [Sessions](https://kodacode.dev/features/sessions/)
- [Agents](https://kodacode.dev/features/agents/)
- [Project Memory & Instructions](https://kodacode.dev/features/project-memory/)
- [Tools](https://kodacode.dev/features/tools/)
- [Providers](https://kodacode.dev/features/providers/)
- [Model Routing](https://kodacode.dev/features/model-routing/)
- [Context Management](https://kodacode.dev/features/context/)
- [Common Workflows](https://kodacode.dev/getting-started/workflows/)
- [Budgets](https://kodacode.dev/features/budgets/)
- [Cost Tracking & Optimization](https://kodacode.dev/features/cost-tracking/)
- [MCP Servers](https://kodacode.dev/features/mcp/)
- [Skills](https://kodacode.dev/features/skills/)

## Agents

- `builder`: default project-sandboxed coding agent
- `engineer`: structured planning and workflow task tracking
- `reviewer`: read-only review of current changes
- `planner`: read-only repository analysis and implementation planning

See [Agents](https://kodacode.dev/features/agents/) for custom agent definitions
and tool policy behavior.

## Layouts

KodaCode has two TUI layouts for different working styles:

- **Shell layout** is the default single-plane workflow. It keeps the transcript,
  tool activity, diffs, and final responses in one continuous terminal surface.
- **Classic layout** keeps a persistent right-side inspector with `Details`,
  `Tools`, and `Tasks` tabs. It is useful when you want the main transcript and
  structured tool/task state visible at the same time.

Switch layouts from the TUI with `Ctrl+L`, or set a default in
`~/.config/kodacode/config.yaml` with `tui.layout: shell` or `tui.layout: classic`.
See [TUI Layouts](https://kodacode.dev/reference/layouts/) for details.

<picture>
  <img alt="KodaCode shell layout showing a full-width transcript with inline tool and diff output" src="site/public/screenshots/readme-shell-layout.png">
</picture>

<picture>
  <img alt="KodaCode classic layout showing the transcript beside Details, Tools, and Tasks inspector tabs" src="site/public/screenshots/readme-classic-layout.png">
</picture>

## Useful Commands

- `/connect`: configure a provider
- `/model`: choose the active model
- `/init`: create workspace instructions
- `/timeline`: branch from an earlier completed turn or navigate related branches
- `/trace`: inspect what happened in a turn
- `/cost`: inspect spend and token savings
- `Ctrl+W`: list or select a runtime workflow such as `delivery`, `debug`,
  `review`, or `explore`

See [Slash Commands](https://kodacode.dev/reference/commands/) for the full
command surface.

## One-Shot CLI

```bash
kodacode "summarize this repository"
kodacode --resume "continue the previous refactor"
kodacode --add-dir ../shared "inspect both repos before editing"
kodacode --skill migration "add the schema change and focused tests"
kodacode --workflow delivery "implement this change and verify it"
```

## Documentation

Full documentation, configuration reference, and guides are available at
[kodacode.dev](https://kodacode.dev).

## License

[AGPL-3.0](LICENSE)
