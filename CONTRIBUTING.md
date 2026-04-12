# Contributing to KodaCode

Thanks for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- **Go 1.25+** — [go.dev/dl](https://go.dev/dl/)
- **Task** (go-task) — [taskfile.dev/installation](https://taskfile.dev/installation/)
- **Universal Ctags** — required for search index tests (`brew install universal-ctags`)

## Getting Started

```bash
# Fork and clone the repo
git clone https://github.com/<your-username>/kodacode.git
cd kodacode

# Build
task build

# Run tests
task test

# Run the binary
./bin/kodacode
```

## Project Structure

```
cmd/kodacode/       Entry point and runtime wiring
internal/
  agent/            Agent definitions and loading
  config/           Configuration types, loading, merging
  pipeline/         Request pipeline and middleware
  provider/         AI provider implementations (OpenAI, Anthropic, Google)
  search/           Symbol indexing, FTS, embeddings, hybrid search
  service/          Session service, turn loop, compaction, subagents
  tool/             Built-in tool implementations
  tui/              Terminal UI (Bubble Tea)
schema/             JSON schema for config validation
site/               Documentation site (Astro + Starlight)
```

## Development Workflow

1. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make your changes.** Follow the existing code style — no linter config needed, just match what's there.

3. **Run tests:**
   ```bash
   task test
   ```

4. **Build and verify:**
   ```bash
   task build
   ./bin/kodacode
   ```

5. **Commit** with a clear message following [Conventional Commits](https://www.conventionalcommits.org/):
   ```
   feat: add rate limiting to API endpoints
   fix: prevent sandbox escape via symlinks
   docs: document model routing configuration
   refactor: split session service into smaller files
   ```

6. **Push and open a PR** against `main`.

## Code Style

- Follow existing patterns in the codebase
- Run `go vet ./...` before committing
- No trivial or obvious comments — only comment non-obvious logic
- No generated documentation or README changes unless explicitly requested
- Keep functions focused — if a function does too many things, split it

## Tests

- Add tests for new functionality
- Fix any tests your changes break, even if unrelated
- Use table-driven tests where appropriate
- Tests should be deterministic — no sleeps, no network calls in unit tests

## Pull Requests

- Keep PRs focused on one change
- Include a screenshot if the change affects the TUI
- Fill out the PR template
- Link related issues
- Be responsive to review feedback

## Reporting Issues

Use the issue templates:

- **Bug reports** require a screenshot, reproduction steps, and version info
- **Feature requests** should describe the problem before proposing a solution

Search existing issues before opening a new one.

## Documentation

The documentation site lives in `site/` and is built with Astro + Starlight.

```bash
cd site
pnpm install
pnpm run dev
```

Content files are in `site/src/content/docs/`. Edit existing pages or add new `.mdx` files. Update `astro.config.mjs` to add new pages to the sidebar.

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0](LICENSE) license.
