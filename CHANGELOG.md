# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org).

## [v0.1.0] - 2026-02-22

### Added

- Repo overview tool with standup, focus, review, blockers, config, `--json`, smart default, and cache
- `-R` repo flag support and multiple `-R` flags
- `-O`/`--org` flag with concurrency
- `-i`/`--interactive` flag for interactive selection
- GitHub Actions CI workflow
- Unit tests for pure functions and config
- GoReleaser config and release workflow
- CONTRIBUTING.md and CODE_OF_CONDUCT.md
- Copilot instructions and CCA setup

### Changed

- Refactor: split main.go into per-command files
- Use gh prompter for interactive picker with concurrency
- Use restrictive permissions for config files
- Update README with all commands, flags, and features

### Fixed

- Extra indentation in recent commits output
- Dashboard, CI summary, and linked issues
- Input validation for repo and org format

[v0.1.0]: https://github.com/maxbeizer/gh-wut/releases/tag/v0.1.0
