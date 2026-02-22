# Contributing to gh-wut

Thanks for your interest in contributing!

## Getting started

```bash
git clone https://github.com/maxbeizer/gh-wut.git
cd gh-wut
make build
make test
make relink-local  # install locally for testing
```

## Development workflow

1. Create a branch from `main`
2. Make your changes
3. Run `make ci` (builds, vets, and tests)
4. Test your changes locally with `make relink-local`
5. Open a PR against `main`

## Code organization

- `main.go` — CLI entry point, arg parsing, usage, picker, config subcommand
- `context.go` — `gh wut context` command
- `catchup.go` — `gh wut catch-up` command
- `story.go` — `gh wut story` command
- `dashboard.go` — `gh wut dashboard` command
- `standup.go` — `gh wut standup` command
- `focus.go` — `gh wut focus` command
- `review.go` — `gh wut review` command
- `blockers.go` — `gh wut blockers` command
- `helpers.go` — shared utilities (ghUser, truncate, timeAgo, validation, etc.)
- `types.go` — shared struct types and JSON output types
- `config.go` — user config (~/.config/gh-wut/repos)
- `completions.go` — shell completion scripts (bash, zsh, fish)

## Adding a new command

1. Create `yourcommand.go` with `cmdYourCommand()` and any types it needs
2. Add the case to the `switch` in `main()` with aliases
3. Add to `cmdPicker()` options
4. Update `usage()` string
5. Add completions in `completions.go` for all three shells
6. Add tests for any pure functions
7. Update `README.md`

## Conventions

- No third-party dependencies beyond `github.com/cli/go-gh/v2`
- Use `sync.WaitGroup` for concurrent API calls
- All `gh` API calls go through `exec.Command("gh", ...)`
- Validate user input with `validateRepo()` / `validateOrg()`
- Support `--json` output on every command
- Support `-R` (multi-repo) and `-O` (org) scoping where applicable
- Tests must be deterministic — no network calls in unit tests

## Commit messages

Follow conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`
