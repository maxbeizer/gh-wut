package main

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/prompter"
)

const version = "0.1.0"

func usage() {
	fmt.Printf(`gh-wut v%s — What do I need to know right now?

Usage:
  gh wut                        Interactive picker
  gh wut context [-R repo]      Where was I? Open PRs, issues, recent commits
  gh wut catch-up [-R repo]     What happened? Notification triage
  gh wut story [-R repo] [pr]   Full PR story — issue, related PRs, CI
  gh wut dashboard [-R repo]    Cross-repo: everything with your name on it
  gh wut standup [-R repo]      What did I do? Merged PRs, closed issues, commits
  gh wut focus [-R repo]        Just tell me what to do next
  gh wut review [-R repo] [pr]  Reviewer-focused PR view — threads, CI, staleness
  gh wut blockers [-R repo]     What's stuck? Failing CI, awaiting review, etc.
  gh wut config add-repo <owner/repo>   Add a default repo
  gh wut config remove-repo <owner/repo> Remove a default repo
  gh wut config list            List default repos

Flags:
  -R, --repo <owner/repo>       Target repo (repeatable; default: configured repos or current)
  -O, --org <org>               Target org (repeatable; scopes search to org)
  --since <date>                Since when (yesterday, monday..sunday, last-week, 2025-01-15)
  -i, --interactive             Show selection prompt to open items in browser
      --json                    Output as JSON (for scripting)
  -h, --help                    Show help
  -v, --version                 Show version

Aliases: context|ctx, catch-up|catchup|cu, story|st, dashboard|dash|db, standup|su, focus|f, review|rv, blockers|bl
`, version)
}

func main() {
	if len(os.Args) < 2 {
		if r := detectRepo(); r != "" {
			cmdContext([]string{r}, false, false)
		} else {
			cmdPicker(nil)
		}
		return
	}

	// Extract repo flags, --org, --since, and --json from anywhere in args
	var repos []string
	var orgs []string
	var cleanArgs []string
	sinceArg := ""
	jsonOutput := false
	interactive := false
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "-R" || os.Args[i] == "--repo") && i+1 < len(os.Args) {
			i++
			repos = append(repos, os.Args[i])
		} else if (os.Args[i] == "-O" || os.Args[i] == "--org") && i+1 < len(os.Args) {
			i++
			orgs = append(orgs, os.Args[i])
		} else if os.Args[i] == "--since" && i+1 < len(os.Args) {
			i++
			sinceArg = os.Args[i]
		} else if os.Args[i] == "--json" {
			jsonOutput = true
		} else if os.Args[i] == "-i" || os.Args[i] == "--interactive" {
			interactive = true
		} else {
			cleanArgs = append(cleanArgs, os.Args[i])
		}
	}

	// Validate inputs
	for _, r := range repos {
		if err := validateRepo(r); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	for _, o := range orgs {
		if err := validateOrg(o); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	// Load default repos from config if none specified via flags
	if len(repos) == 0 {
		repos = listRepos()
	}

	if len(cleanArgs) == 0 {
		if len(repos) > 0 {
			cmdContext(repos, jsonOutput, interactive)
		} else if r := detectRepo(); r != "" {
			cmdContext([]string{r}, jsonOutput, interactive)
		} else {
			cmdPicker(repos)
		}
		return
	}

	switch cleanArgs[0] {
	case "context", "ctx":
		cmdContext(repos, jsonOutput, interactive)
	case "catch-up", "catchup", "cu":
		cmdCatchUp(repos, jsonOutput)
	case "story", "st":
		pr := ""
		if len(cleanArgs) > 1 {
			pr = cleanArgs[1]
		}
		repo := ""
		if len(repos) > 0 {
			repo = repos[0]
		}
		cmdStory(repo, pr, jsonOutput)
	case "dashboard", "dash", "db":
		cmdDashboard(repos, orgs, jsonOutput, interactive)
	case "standup", "su":
		cmdStandup(repos, orgs, sinceArg, jsonOutput)
	case "focus", "f":
		cmdFocus(repos, orgs, jsonOutput)
	case "review", "rv":
		pr := ""
		if len(cleanArgs) > 1 {
			pr = cleanArgs[1]
		}
		repo := ""
		if len(repos) > 0 {
			repo = repos[0]
		}
		cmdReview(repo, pr, jsonOutput)
	case "blockers", "bl":
		cmdBlockers(repos, orgs, jsonOutput, interactive)
	case "config":
		cmdConfig(cleanArgs[1:])
	case "-h", "--help", "help":
		usage()
	case "--completions":
		shell := "bash"
		if len(cleanArgs) > 1 {
			shell = cleanArgs[1]
		}
		printCompletions(shell)
	case "-v", "--version", "version":
		fmt.Printf("gh-wut v%s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cleanArgs[0])
		usage()
		os.Exit(1)
	}
}

// --- Interactive Picker ---

func cmdPicker(repos []string) {
	p := prompter.New(os.Stdin, os.Stdout, os.Stderr)
	options := []string{
		"context    — Where was I?",
		"catch-up   — What happened while I was gone?",
		"story      — Full PR story",
		"dashboard  — Everything, everywhere",
		"standup    — What did I do?",
		"focus      — Just tell me what to do next",
		"review     — Reviewer-focused PR view",
		"blockers   — What's stuck?",
	}

	idx, err := p.Select("wut?", "", options)
	if err != nil {
		return
	}

	repo := ""
	if len(repos) > 0 {
		repo = repos[0]
	}

	switch idx {
	case 0:
		cmdContext(repos, false, false)
	case 1:
		cmdCatchUp(repos, false)
	case 2:
		cmdStory(repo, "", false)
	case 3:
		cmdDashboard(repos, nil, false, false)
	case 4:
		cmdStandup(repos, nil, "", false)
	case 5:
		cmdFocus(repos, nil, false)
	case 6:
		cmdReview(repo, "", false)
	case 7:
		cmdBlockers(repos, nil, false, false)
	}
}

// --- Config: manage default repos ---

func cmdConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh wut config <add-repo|remove-repo|list> [owner/repo]")
		os.Exit(1)
	}
	switch args[0] {
	case "add-repo":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: gh wut config add-repo <owner/repo>")
			os.Exit(1)
		}
		if err := addRepo(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Added %s\n", args[1])
	case "remove-repo":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: gh wut config remove-repo <owner/repo>")
			os.Exit(1)
		}
		if err := removeRepo(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Removed %s\n", args[1])
	case "list":
		repos := listRepos()
		if len(repos) == 0 {
			fmt.Println("No default repos configured.")
			return
		}
		for _, r := range repos {
			fmt.Println(r)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", args[0])
		os.Exit(1)
	}
}
