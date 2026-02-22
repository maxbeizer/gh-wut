# gh-wut

What do I need to know right now?

![demo](demo.gif)

## Install

```bash
gh extension install maxbeizer/gh-wut
```

## Commands

```
gh wut                                  Interactive picker
gh wut context [-R repo]                Where was I? Open PRs, issues, recent commits
gh wut catch-up [-R repo]               What happened? Notification triage
gh wut story [-R repo] [pr]             Full PR story — issue, related PRs, CI
gh wut dashboard [-R repo]              Cross-repo: everything with your name on it
gh wut standup [-R repo]                What did I do? Merged PRs, closed issues, commits
gh wut focus [-R repo]                  Just tell me what to do next
gh wut review [-R repo] [pr]            Reviewer-focused PR view — threads, CI, staleness
gh wut blockers [-R repo]               What's stuck? Failing CI, awaiting review, etc.
gh wut config add-repo <owner/repo>     Add a default repo
gh wut config remove-repo <owner/repo>  Remove a default repo
gh wut config list                      List default repos
```

Every command has short aliases:

| Command    | Aliases          |
|------------|------------------|
| `context`  | `ctx`            |
| `catch-up` | `catchup`, `cu`  |
| `story`    | `st`             |
| `dashboard`| `dash`, `db`     |
| `standup`  | `su`             |
| `focus`    | `f`              |
| `review`   | `rv`             |
| `blockers` | `bl`             |

## Flags

```
-R, --repo <owner/repo>       Target repo (repeatable; default: configured repos or current)
-O, --org <org>               Target org (repeatable; scopes search to org)
    --since <date>            Since when (yesterday, monday..sunday, last-week, 2025-01-15)
-i, --interactive             Show selection prompt to open items in browser
    --json                    Output as JSON (for scripting)
```

`-R` and `-O` can be passed multiple times to target several repos or orgs at once.

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

### `standup` — What did I do?

Daily summary: merged PRs, closed issues, and commits. Defaults to yesterday; use `--since` for a different window (`monday`, `last-week`, `2025-01-15`, etc.).

### `focus` — Just tell me what to do next

Picks your single highest-priority action item — the one PR to review or issue to tackle right now.

### `review` — Reviewer-focused PR view

Like `story` but from the reviewer's perspective: unresolved threads, CI status, staleness, and a quick verdict on whether it's ready.

### `blockers` — What's stuck?

Surfaces work that needs attention: PRs with failing CI, stale review requests, issues blocked on dependencies.

### `config` — Default repo management

Manage your default repos so you don't have to pass `-R` every time.

```bash
gh wut config add-repo owner/repo     # add a default
gh wut config remove-repo owner/repo  # remove one
gh wut config list                    # show all defaults
```

Repos are stored in `~/.config/gh-wut/repos` (one `owner/repo` per line).

## Configuration

Default repos live in `~/.config/gh-wut/repos`. When no `-R` flag is passed, commands automatically use these repos. If none are configured and you're inside a git repo, the current repo is used.

## Uninstall

```bash
gh extension remove wut
```

## License

MIT
