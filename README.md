# gh-wut

What do I need to know right now?

![demo](demo.gif)

## Install

```bash
gh extension install maxbeizer/gh-wut
```

## Commands

```
gh wut                        Interactive picker
gh wut context [-R repo]      Where was I? Open PRs, issues, recent commits
gh wut catch-up               What happened? Notification triage
gh wut story [pr-url]         Full PR story — issue, related PRs, CI, reviews
gh wut dashboard              Cross-repo: everything with your name on it
```

Every command has short aliases:

| Command    | Aliases          |
|------------|------------------|
| `context`  | `ctx`            |
| `catch-up` | `catchup`, `cu`  |
| `story`    | `st`             |
| `dashboard`| `dash`, `db`     |

## Shell Completions

```bash
# bash
eval "$(gh wut --completions bash)"

# zsh
gh wut --completions zsh > ~/.zfunc/_gh-wut

# fish
gh wut --completions fish | source
```

## What Each Command Does

### `context` — Where was I?

Shows your open PRs, assigned issues, and recent commits for a repo. Detects current repo or use `-R`.

### `catch-up` — What happened while I was gone?

Triages notifications into buckets: review requests, mentions, assignments, CI, other. Falls back to search API if notifications aren't available.

### `story` — Full PR story

Given a PR (number, URL, or auto-detected from current branch), shows: title, author, branch, mergeable status, CI checks, reviews, files changed, and linked issues. Everything in one view.

### `dashboard` — Everything, everywhere

Cross-repo view: your open PRs, PRs awaiting your review, and assigned issues across all repos.

## Uninstall

```bash
gh extension remove wut
```

## License

MIT
