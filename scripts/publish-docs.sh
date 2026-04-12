#!/usr/bin/env bash
set -euo pipefail

REPO="git@github.com:sageil/kodacode.git"
BRANCH="main"
KODA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMPDIR="$(mktemp -d)"

trap 'rm -rf "$TMPDIR"' EXIT

cd "$TMPDIR"
git init -b "$BRANCH"
git remote add origin "$REPO"

cp -R "$KODA_ROOT/site" site
rm -rf site/node_modules site/dist site/.astro

mkdir -p .github/workflows
cp "$KODA_ROOT/.github/workflows/deploy-docs.yml" .github/workflows/

git add site .github
git commit -m "docs: update documentation site"
git push -f origin "$BRANCH"

echo "Pushed docs to $REPO ($BRANCH)"
