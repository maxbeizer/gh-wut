# Copilot Coding Agent Setup

## Overview

This document describes how to set up the Copilot Coding Agent (CCA) for `gh-wut`.

## Prerequisites

- Repository must be hosted on GitHub
- Copilot must be enabled for the repository/organization
- GitHub Actions must be enabled

## Setup Steps

### 1. Enable Copilot Coding Agent

In your repository settings:
1. Go to **Settings → Copilot → Coding agent**
2. Enable "Allow Copilot to open pull requests"
3. Select the appropriate access level

### 2. Configure Allowed Tools

CCA needs access to run Go build commands. The firewall allowlist is configured below.

### 3. Assign Issues

Once enabled, you can assign issues to Copilot by:
- Assigning `@copilot` as an assignee on an issue
- Using `gh issue edit <number> --add-assignee @copilot`

Copilot will read the issue, plan changes, open a PR, and iterate based on CI feedback.

## Configuration Files

### `.github/copilot/agents.yml`

The agent configuration file defines tools and permissions for the coding agent.

### Firewall / Network Access

This project has no external dependencies beyond the Go stdlib, so no network allowlist is needed for builds. The `go build` command works fully offline.

## CI for CCA Pull Requests

CCA benefits from fast CI feedback. The minimum CI check for this repo:

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go build ./...
      - run: go vet ./...
```

## Tips

- Keep issues small and well-scoped for best CCA results
- Include acceptance criteria in issue descriptions
- Reference existing code patterns in issues so CCA follows conventions
- The `.github/copilot-instructions.md` file provides CCA with project context automatically
