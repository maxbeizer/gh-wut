package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

type FocusPR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	StatusCheckRollup []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

type FocusReviewPR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

type FocusIssue struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

type FocusOutput struct {
	Action string `json:"action"`
	Icon   string `json:"icon"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Repo   string `json:"repo"`
}

func cmdFocus(repos, orgs []string, jsonOutput bool) {
	user := ghUser()

	scopeArgs := searchScopeArgs(repos, orgs)

	var myPRs []FocusPR
	var reviewPRs []FocusReviewPR
	var noReviewPRs []FocusPR
	var assignedIssues []FocusIssue
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--state", "open",
			"--limit", "30", "--json", "number,title,repository,statusCheckRollup"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &myPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--review-requested", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &reviewPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--state", "open",
			"--review", "none",
			"--limit", "15", "--json", "number,title,repository,statusCheckRollup"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &noReviewPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository"}
		args = append(args, scopeArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &assignedIssues)
		}
	}()

	wg.Wait()

	emitFocus := func(action, icon string, number int, title, repo string) {
		if jsonOutput {
			printJSON(FocusOutput{Action: action, Icon: icon, Number: number, Title: title, Repo: repo})
		} else {
			fmt.Printf("%s %s: #%d %s (%s)\n", icon, action, number, title, repo)
		}
	}

	// Priority 1: Your PR with failing CI
	for _, pr := range myPRs {
		for _, check := range pr.StatusCheckRollup {
			if check.Conclusion == "FAILURE" {
				emitFocus("Fix CI", "🔴", pr.Number, pr.Title, pr.Repository.NameWithOwner)
				return
			}
		}
	}

	// Priority 2: Review request waiting for you
	if len(reviewPRs) > 0 {
		pr := reviewPRs[0]
		emitFocus("Review", "👀", pr.Number, pr.Title, pr.Repository.NameWithOwner)
		return
	}

	// Priority 3: Your PR with no reviews yet (skip those with failing CI)
	for _, pr := range noReviewPRs {
		hasFail := false
		for _, check := range pr.StatusCheckRollup {
			if check.Conclusion == "FAILURE" {
				hasFail = true
				break
			}
		}
		if !hasFail {
			emitFocus("Needs review", "⏳", pr.Number, pr.Title, pr.Repository.NameWithOwner)
			return
		}
	}

	// Priority 4: Assigned issue with no linked PR
	if len(assignedIssues) > 0 {
		issue := assignedIssues[0]
		emitFocus("Start", "📋", issue.Number, issue.Title, issue.Repository.NameWithOwner)
		return
	}

	// Nothing
	if jsonOutput {
		printJSON(FocusOutput{Action: "All clear", Icon: "✅"})
	} else {
		fmt.Println("✅ All clear. Go build something new.")
	}
}
