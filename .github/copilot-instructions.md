# Copilot Instructions for gh-wut

## Project Overview

`gh-wut` is a GitHub CLI extension written in Go that answers "What do I need to know right now?" It wraps `gh` CLI commands to surface PRs, issues, notifications, and full PR stories.

## Architecture

- **Single-binary Go extension** — no external dependencies beyond the `gh` CLI
- `main.go` contains all command logic (context, catch-up, story, dashboard, picker, helpers)
- `completions.go` handles shell completion generation (bash, zsh, fish)
- Commands shell out to `gh` and `git` via `os/exec`, parse JSON responses

## Conventions

- No third-party Go dependencies — stdlib only (`encoding/json`, `os/exec`, `fmt`, `strings`, `time`)
- ANSI escape codes for terminal styling (dim, bold, colors) — no TUI library
- All `gh` API calls use `gh api` or `gh pr`/`gh issue` subcommands
- Commands have short aliases (e.g., `context`→`ctx`, `story`→`st`)
- Errors are printed to stderr; graceful fallbacks when APIs fail (e.g., notifications → search API)

## Adding a New Command

1. Add the command function (`cmdFoo`) in `main.go`
2. Add the case to the `switch` in `main()` with any aliases
3. Add the case to `cmdPicker()`
4. Update `usage()` string
5. Add completions in `completions.go` for all three shells
6. Update `README.md` command table and description

## Testing

- No test framework currently — validate by building (`go build`) and running commands manually
- Demo is recorded with [VHS](https://github.com/charmbracelet/vhs) via `demo.tape`

## Style

- Keep output compact and scannable — terminal dashboard style
- Use emoji/unicode sparingly (✓, ✗, 👀, @, →)
- Align columns for readability
- Truncate long strings with `truncate()` helper
