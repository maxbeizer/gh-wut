package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type StandupOutput struct {
	Since        string        `json:"since"`
	MergedPRs    []SearchPR    `json:"merged_prs"`
	ClosedIssues []SearchIssue `json:"closed_issues"`
	Commits      []string      `json:"commits"`
}

func parseSince(s string) string {
	if s == "" || s == "yesterday" {
		return time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}
	if s == "last-week" {
		return time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	// Day names: find the most recent occurrence of that weekday
	dayMap := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
	}
	if wd, ok := dayMap[strings.ToLower(s)]; ok {
		now := time.Now()
		diff := (int(now.Weekday()) - int(wd) + 7) % 7
		if diff == 0 {
			diff = 7
		}
		return now.AddDate(0, 0, -diff).Format("2006-01-02")
	}
	// Try ISO date
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	return time.Now().AddDate(0, 0, -1).Format("2006-01-02")
}

func cmdStandup(repos, orgs []string, sinceArg string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()
	since := parseSince(sinceArg)

	fmt.Printf("\n  %swut did I do?%s %s@%s since %s%s\n\n", bold, reset, dim, user, since, reset)

	scopeArgs := searchScopeArgs(repos, orgs)

	var mergedPRs []SearchPR
	var closedIssues []SearchIssue
	type repoCommits struct {
		repo    string
		commits []string
	}
	var allCommits []repoCommits
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--merged",
			"--sort", "updated", "--limit", "15",
			"--json", "number,title,repository,url,isDraft,updatedAt",
			"--", fmt.Sprintf("merged:>=%s", since)}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &mergedPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "closed",
			"--sort", "updated", "--limit", "15",
			"--json", "number,title,repository,url,updatedAt",
			"--", fmt.Sprintf("closed:>=%s", since)}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &closedIssues)
		}
	}()

	wg.Wait()

	// Fetch commits per repo concurrently
	commitRepos := repos
	if len(commitRepos) == 0 {
		r := detectRepo()
		if r != "" {
			commitRepos = []string{r}
		}
	}
	if len(commitRepos) > 0 {
		allCommits = make([]repoCommits, len(commitRepos))
		var cwg sync.WaitGroup
		for i, repo := range commitRepos {
			cwg.Add(1)
			go func(i int, repo string) {
				defer cwg.Done()
				out, err := exec.Command("gh", "api",
					fmt.Sprintf("repos/%s/commits?author=%s&since=%sT00:00:00Z&per_page=10", repo, user, since),
					"--jq", `.[] | (.commit.message | split("\n")[0] | .[0:55]) + "  \u001b[2m" + (.commit.author.date | .[0:10]) + "\u001b[0m"`).Output()
				if err != nil {
					return
				}
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) == 1 && lines[0] == "" {
					return
				}
				allCommits[i] = repoCommits{repo: repo, commits: lines}
			}(i, repo)
		}
		cwg.Wait()
	}

	if jsonOutput {
		if mergedPRs == nil {
			mergedPRs = []SearchPR{}
		}
		if closedIssues == nil {
			closedIssues = []SearchIssue{}
		}
		var flatCommits []string
		for _, rc := range allCommits {
			flatCommits = append(flatCommits, rc.commits...)
		}
		if flatCommits == nil {
			flatCommits = []string{}
		}
		printJSON(StandupOutput{
			Since:        since,
			MergedPRs:    mergedPRs,
			ClosedIssues: closedIssues,
			Commits:      flatCommits,
		})
		return
	}

	// Print merged PRs
	fmt.Printf("  %sMerged PRs:%s\n", bold, reset)
	if len(mergedPRs) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, pr := range mergedPRs {
		age := timeAgo(pr.UpdatedAt)
		fmt.Printf("    #%-6d %-35s %s%-20s %s%s\n",
			pr.Number, truncate(pr.Title, 35), dim, pr.Repository.NameWithOwner, age, reset)
	}

	// Print closed issues
	fmt.Printf("\n  %sClosed issues:%s\n", bold, reset)
	if len(closedIssues) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, issue := range closedIssues {
		age := timeAgo(issue.UpdatedAt)
		fmt.Printf("    #%-6d %-35s %s%-20s %s%s\n",
			issue.Number, truncate(issue.Title, 35), dim, issue.Repository.NameWithOwner, age, reset)
	}

	// Print commits
	fmt.Printf("\n  %sCommits pushed:%s\n", bold, reset)
	hasCommits := false
	for _, rc := range allCommits {
		if len(rc.commits) > 0 {
			hasCommits = true
			if len(commitRepos) > 1 {
				fmt.Printf("    %s%s%s\n", dim, rc.repo, reset)
			}
			for _, c := range rc.commits {
				fmt.Printf("    %s\n", c)
			}
		}
	}
	if !hasCommits {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	fmt.Println()
}
