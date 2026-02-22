package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

type SearchPR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	URL       string `json:"url"`
	IsDraft   bool   `json:"isDraft"`
	UpdatedAt string `json:"updatedAt"`
}

type SearchIssue struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
}

func cmdDashboard(repos, orgs []string, jsonOutput, interactive bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()
	fmt.Printf("\n  %swut's on my plate?%s %s@%s%s\n\n", bold, reset, dim, user, reset)

	scopeArgs := searchScopeArgs(repos, orgs)

	// Fetch all three sections concurrently
	var myPRs []SearchPR
	var reviewPRs []SearchPR
	var assignedIssues []SearchIssue
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,isDraft,updatedAt"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &myPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--review-requested", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &reviewPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &assignedIssues)
		}
	}()

	wg.Wait()

	if jsonOutput {
		if myPRs == nil {
			myPRs = []SearchPR{}
		}
		if reviewPRs == nil {
			reviewPRs = []SearchPR{}
		}
		if assignedIssues == nil {
			assignedIssues = []SearchIssue{}
		}
		printJSON(DashboardOutput{
			OpenPRs:        myPRs,
			ReviewRequests: reviewPRs,
			AssignedIssues: assignedIssues,
		})
		return
	}

	// Print results in consistent order
	fmt.Printf("  %sYour open PRs:%s\n", bold, reset)
	if len(myPRs) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, pr := range myPRs {
		draft := ""
		if pr.IsDraft {
			draft = " [draft]"
		}
		age := timeAgo(pr.UpdatedAt)
		fmt.Printf("    #%-6d %-35s %s%-20s %s%s%s\n",
			pr.Number, truncate(pr.Title, 35), dim, pr.Repository.NameWithOwner, age, draft, reset)
	}

	fmt.Printf("\n  %sAwaiting your review:%s\n", bold, reset)
	if len(reviewPRs) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, pr := range reviewPRs {
		age := timeAgo(pr.UpdatedAt)
		fmt.Printf("    #%-6d %-35s %s%-20s %s%s\n",
			pr.Number, truncate(pr.Title, 35), dim, pr.Repository.NameWithOwner, age, reset)
	}

	fmt.Printf("\n  %sAssigned to you:%s\n", bold, reset)
	if len(assignedIssues) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, issue := range assignedIssues {
		age := timeAgo(issue.UpdatedAt)
		fmt.Printf("    #%-6d %-35s %s%-20s %s%s\n",
			issue.Number, truncate(issue.Title, 35), dim, issue.Repository.NameWithOwner, age, reset)
	}

	fmt.Println()

	// Interactive selection
	var items []selectableItem
	for _, pr := range myPRs {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("PR #%d  %s  (%s)", pr.Number, truncate(pr.Title, 40), pr.Repository.NameWithOwner),
			URL:   pr.URL,
		})
	}
	for _, pr := range reviewPRs {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("Review #%d  %s  (%s)", pr.Number, truncate(pr.Title, 40), pr.Repository.NameWithOwner),
			URL:   pr.URL,
		})
	}
	for _, issue := range assignedIssues {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("Issue #%d  %s  (%s)", issue.Number, truncate(issue.Title, 40), issue.Repository.NameWithOwner),
			URL:   issue.URL,
		})
	}
	offerSelection(items, interactive)
}
