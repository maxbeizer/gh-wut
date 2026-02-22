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

Flags:
  -R, --repo <owner/repo>       Target repo (repeatable; default: current)
  -h, --help                    Show help
  -v, --version                 Show version

Aliases: context|ctx, catch-up|catchup|cu, story|st, dashboard|dash|db
`, version)
}

func main() {
	if len(os.Args) < 2 {
		cmdPicker(nil)
		return
	}

	// Extract repo flags from anywhere in args (supports multiple -R)
	var repos []string
	var cleanArgs []string
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "-R" || os.Args[i] == "--repo") && i+1 < len(os.Args) {
			i++
			repos = append(repos, os.Args[i])
		} else {
			cleanArgs = append(cleanArgs, os.Args[i])
		}
	}

	if len(cleanArgs) == 0 {
		cmdPicker(repos)
		return
	}

	switch cleanArgs[0] {
	case "context", "ctx":
		cmdContext(repos)
	case "catch-up", "catchup", "cu":
		cmdCatchUp(repos)
	case "story", "st":
		pr := ""
		if len(cleanArgs) > 1 {
			pr = cleanArgs[1]
		}
		repo := ""
		if len(repos) > 0 {
			repo = repos[0]
		}
		cmdStory(repo, pr)
	case "dashboard", "dash", "db":
		cmdDashboard(repos)
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
		cmdContext(repos)
	case 1:
		cmdCatchUp(repos)
	case 2:
		cmdStory(repo, "")
	case 3:
		cmdDashboard(repos)
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

func cmdContext(repos []string) {
	if len(repos) == 0 {
		r := detectRepo()
		if r == "" {
			fmt.Fprintln(os.Stderr, "Not in a repo. Use -R owner/repo")
			os.Exit(1)
		}
		repos = []string{r}
	}

	user := ghUser()

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

func cmdCatchUp(repos []string) {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n  %swut happened?%s\n\n", bold, reset)

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

func cmdStory(repo, prRef string) {
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

func cmdDashboard(repos []string) {
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

// --- Helpers ---

func ghUser() string {
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
