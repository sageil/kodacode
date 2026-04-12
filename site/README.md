# KodaCode Documentation

*Truly yours.*

Documentation site for [KodaCode](https://github.com/sageil/kodacode) — an AI-powered coding assistant that lives in your terminal. Built with [Astro Starlight](https://starlight.astro.build).

## Development

```bash
task install   # install dependencies
task dev       # start dev server at http://localhost:4321/koda/
task build     # build static site to dist/
task preview   # build + preview locally
task clean     # remove build artifacts
```

Requires [Node.js](https://nodejs.org/) 22+ and [pnpm](https://pnpm.io/).

## Deployment

Push to `main` to trigger the GitHub Actions workflow (`.github/workflows/deploy.yml`). The site deploys to GitHub Pages at [sageil.github.io/koda](https://sageil.github.io/koda/).

Set **Settings > Pages > Source** to **GitHub Actions** in the repo.

## Structure

```
src/
  content/docs/
    getting-started/   # Introduction, Installation, Quick Start
    features/          # Sandbox, Permissions, Providers, Agents, Tools,
                       # Context, Sessions, Cost Tracking, MCP
    reference/         # Configuration, Commands, Shortcuts, API
    architecture/      # Overview
  components/          # Starlight component overrides
  styles/custom.css    # Theme (dark-only, koda red accent)
```
