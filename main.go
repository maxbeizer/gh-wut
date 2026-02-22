package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

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
  --since <date>                Since when (yesterday, monday..sunday, last-week, 2025-01-15)
      --json                    Output as JSON (for scripting)
  -h, --help                    Show help
  -v, --version                 Show version

Aliases: context|ctx, catch-up|catchup|cu, story|st, dashboard|dash|db, standup|su, focus|f, review|rv, blockers|bl
`, version)
}

func main() {
	if len(os.Args) < 2 {
		if r := detectRepo(); r != "" {
			cmdContext([]string{r}, false)
		} else {
			cmdPicker(nil)
		}
		return
	}

	// Extract repo flags, --since, and --json from anywhere in args
	var repos []string
	var cleanArgs []string
	sinceArg := ""
	jsonOutput := false
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "-R" || os.Args[i] == "--repo") && i+1 < len(os.Args) {
			i++
			repos = append(repos, os.Args[i])
		} else if os.Args[i] == "--since" && i+1 < len(os.Args) {
			i++
			sinceArg = os.Args[i]
		} else if os.Args[i] == "--json" {
			jsonOutput = true
		} else {
			cleanArgs = append(cleanArgs, os.Args[i])
		}
	}

	// Load default repos from config if none specified via flags
	if len(repos) == 0 {
		repos = listRepos()
	}

	if len(cleanArgs) == 0 {
		if len(repos) > 0 {
			cmdContext(repos, jsonOutput)
		} else if r := detectRepo(); r != "" {
			cmdContext([]string{r}, jsonOutput)
		} else {
			cmdPicker(repos)
		}
		return
	}

	switch cleanArgs[0] {
	case "context", "ctx":
		cmdContext(repos, jsonOutput)
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
		cmdDashboard(repos, jsonOutput)
	case "standup", "su":
		cmdStandup(repos, sinceArg, jsonOutput)
	case "focus", "f":
		cmdFocus(repos, jsonOutput)
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
		cmdBlockers(repos, jsonOutput)
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
		cmdContext(repos, false)
	case 1:
		cmdCatchUp(repos, false)
	case 2:
		cmdStory(repo, "", false)
	case 3:
		cmdDashboard(repos, false)
	case 4:
		cmdStandup(repos, "", false)
	case 5:
		cmdFocus(repos, false)
	case 6:
		cmdReview(repo, "", false)
	case 7:
		cmdBlockers(repos, false)
	}
}

// --- JSON output types ---

type ContextOutput struct {
	Repo    string   `json:"repo"`
	Branch  string   `json:"branch,omitempty"`
	PRs     []PR     `json:"open_prs"`
	Issues  []Issue  `json:"assigned_issues"`
	Commits []string `json:"recent_commits"`
}

type CatchUpOutput struct {
	ReviewRequested []Notification `json:"review_requested"`
	Mentions        []Notification `json:"mentions"`
	Assigned        []Notification `json:"assigned"`
	CIActivity      []Notification `json:"ci_activity"`
	Other           []Notification `json:"other"`
}

type DashboardOutput struct {
	OpenPRs        []SearchPR    `json:"open_prs"`
	ReviewRequests []SearchPR    `json:"review_requests"`
	AssignedIssues []SearchIssue `json:"assigned_issues"`
}

type StandupOutput struct {
	Since        string        `json:"since"`
	MergedPRs    []SearchPR    `json:"merged_prs"`
	ClosedIssues []SearchIssue `json:"closed_issues"`
	Commits      []string      `json:"commits"`
}

type FocusOutput struct {
	Action string `json:"action"`
	Icon   string `json:"icon"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Repo   string `json:"repo"`
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

func printJSON(v interface{}) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
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

// --- Context: Where was I? ---

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

func cmdContext(repos []string, jsonOutput bool) {
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

	for _, repo := range repos {
		showContextForRepo(repo, user)
	}
}

func showContextForRepo(repo, user string) {
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
		"--jq", `.[] | "  " + (.commit.message | split("\n")[0] | .[0:55]) + "  \u001b[2m" + (.commit.author.date | .[0:10]) + "\u001b[0m"`).Output()
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

// --- Catch-Up: What happened while I was gone? ---

