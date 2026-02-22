package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type PR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Branch    string `json:"headRefName"`
	URL       string `json:"url"`
	IsDraft   bool   `json:"isDraft"`
	UpdatedAt string `json:"updatedAt"`
}

type Issue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
}

func cmdContext(repos []string, jsonOutput, interactive bool) {
	if len(repos) == 0 {
		r := detectRepo()
		if r == "" {
			fmt.Fprintln(os.Stderr, "Not in a repo. Use -R owner/repo")
			os.Exit(1)
		}
		repos = []string{r}
	}

	user := ghUser()

	if jsonOutput {
		var results []ContextOutput
		for _, repo := range repos {
			results = append(results, buildContextForRepo(repo, user))
		}
		if len(results) == 1 {
			printJSON(results[0])
		} else {
			printJSON(results)
		}
		return
	}

	var allItems []selectableItem
	for _, repo := range repos {
		allItems = append(allItems, showContextForRepo(repo, user)...)
	}
	offerSelection(allItems, interactive)
}

func showContextForRepo(repo, user string) []selectableItem {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n  %s%s%s %s@%s%s\n\n", bold, repo, reset, dim, user, reset)

	// Current branch
	branch := currentBranch()
	if branch != "" {
		fmt.Printf("  On branch: %s%s%s\n\n", bold, branch, reset)
	}

	// Fetch PRs, issues, and commits concurrently
	var prs []PR
	var issues []Issue
	var commits []string
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); prs = listPRs(repo, user) }()
	go func() { defer wg.Done(); issues = listIssues(repo, user) }()
	go func() { defer wg.Done(); commits = recentCommits(repo, user) }()
	wg.Wait()

	// Your open PRs
	fmt.Printf("  %sYour open PRs:%s\n", bold, reset)
	if len(prs) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, pr := range prs {
		draft := ""
		if pr.IsDraft {
			draft = " [draft]"
		}
		age := timeAgo(pr.UpdatedAt)
		fmt.Printf("    #%-4d %-35s %s%s  %s%s\n", pr.Number, truncate(pr.Title, 35), dim, age, draft, reset)
	}

	// Your assigned issues
	fmt.Printf("\n  %sAssigned issues:%s\n", bold, reset)
	if len(issues) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, issue := range issues {
		age := timeAgo(issue.UpdatedAt)
		fmt.Printf("    #%-4d %-35s %s%s%s\n", issue.Number, truncate(issue.Title, 35), dim, age, reset)
	}

	// Recent commits by you
	fmt.Printf("\n  %sYour recent commits:%s\n", bold, reset)
	if len(commits) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, c := range commits {
		fmt.Printf("    %s\n", c)
	}

	fmt.Println()

	// Build selectable items
	var items []selectableItem
	for _, pr := range prs {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("PR #%d  %s", pr.Number, truncate(pr.Title, 45)),
			URL:   pr.URL,
		})
	}
	for _, issue := range issues {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("Issue #%d  %s", issue.Number, truncate(issue.Title, 45)),
			URL:   issue.URL,
		})
	}
	return items
}

func listPRs(repo, user string) []PR {
	out, err := exec.Command("gh", "pr", "list", "--repo", repo, "--author", user,
		"--state", "open", "--json", "number,title,headRefName,url,isDraft,updatedAt").Output()
	if err != nil {
		return nil
	}
	var prs []PR
	_ = json.Unmarshal(out, &prs)
	return prs
}

func listIssues(repo, user string) []Issue {
	out, err := exec.Command("gh", "issue", "list", "--repo", repo, "--assignee", user,
		"--state", "open", "--json", "number,title,url,updatedAt").Output()
	if err != nil {
		return nil
	}
	var issues []Issue
	_ = json.Unmarshal(out, &issues)
	return issues
}

func recentCommits(repo, user string) []string {
	out, err := exec.Command("gh", "api", fmt.Sprintf("repos/%s/commits?author=%s&per_page=5", repo, user),
		"--jq", `.[] | (.commit.message | split("\n")[0] | .[0:55]) + "  \u001b[2m" + (.commit.author.date | .[0:10]) + "\u001b[0m"`).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func buildContextForRepo(repo, user string) ContextOutput {
	branch := currentBranch()

	var prs []PR
	var issues []Issue
	var commits []string
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); prs = listPRs(repo, user) }()
	go func() { defer wg.Done(); issues = listIssues(repo, user) }()
	go func() { defer wg.Done(); commits = recentCommitsRaw(repo, user) }()
	wg.Wait()

	if prs == nil {
		prs = []PR{}
	}
	if issues == nil {
		issues = []Issue{}
	}
	if commits == nil {
		commits = []string{}
	}

	return ContextOutput{
		Repo:    repo,
		Branch:  branch,
		PRs:     prs,
		Issues:  issues,
		Commits: commits,
	}
}

func recentCommitsRaw(repo, user string) []string {
	out, err := exec.Command("gh", "api", fmt.Sprintf("repos/%s/commits?author=%s&per_page=5", repo, user),
		"--jq", `.[] | (.commit.message | split("\n")[0] | .[0:80]) + " (" + (.commit.author.date | .[0:10]) + ")"`).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}
