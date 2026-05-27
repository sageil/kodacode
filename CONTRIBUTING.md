# Contributing to KodaCode

Thanks for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- **Go 1.25+** [go.dev/dl](https://go.dev/dl/)
- **Task** (go-task) [taskfile.dev/installation](https://taskfile.dev/installation/)

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
cmd/kodacode/       CLI and TUI entrypoint
internal/
  agent/            Agent definitions and loading
  app/              Runtime orchestration, sessions, turns, config, permissions
  bootstrap/        Default config/auth file creation
  configdir/        Platform config/data directory resolution
  engine/           Engine boundary tests and orchestration helpers
  events/           Durable event model, SQLite storage, projection
  lsp/              Language server discovery and code intelligence
  mcp/              MCP stdio transport and tool registry
  observability/    Logging and retention helpers
  permissionpolicy/ Permission policy matching
  prompt/           Prompt fragment compilation and shaping
  provider/         Model providers, routing, model catalog, usage accounting
  search/           Lexical and semantic repository search
  sessiontitle/     Session title generation
  skill/            Skill loading and search
  textdiff/         Diff preview helpers
  textutil/         Text utilities
  tool/             Tool contracts and built-in tool implementations
  tui/              Terminal UI
  websearch/        Web search provider backends
  workspace/        Workspace root and path scope handling
schema/             JSON schema for config validation
site/               Documentation site (Astro + Starlight)
tools/              Tooling modules, including pinned golangci-lint
```

## Development Workflow

1. **Create a branch** from `main`:

   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make your changes.** Follow the existing code style. There is no linter config, so match what's already there.

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
- No trivial or obvious comments, only comment non-obvious logic
- No generated documentation or README changes unless explicitly requested
- Keep functions focused. If a function does too many things, split it

## Tests

- Add tests for new functionality
- Fix any tests your changes break, even if unrelated
- Use table-driven tests where appropriate
- Tests should be deterministic, no sleeps, no network calls in unit tests

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