type Notification struct {
	Reason     string `json:"reason"`
	Subject    struct {
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

	// Search for recent mentions
	mentionQuery := fmt.Sprintf("q=mentions:%s%s updated:>%s", user, repoScope, daysAgo(7))
	out, err := exec.Command("gh", "api", "search/issues",
		"-f", mentionQuery,
		"--jq", `.items[:10] | .[] | "#" + (.number|tostring) + " " + .title + " (" + .repository_url + ")"`).Output()
	if err == nil && len(out) > 0 {
		fmt.Printf("  %sRecent mentions:%s\n", bold, reset)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
	} else {
		fmt.Printf("  %sNo recent mentions found.%s\n", dim, reset)
	}

	// Search for PRs needing your review
	reviewQuery := fmt.Sprintf("q=type:pr review-requested:%s%s state:open", user, repoScope)
	out2, err := exec.Command("gh", "api", "search/issues",
		"-f", reviewQuery,
		"--jq", `.items[:10] | .[] | "#" + (.number|tostring) + " " + .title`).Output()
	if err == nil && len(out2) > 0 {
		fmt.Printf("\n  %sPRs awaiting your review:%s\n", bold, reset)
		for _, line := range strings.Split(strings.TrimSpace(string(out2)), "\n") {
			if line != "" {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	fmt.Println()
}

// --- Story: Full PR context ---

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

var issueRefRe = regexp.MustCompile(`(?:^|\s)#(\d+)`)
var issueURLRe = regexp.MustCompile(`https?://github\.com/[^\s/]+/[^\s/]+/(?:issues|pull)/\d+`)

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

// --- Review: Reviewer-focused PR view ---

type ReviewPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Branch    string `json:"headRefName"`
	URL       string `json:"url"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Author    struct{ Login string } `json:"author"`
	Reviews   []struct {
		Author      struct{ Login string } `json:"author"`
		State       string                 `json:"state"`
		SubmittedAt string                 `json:"submittedAt"`
	} `json:"reviews"`
	ReviewRequests []struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"reviewRequests"`
	StatusCheckRollup []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

type ReviewComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	User      struct{ Login string } `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	InReplyTo *struct {
		ID int `json:"id"`
	} `json:"in_reply_to_id,omitempty"`
	SubjectType       string `json:"subject_type"`
	PullRequestReview struct {
		ID int `json:"id"`
	} `json:"pull_request_review_id,omitempty"`
	DiffHunk string `json:"diff_hunk"`
	Line     *int   `json:"line"`
}

type ReviewThread struct {
	Path       string `json:"path"`
	Body       string `json:"body"`
	User       string `json:"user"`
	DiffHunk   string `json:"diff_hunk"`
	Line       *int   `json:"line"`
	IsResolved bool   `json:"is_resolved"`
	Replies    int    `json:"replies"`
}

func cmdReview(repo, prRef string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

	if prRef == "" {
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

	// Determine repo for API calls
	effectiveRepo := repo
	if effectiveRepo == "" {
		effectiveRepo = detectRepo()
	}

	// Fetch PR details and review comments concurrently
	var pr ReviewPR
	var comments []ReviewComment
	var prErr, commentsErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		args := []string{"pr", "view", prRef,
			"--json", "number,title,author,headRefName,additions,deletions,statusCheckRollup,reviewRequests,reviews,url"}
		if repo != "" {
			args = append(args, "--repo", repo)
		}
		out, err := exec.Command("gh", args...).Output()
		if err != nil {
			prErr = err
			return
		}
		_ = json.Unmarshal(out, &pr)
	}()

	go func() {
		defer wg.Done()
		if effectiveRepo == "" {
			return
		}
		apiPR := prRef
		isNumeric := true
		for _, c := range prRef {
			if c < '0' || c > '9' {
				isNumeric = false
				break
			}
		}
		if !isNumeric {
			return
		}
		endpoint := fmt.Sprintf("repos/%s/pulls/%s/comments?per_page=100", effectiveRepo, apiPR)
		out, err := exec.Command("gh", "api", endpoint, "--paginate").Output()
		if err != nil {
			commentsErr = err
			return
		}
		_ = json.Unmarshal(out, &comments)
	}()

	wg.Wait()

	if prErr != nil {
		fmt.Fprintf(os.Stderr, "Error fetching PR: %v\n", prErr)
		os.Exit(1)
	}

	// If we couldn't fetch comments concurrently (non-numeric ref), fetch now
	if comments == nil && commentsErr == nil && effectiveRepo != "" && pr.Number > 0 {
		endpoint := fmt.Sprintf("repos/%s/pulls/%d/comments?per_page=100", effectiveRepo, pr.Number)
		out, err := exec.Command("gh", "api", endpoint, "--paginate").Output()
		if err == nil {
			_ = json.Unmarshal(out, &comments)
		}
	}

	threads := buildReviewThreads(comments)

	if jsonOutput {
		printJSON(struct {
			PR      ReviewPR       `json:"pr"`
			Threads []ReviewThread `json:"threads"`
		}{PR: pr, Threads: threads})
		return
	}

	// Header
	fmt.Printf("\n  %s#%d %s%s\n", bold, pr.Number, pr.Title, reset)
	fmt.Printf("  %s%s · %s · +%d/-%d%s\n", dim, pr.Author.Login, pr.Branch, pr.Additions, pr.Deletions, reset)
	fmt.Printf("  %s\n", pr.URL)

	// CI Status
	fmt.Printf("\n  %sChecks:%s ", bold, reset)
	if len(pr.StatusCheckRollup) == 0 {
		fmt.Printf("%s(none)%s\n", dim, reset)
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
					pass++
				}
			}
		}
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
		fmt.Printf("%s\n", strings.Join(summary, "  "))

		if len(failures) > 0 {
			for _, name := range failures {
				fmt.Printf("    %s✗%s %s\n", red, reset, name)
			}
		}
	}

	totalThreads := len(threads)
	resolved := 0
	unresolved := 0
	for _, t := range threads {
		if t.IsResolved {
			resolved++
		} else {
			unresolved++
		}
	}

	fmt.Printf("\n  %sReview threads:%s %d total — %s%d resolved%s, %s%d unresolved%s\n",
		bold, reset, totalThreads, green, resolved, reset, red, unresolved, reset)

	// Check if author pushed new commits since your last review
	user := ghUser()
	var yourLastReview string
	for i := len(pr.Reviews) - 1; i >= 0; i-- {
		if pr.Reviews[i].Author.Login == user {
			yourLastReview = pr.Reviews[i].SubmittedAt
			break
		}
	}

	if yourLastReview != "" {
		reviewTime, err := time.Parse(time.RFC3339, yourLastReview)
		if err == nil && effectiveRepo != "" {
			endpoint := fmt.Sprintf("repos/%s/pulls/%d/commits?per_page=100", effectiveRepo, pr.Number)
			out, err := exec.Command("gh", "api", endpoint,
				"--jq", fmt.Sprintf(`[.[] | select(.commit.committer.date > "%s")] | length`, reviewTime.Format(time.RFC3339))).Output()
			if err == nil {
				countStr := strings.TrimSpace(string(out))
				if countStr != "0" && countStr != "" {
					fmt.Printf("\n  %s⚠ %s pushed %s new commit(s) since your last review%s %s(%s)%s\n",
						yellow, pr.Author.Login, countStr, reset, dim, timeAgo(yourLastReview), reset)
				} else {
					fmt.Printf("\n  %s✓ No new commits since your last review%s %s(%s)%s\n",
						green, reset, dim, timeAgo(yourLastReview), reset)
				}
			}
		}
	} else {
		fmt.Printf("\n  %sYou haven't reviewed this PR yet.%s\n", dim, reset)
	}

	// Show unresolved threads
	if unresolved > 0 {
		fmt.Printf("\n  %sUnresolved threads:%s\n", bold, reset)
		for _, t := range threads {
			if t.IsResolved {
				continue
			}
			lineInfo := ""
			if t.Line != nil {
				lineInfo = fmt.Sprintf(":%d", *t.Line)
			}
			fmt.Printf("\n    %s%s%s%s%s\n", cyan, t.Path, lineInfo, reset, "")
			if t.DiffHunk != "" {
				hunkLines := strings.Split(t.DiffHunk, "\n")
				start := len(hunkLines) - 3
				if start < 0 {
					start = 0
				}
				for _, hl := range hunkLines[start:] {
					fmt.Printf("    %s│%s %s\n", dim, reset, truncate(hl, 72))
				}
			}
			body := strings.TrimSpace(t.Body)
			if len(body) > 120 {
				body = body[:117] + "..."
			}
			fmt.Printf("    %s💬 %s:%s %s\n", dim, t.User, reset, body)
			if t.Replies > 0 {
				fmt.Printf("    %s↳ %d repl%s%s\n", dim, t.Replies, pluralY(t.Replies), reset)
			}
		}
	}

	fmt.Println()
}

func buildReviewThreads(comments []ReviewComment) []ReviewThread {
	type threadKey struct {
		Path     string
		DiffHunk string
	}

	threadMap := map[threadKey]*ReviewThread{}
	var threadOrder []threadKey

	for _, c := range comments {
		key := threadKey{Path: c.Path, DiffHunk: c.DiffHunk}
		if existing, ok := threadMap[key]; ok {
			existing.Replies++
		} else {
			t := &ReviewThread{
				Path:     c.Path,
				Body:     c.Body,
				User:     c.User.Login,
				DiffHunk: c.DiffHunk,
				Line:     c.Line,
			}
			threadMap[key] = t
			threadOrder = append(threadOrder, key)
		}
	}

	var threads []ReviewThread
	for _, key := range threadOrder {
		threads = append(threads, *threadMap[key])
	}
	return threads
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// --- Dashboard: Cross-repo overview ---

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

func cmdDashboard(repos []string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()
	fmt.Printf("\n  %swut's on my plate?%s %s@%s%s\n\n", bold, reset, dim, user, reset)

	// Build repo filter args for gh search
	var repoArgs []string
	for _, r := range repos {
		repoArgs = append(repoArgs, "--repo", r)
	}

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
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &myPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--review-requested", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &reviewPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, repoArgs...)
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
}

// --- Focus: Just tell me what to do next ---

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

func cmdFocus(repos []string, jsonOutput bool) {
	user := ghUser()

	var repoArgs []string
	for _, r := range repos {
		repoArgs = append(repoArgs, "--repo", r)
	}

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
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &myPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--review-requested", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository"}
		args = append(args, repoArgs...)
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
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &noReviewPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository"}
		args = append(args, repoArgs...)
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

// --- Blockers: What's stuck? ---

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

func cmdBlockers(repos []string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	red := "\033[31m"
	yellow := "\033[33m"
	reset := "\033[0m"

	user := ghUser()

	if !jsonOutput {
		fmt.Printf("\n  %swut's stuck?%s %s@%s%s\n\n", bold, reset, dim, user, reset)
	}

	var repoArgs []string
	for _, r := range repos {
		repoArgs = append(repoArgs, "--repo", r)
	}

	var myPRs []BlockerPR
	var reviewRequestedPRs []BlockerPR
	var assignedIssues []BlockerIssue
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--author", user, "--state", "open",
			"--limit", "30", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &myPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "prs", "--review-requested", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, repoArgs...)
		out, err := exec.Command("gh", args...).Output()
		if err == nil {
			_ = json.Unmarshal(out, &reviewRequestedPRs)
		}
	}()

	go func() {
		defer wg.Done()
		args := []string{"search", "issues", "--assignee", user, "--state", "open",
			"--limit", "15", "--json", "number,title,repository,url,updatedAt"}
		args = append(args, repoArgs...)
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

	// Section 4: Assigned issues with no linked PR
	fmt.Printf("\n  %s📌 Issues with no linked PR:%s\n", bold, reset)
	unlinkedFound := false
	for _, issue := range assignedIssues {
		if !issueHasLinkedPR(issue.Repository.NameWithOwner, issue.Number) {
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

// --- Helpers ---

var (
	cachedUser     string
	cachedUserOnce sync.Once
)

func ghUser() string {
	cachedUserOnce.Do(func() {
		out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
		if err != nil {
			return
		}
		cachedUser = strings.TrimSpace(string(out))
	})
	return cachedUser
}

func detectRepo() string {
	out, err := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func currentBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func timeAgo(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func daysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02")
}

// --- Standup: What did I do? ---

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

func cmdStandup(repos []string, sinceArg string, jsonOutput bool) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()
	since := parseSince(sinceArg)

	fmt.Printf("\n  %swut did I do?%s %s@%s since %s%s\n\n", bold, reset, dim, user, since, reset)

	// Build repo filter args for gh search
	var repoArgs []string
	for _, r := range repos {
		repoArgs = append(repoArgs, "--repo", r)
	}

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
		args = append(args, repoArgs...)
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
		args = append(args, repoArgs...)
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
