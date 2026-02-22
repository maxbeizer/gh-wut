package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type BlockerPR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
}

type BlockerIssue struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updatedAt"`
}

type BlockersOutput struct {
	FailingCI      []BlockerStatus `json:"failing_ci"`
	WaitingReview  []BlockerStatus `json:"waiting_review"`
	AwaitingYours  []BlockerPR     `json:"awaiting_your_review"`
	UnlinkedIssues []BlockerIssue  `json:"unlinked_issues"`
}

type BlockerStatus struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Repo   string `json:"repo"`
}

func cmdBlockers(repos, orgs []string, jsonOutput, interactive bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	red := "\033[31m"
	yellow := "\033[33m"
	reset := "\033[0m"

	user := ghUser()

	if !jsonOutput {
		fmt.Printf("\n  %swut's stuck?%s %s@%s%s\n\n", bold, reset, dim, user, reset)
	}

	scopeArgs := searchScopeArgs(repos, orgs)

	var myPRs []BlockerPR
	var reviewRequestedPRs []BlockerPR
	var assignedIssues []BlockerIssue
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--state", "open",
			"--limit", "30", "--json", "number,title,repository,url,updatedAt"}
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
			_ = json.Unmarshal(out, &reviewRequestedPRs)
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

	// Phase 2: For each of my PRs, concurrently fetch statusCheckRollup and reviews
	type prStatus struct {
		Number      int
		Title       string
		Repo        string
		URL         string
		FailCount   int
		HasApproval bool
	}

	statuses := make([]prStatus, len(myPRs))
	var wg2 sync.WaitGroup
	for i, pr := range myPRs {
		wg2.Add(1)
		go func(i int, pr BlockerPR) {
			defer wg2.Done()
			statuses[i] = prStatus{
				Number: pr.Number,
				Title:  pr.Title,
				Repo:   pr.Repository.NameWithOwner,
				URL:    pr.URL,
			}

			out, err := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", pr.Number),
				"--repo", pr.Repository.NameWithOwner,
				"--json", "statusCheckRollup,reviews").Output()
			if err != nil {
				return
			}

			var detail struct {
				StatusCheckRollup []struct {
					Conclusion string `json:"conclusion"`
				} `json:"statusCheckRollup"`
				Reviews []struct {
					State string `json:"state"`
				} `json:"reviews"`
			}
			_ = json.Unmarshal(out, &detail)

			for _, check := range detail.StatusCheckRollup {
				if check.Conclusion == "FAILURE" {
					statuses[i].FailCount++
				}
			}
			for _, r := range detail.Reviews {
				if r.State == "APPROVED" {
					statuses[i].HasApproval = true
					break
				}
			}
		}(i, pr)
	}
	wg2.Wait()

	if jsonOutput {
		var failing, waiting []BlockerStatus
		for _, s := range statuses {
			if s.FailCount > 0 {
				failing = append(failing, BlockerStatus{Number: s.Number, Title: s.Title, Repo: s.Repo})
			}
			if !s.HasApproval {
				waiting = append(waiting, BlockerStatus{Number: s.Number, Title: s.Title, Repo: s.Repo})
			}
		}
		var unlinked []BlockerIssue
		for _, issue := range assignedIssues {
			if !issueHasLinkedPR(issue.Repository.NameWithOwner, issue.Number) {
				unlinked = append(unlinked, issue)
			}
		}
		if failing == nil {
			failing = []BlockerStatus{}
		}
		if waiting == nil {
			waiting = []BlockerStatus{}
		}
		if reviewRequestedPRs == nil {
			reviewRequestedPRs = []BlockerPR{}
		}
		if unlinked == nil {
			unlinked = []BlockerIssue{}
		}
		printJSON(BlockersOutput{
			FailingCI:      failing,
			WaitingReview:  waiting,
			AwaitingYours:  reviewRequestedPRs,
			UnlinkedIssues: unlinked,
		})
		return
	}

	// Section 1: Failing CI
	fmt.Printf("  %s🔴 Failing CI:%s\n", bold, reset)
	failingFound := false
	for _, s := range statuses {
		if s.FailCount > 0 {
			failingFound = true
			fmt.Printf("    %sPR #%d: %s — %d✗ failing%s\n",
				red, s.Number, truncate(s.Title, 40), s.FailCount, reset)
		}
	}
	if !failingFound {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	// Section 2: Waiting on review (your PRs with no approving review)
	fmt.Printf("\n  %s🟡 Waiting on review:%s\n", bold, reset)
	waitingFound := false
	for _, s := range statuses {
		if !s.HasApproval {
			waitingFound = true
			fmt.Printf("    %sPR #%d: %s%s  %s%s%s\n",
				yellow, s.Number, truncate(s.Title, 40), reset, dim, s.Repo, reset)
		}
	}
	if !waitingFound {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	// Section 3: PRs awaiting YOUR review
	fmt.Printf("\n  %s👀 Awaiting your review:%s\n", bold, reset)
	if len(reviewRequestedPRs) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, pr := range reviewRequestedPRs {
		age := timeAgo(pr.UpdatedAt)
		fmt.Printf("    %sPR #%-6d %-35s%s  %s%-20s %s%s\n",
			yellow, pr.Number, truncate(pr.Title, 35), reset, dim, pr.Repository.NameWithOwner, age, reset)
	}

	// Section 4: Assigned issues with no linked PR (check concurrently)
	fmt.Printf("\n  %s📌 Issues with no linked PR:%s\n", bold, reset)
	hasLinkedPR := make([]bool, len(assignedIssues))
	var wg3 sync.WaitGroup
	for i, issue := range assignedIssues {
		wg3.Add(1)
		go func(i int, repo string, number int) {
			defer wg3.Done()
			hasLinkedPR[i] = issueHasLinkedPR(repo, number)
		}(i, issue.Repository.NameWithOwner, issue.Number)
	}
	wg3.Wait()

	unlinkedFound := false
	for i, issue := range assignedIssues {
		if !hasLinkedPR[i] {
			unlinkedFound = true
			age := timeAgo(issue.UpdatedAt)
			fmt.Printf("    %s#%-6d %-35s%s  %s%-20s %s%s\n",
				dim, issue.Number, truncate(issue.Title, 35), reset, dim, issue.Repository.NameWithOwner, age, reset)
		}
	}
	if !unlinkedFound {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	fmt.Println()

	// Interactive selection
	var items []selectableItem
	for _, s := range statuses {
		if s.FailCount > 0 {
			items = append(items, selectableItem{
				Label: fmt.Sprintf("🔴 PR #%d  %s  (%s)", s.Number, truncate(s.Title, 40), s.Repo),
				URL:   s.URL,
			})
		}
	}
	for _, s := range statuses {
		if !s.HasApproval {
			items = append(items, selectableItem{
				Label: fmt.Sprintf("🟡 PR #%d  %s  (%s)", s.Number, truncate(s.Title, 40), s.Repo),
				URL:   s.URL,
			})
		}
	}
	for _, pr := range reviewRequestedPRs {
		items = append(items, selectableItem{
			Label: fmt.Sprintf("👀 PR #%d  %s  (%s)", pr.Number, truncate(pr.Title, 40), pr.Repository.NameWithOwner),
			URL:   pr.URL,
		})
	}
	for i, issue := range assignedIssues {
		if !hasLinkedPR[i] {
			items = append(items, selectableItem{
				Label: fmt.Sprintf("📌 Issue #%d  %s  (%s)", issue.Number, truncate(issue.Title, 40), issue.Repository.NameWithOwner),
				URL:   issue.URL,
			})
		}
	}
	offerSelection(items, interactive)
}

func issueHasLinkedPR(repo string, number int) bool {
	out, err := exec.Command("gh", "api", "--paginate",
		fmt.Sprintf("repos/%s/issues/%d/timeline", repo, number),
		"--jq", `[.[] | select(.event == "cross-referenced" and .source.issue.pull_request != null)] | length`).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != "0"
}
