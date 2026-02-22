package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Notification struct {
	Reason  string `json:"reason"`
	Subject struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		URL   string `json:"url"`
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	UpdatedAt string `json:"updated_at"`
	Unread    bool   `json:"unread"`
}

func cmdCatchUp(repos []string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	if !jsonOutput {
		fmt.Printf("\n  %swut happened?%s\n\n", bold, reset)
	}

	// If repos specified, fetch notifications per-repo concurrently; otherwise global
	var allNotifs []Notification
	if len(repos) > 0 {
		type result struct{ notifs []Notification }
		results := make([]result, len(repos))
		var wg sync.WaitGroup
		for i, repo := range repos {
			wg.Add(1)
			go func(i int, repo string) {
				defer wg.Done()
				endpoint := fmt.Sprintf("repos/%s/notifications?per_page=30", repo)
				out, err := exec.Command("gh", "api", endpoint, "--paginate").Output()
				if err != nil {
					return
				}
				var notifs []Notification
				_ = json.Unmarshal(out, &notifs)
				results[i] = result{notifs}
			}(i, repo)
		}
		wg.Wait()
		for _, r := range results {
			allNotifs = append(allNotifs, r.notifs...)
		}
	} else {
		out, err := exec.Command("gh", "api", "notifications?per_page=30", "--paginate").Output()
		if err != nil {
			if jsonOutput {
				printJSON(CatchUpOutput{})
				return
			}
			fmt.Printf("  %sCouldn't fetch notifications.%s\n", dim, reset)
			fmt.Printf("  %s(Notifications API requires classic PAT or OAuth token)%s\n\n", dim, reset)
			fmt.Println("  Fallback: checking recent mentions...")
			fmt.Println()
			catchUpFallback(repos)
			return
		}
		_ = json.Unmarshal(out, &allNotifs)
	}

	if len(allNotifs) == 0 {
		fmt.Println("  Nothing new. Go touch grass.")
		fmt.Println()
		return
	}

	// Bucket by reason
	buckets := map[string][]Notification{
		"review_requested": {},
		"mention":          {},
		"assign":           {},
		"ci_activity":      {},
		"other":            {},
	}

	for _, n := range allNotifs {
		switch n.Reason {
		case "review_requested":
			buckets["review_requested"] = append(buckets["review_requested"], n)
		case "mention":
			buckets["mention"] = append(buckets["mention"], n)
		case "assign":
			buckets["assign"] = append(buckets["assign"], n)
		case "ci_activity":
			buckets["ci_activity"] = append(buckets["ci_activity"], n)
		default:
			buckets["other"] = append(buckets["other"], n)
		}
	}

	if jsonOutput {
		printJSON(CatchUpOutput{
			ReviewRequested: buckets["review_requested"],
			Mentions:        buckets["mention"],
			Assigned:        buckets["assign"],
			CIActivity:      buckets["ci_activity"],
			Other:           buckets["other"],
		})
		return
	}

	labels := []struct{ key, emoji, label string }{
		{"review_requested", "👀", "Review requested"},
		{"mention", "@", "Mentioned"},
		{"assign", "→", "Assigned"},
		{"ci_activity", "⚙", "CI"},
		{"other", "·", "Other"},
	}

	for _, l := range labels {
		items := buckets[l.key]
		if len(items) == 0 {
			continue
		}
		fmt.Printf("  %s %s%s%s (%d)\n", l.emoji, bold, l.label, reset, len(items))
		for _, n := range items {
			age := timeAgo(n.UpdatedAt)
			fmt.Printf("    %-45s %s%-15s %s%s\n",
				truncate(n.Subject.Title, 45),
				dim, n.Repository.FullName,
				age, reset)
		}
		fmt.Println()
	}
}

func catchUpFallback(repos []string) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()

	// Build repo scope for search queries
	repoScope := ""
	if len(repos) > 0 {
		var parts []string
		for _, r := range repos {
			parts = append(parts, "repo:"+r)
		}
		repoScope = " " + strings.Join(parts, " ")
	}

	// Fetch mentions and review requests concurrently
	var mentionsOut, reviewOut []byte
	var mentionsErr, reviewErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		mentionQuery := fmt.Sprintf("q=mentions:%s%s updated:>%s", user, repoScope, daysAgo(7))
		mentionsOut, mentionsErr = exec.Command("gh", "api", "search/issues",
			"-f", mentionQuery,
			"--jq", `.items[:10] | .[] | "#" + (.number|tostring) + " " + .title + " (" + .repository_url + ")"`).Output()
	}()
	go func() {
		defer wg.Done()
		reviewQuery := fmt.Sprintf("q=type:pr review-requested:%s%s state:open", user, repoScope)
		reviewOut, reviewErr = exec.Command("gh", "api", "search/issues",
			"-f", reviewQuery,
			"--jq", `.items[:10] | .[] | "#" + (.number|tostring) + " " + .title`).Output()
	}()
	wg.Wait()

	if mentionsErr == nil && len(mentionsOut) > 0 {
		fmt.Printf("  %sRecent mentions:%s\n", bold, reset)
		for _, line := range strings.Split(strings.TrimSpace(string(mentionsOut)), "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
	} else {
		fmt.Printf("  %sNo recent mentions found.%s\n", dim, reset)
	}

	if reviewErr == nil && len(reviewOut) > 0 {
		fmt.Printf("\n  %sPRs awaiting your review:%s\n", bold, reset)
		for _, line := range strings.Split(strings.TrimSpace(string(reviewOut)), "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	fmt.Println()
}
