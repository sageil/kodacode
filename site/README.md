# KodaCode Documentation

*Truly yours.*

Documentation site for [KodaCode](https://github.com/sageil/kodacode), the event-first coding agent for your terminal. Built with [Astro Starlight](https://starlight.astro.build).

## Development

```bash
task install   # install dependencies
task dev       # start dev server at http://localhost:4321/
task build     # build static site to dist/
task preview   # build + preview locally
task clean     # remove build artifacts
```

Requires [Node.js](https://nodejs.org/) 22+ and [pnpm](https://pnpm.io/).

## Deployment

Push to `main` to trigger the GitHub Actions workflow (`.github/workflows/deploy-docs.yml`). The site deploys through GitHub Pages and is configured for [kodacode.dev](https://kodacode.dev).

Set **Settings > Pages > Source** to **GitHub Actions** in the repo.

## Structure

```
src/
  content/docs/
    getting-started/   # Introduction, Installation, Quick Start
    features/          # Agents, Sandbox, Tools, Routing, Sessions, Context, Cost
    reference/         # Configuration, Commands, Shortcuts
    architecture/      # Overview
  components/          # Starlight component overrides
  styles/custom.css    # Theme (dark-only, kodacode visual system)
```
