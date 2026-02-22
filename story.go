package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/cli/go-gh/v2/pkg/prompter"
)

type PRDetail struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	Branch    string `json:"headRefName"`
	URL       string `json:"url"`
	Author    struct{ Login string } `json:"author"`
	IsDraft   bool   `json:"isDraft"`
	Mergeable string `json:"mergeable"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Files     []struct{ Path string } `json:"files"`
	Reviews   []struct {
		Author struct{ Login string } `json:"author"`
		State  string                 `json:"state"`
	} `json:"reviews"`
	StatusCheckRollup []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

var issueRefRe = regexp.MustCompile(`(?:^|\s)#(\d+)`)
var issueURLRe = regexp.MustCompile(`https?://github\.com/[^\s/]+/[^\s/]+/(?:issues|pull)/\d+`)

func cmdStory(repo, prRef string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	reset := "\033[0m"

	if prRef == "" {
		// Try current branch
		branch := currentBranch()
		if branch != "" && branch != "main" && branch != "master" {
			prRef = branch
		}
	}

	if prRef == "" {
		p := prompter.New(os.Stdin, os.Stdout, os.Stderr)
		prRef, _ = p.Input("PR number or URL", "")
		prRef = strings.TrimSpace(prRef)
	}

	if prRef == "" {
		fmt.Fprintln(os.Stderr, "No PR specified.")
		os.Exit(1)
	}

	// Build gh pr view command
	args := []string{"pr", "view", prRef,
		"--json", "number,title,body,state,headRefName,url,author,isDraft,mergeable,additions,deletions,files,reviews,statusCheckRollup"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}

	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching PR: %v\n", err)
		os.Exit(1)
	}

	var pr PRDetail
	_ = json.Unmarshal(out, &pr)

	if jsonOutput {
		printJSON(pr)
		return
	}

	// Header
	fmt.Printf("\n  %s#%d %s%s\n", bold, pr.Number, pr.Title, reset)
	fmt.Printf("  %s%s · %s · +%d/-%d%s\n", dim, pr.Author.Login, pr.Branch, pr.Additions, pr.Deletions, reset)
	if pr.IsDraft {
		fmt.Printf("  %s[DRAFT]%s\n", yellow, reset)
	}
	fmt.Printf("  %s\n", pr.URL)

	// Mergeable
	fmt.Printf("\n  %sMergeable:%s ", bold, reset)
	switch pr.Mergeable {
	case "MERGEABLE":
		fmt.Printf("%s✓ yes%s\n", green, reset)
	case "CONFLICTING":
		fmt.Printf("%s✗ conflicts%s\n", red, reset)
	default:
		fmt.Printf("%s? %s%s\n", yellow, pr.Mergeable, reset)
	}

	// CI Status — summary + failures only
	fmt.Printf("\n  %sChecks:%s\n", bold, reset)
	if len(pr.StatusCheckRollup) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	} else {
		pass, fail, pending := 0, 0, 0
		var failures []string
		for _, check := range pr.StatusCheckRollup {
			switch check.Conclusion {
			case "SUCCESS":
				pass++
			case "FAILURE":
				fail++
				failures = append(failures, check.Name)
			default:
				if check.Status == "IN_PROGRESS" || check.Status == "QUEUED" || check.Status == "PENDING" {
					pending++
				} else if check.Conclusion == "" && check.Status == "" {
					pending++
				} else {
					pass++ // NEUTRAL, SKIPPED, etc.
				}
			}
		}
		// Summary line
		var summary []string
		if pass > 0 {
			summary = append(summary, fmt.Sprintf("%s%d✓%s", green, pass, reset))
		}
		if fail > 0 {
			summary = append(summary, fmt.Sprintf("%s%d✗%s", red, fail, reset))
		}
		if pending > 0 {
			summary = append(summary, fmt.Sprintf("%s%d◌%s", yellow, pending, reset))
		}
		fmt.Printf("    %s\n", strings.Join(summary, "  "))

		// Only show failures
		if len(failures) > 0 {
			fmt.Printf("\n    %sFailing:%s\n", bold, reset)
			for _, name := range failures {
				fmt.Printf("      %s✗%s %s\n", red, reset, name)
			}
		}
	}

	// Reviews
	fmt.Printf("\n  %sReviews:%s\n", bold, reset)
	if len(pr.Reviews) == 0 {
		fmt.Printf("    %s(none yet)%s\n", dim, reset)
	}
	for _, r := range pr.Reviews {
		icon := "·"
		color := dim
		switch r.State {
		case "APPROVED":
			icon = "✓"
			color = green
		case "CHANGES_REQUESTED":
			icon = "✗"
			color = red
		case "COMMENTED":
			icon = "💬"
		}
		fmt.Printf("    %s%s%s %s %s(%s)%s\n", color, icon, reset, r.Author.Login, dim, r.State, reset)
	}

	// Files changed
	fmt.Printf("\n  %sFiles (%d):%s\n", bold, len(pr.Files), reset)
	max := 10
	if len(pr.Files) < max {
		max = len(pr.Files)
	}
	for _, f := range pr.Files[:max] {
		fmt.Printf("    %s\n", f.Path)
	}
	if len(pr.Files) > 10 {
		fmt.Printf("    %s...and %d more%s\n", dim, len(pr.Files)-10, reset)
	}

	// Linked issues (parse from body)
	fmt.Printf("\n  %sLinked issues:%s\n", bold, reset)
	linked := findLinkedIssues(pr.Body)
	if len(linked) == 0 {
		fmt.Printf("    %s(none found in description)%s\n", dim, reset)
	}
	for _, ref := range linked {
		fmt.Printf("    %s\n", ref)
	}

	fmt.Println()
}

func findLinkedIssues(body string) []string {
	var refs []string
	seen := map[string]bool{}

	// Match #123 style refs (but not ## markdown headers)
	for _, m := range issueRefRe.FindAllStringSubmatch(body, -1) {
		ref := "#" + m[1]
		if !seen[ref] {
			refs = append(refs, ref)
			seen[ref] = true
		}
	}

	// Match full GitHub URLs
	for _, url := range issueURLRe.FindAllString(body, -1) {
		if !seen[url] {
			refs = append(refs, url)
			seen[url] = true
		}
	}

	return refs
}
