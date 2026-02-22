package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const version = "0.1.0"

func usage() {
	fmt.Printf(`gh-wut v%s — What do I need to know right now?

Usage:
  gh wut                        Interactive picker
  gh wut context [-R repo]      Where was I? Open PRs, issues, recent commits
  gh wut catch-up               What happened? Notification triage
  gh wut story [pr-url]         Full PR story — issue, related PRs, CI
  gh wut dashboard              Cross-repo: everything with your name on it

Flags:
  -R, --repo <owner/repo>       Target repo (default: current)
  -h, --help                    Show help
  -v, --version                 Show version

Aliases: context|ctx, catch-up|catchup|cu, story|st, dashboard|dash|db
`, version)
}

func main() {
	if len(os.Args) < 2 {
		cmdPicker()
		return
	}

	// Extract repo flag from anywhere in args
	repo := ""
	var cleanArgs []string
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "-R" || os.Args[i] == "--repo") && i+1 < len(os.Args) {
			i++
			repo = os.Args[i]
		} else {
			cleanArgs = append(cleanArgs, os.Args[i])
		}
	}

	if len(cleanArgs) == 0 {
		cmdPicker()
		return
	}

	switch cleanArgs[0] {
	case "context", "ctx":
		cmdContext(repo)
	case "catch-up", "catchup", "cu":
		cmdCatchUp()
	case "story", "st":
		pr := ""
		if len(cleanArgs) > 1 {
			pr = cleanArgs[1]
		}
		cmdStory(repo, pr)
	case "dashboard", "dash", "db":
		cmdDashboard()
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

func cmdPicker() {
	fmt.Println("  wut?")
	fmt.Println()
	fmt.Println("  1) context    — Where was I?")
	fmt.Println("  2) catch-up   — What happened while I was gone?")
	fmt.Println("  3) story      — Full PR story")
	fmt.Println("  4) dashboard  — Everything, everywhere")
	fmt.Println()
	fmt.Print("  Choose [1-4]: ")

	var choice string
	fmt.Scanln(&choice)

	switch strings.TrimSpace(choice) {
	case "1", "context":
		cmdContext("")
	case "2", "catch-up", "catchup":
		cmdCatchUp()
	case "3", "story":
		cmdStory("", "")
	case "4", "dashboard":
		cmdDashboard()
	default:
		fmt.Fprintln(os.Stderr, "  ¯\\_(ツ)_/¯")
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

func cmdContext(repo string) {
	if repo == "" {
		repo = detectRepo()
	}
	if repo == "" {
		fmt.Fprintln(os.Stderr, "Not in a repo. Use -R owner/repo")
		os.Exit(1)
	}

	user := ghUser()
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n  %s%s%s %s@%s%s\n\n", bold, repo, reset, dim, user, reset)

	// Current branch
	branch := currentBranch()
	if branch != "" {
		fmt.Printf("  On branch: %s%s%s\n\n", bold, branch, reset)
	}

	// Your open PRs
	fmt.Printf("  %sYour open PRs:%s\n", bold, reset)
	prs := listPRs(repo, user)
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
	issues := listIssues(repo, user)
	if len(issues) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, issue := range issues {
		age := timeAgo(issue.UpdatedAt)
		fmt.Printf("    #%-4d %-35s %s%s%s\n", issue.Number, truncate(issue.Title, 35), dim, age, reset)
	}

	// Recent commits by you
	fmt.Printf("\n  %sYour recent commits:%s\n", bold, reset)
	commits := recentCommits(repo, user)
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

func cmdCatchUp() {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n  %swut happened?%s\n\n", bold, reset)

	out, err := exec.Command("gh", "api", "notifications?per_page=30", "--paginate").Output()
	if err != nil {
		// Notifications API may not work with fine-grained PATs
		fmt.Printf("  %sCouldn't fetch notifications.%s\n", dim, reset)
		fmt.Printf("  %s(Notifications API requires classic PAT or OAuth token)%s\n\n", dim, reset)
		fmt.Println("  Fallback: checking recent mentions...")
		fmt.Println()
		catchUpFallback()
		return
	}

	var notifs []Notification
	_ = json.Unmarshal(out, &notifs)

	if len(notifs) == 0 {
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

	for _, n := range notifs {
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

func catchUpFallback() {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	// Search for recent mentions
	user := ghUser()
	out, err := exec.Command("gh", "api", "search/issues",
		"-f", fmt.Sprintf("q=mentions:%s updated:>%s", user, daysAgo(7)),
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
	out2, err := exec.Command("gh", "api", "search/issues",
		"-f", fmt.Sprintf("q=type:pr review-requested:%s state:open", user),
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
		fmt.Print("  PR number or URL: ")
		fmt.Scanln(&prRef)
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

	// CI Status
	fmt.Printf("\n  %sChecks:%s\n", bold, reset)
	if len(pr.StatusCheckRollup) == 0 {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}
	for _, check := range pr.StatusCheckRollup {
		icon := "·"
		color := dim
		switch check.Conclusion {
		case "SUCCESS":
			icon = "✓"
			color = green
		case "FAILURE":
			icon = "✗"
			color = red
		case "":
			if check.Status == "IN_PROGRESS" {
				icon = "◌"
				color = yellow
			}
		}
		fmt.Printf("    %s%s%s %s\n", color, icon, reset, check.Name)
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

	// Match patterns like: fixes #123, closes #456, resolves #789, #123
	words := strings.Fields(body)
	for _, w := range words {
		w = strings.TrimRight(w, ".,;:!?)")
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			if !seen[w] {
				refs = append(refs, w)
				seen[w] = true
			}
		}
		// Also match full URLs
		if strings.Contains(w, "/issues/") || strings.Contains(w, "/pull/") {
			w = strings.TrimLeft(w, "(")
			if !seen[w] {
				refs = append(refs, w)
				seen[w] = true
			}
		}
	}
	return refs
}

// --- Dashboard: Cross-repo overview ---

func cmdDashboard() {
	dim := "\033[2m"
	bold := "\033[1m"
	reset := "\033[0m"

	user := ghUser()
	fmt.Printf("\n  %swut's on my plate?%s %s@%s%s\n\n", bold, reset, dim, user, reset)

	// Open PRs you authored
	fmt.Printf("  %sYour open PRs:%s\n", bold, reset)
	out, err := exec.Command("gh", "api", "search/issues",
		"-f", fmt.Sprintf("q=type:pr author:%s state:open", user),
		"--jq", `.items[:15] | .[] | "    #" + (.number|tostring) + "  " + (.title|.[0:40]) + "  \u001b[2m" + (.repository_url | split("/") | .[-2:] | join("/")) + "\u001b[0m"`).Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		fmt.Println(strings.TrimSpace(string(out)))
	} else {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	// PRs awaiting your review
	fmt.Printf("\n  %sAwaiting your review:%s\n", bold, reset)
	out2, err := exec.Command("gh", "api", "search/issues",
		"-f", fmt.Sprintf("q=type:pr review-requested:%s state:open", user),
		"--jq", `.items[:15] | .[] | "    #" + (.number|tostring) + "  " + (.title|.[0:40]) + "  \u001b[2m" + (.repository_url | split("/") | .[-2:] | join("/")) + "\u001b[0m"`).Output()
	if err == nil && len(strings.TrimSpace(string(out2))) > 0 {
		fmt.Println(strings.TrimSpace(string(out2)))
	} else {
		fmt.Printf("    %s(none)%s\n", dim, reset)
	}

	// Your assigned issues
	fmt.Printf("\n  %sAssigned to you:%s\n", bold, reset)
	out3, err := exec.Command("gh", "api", "search/issues",
		"-f", fmt.Sprintf("q=type:issue assignee:%s state:open", user),
		"--jq", `.items[:15] | .[] | "    #" + (.number|tostring) + "  " + (.title|.[0:40]) + "  \u001b[2m" + (.repository_url | split("/") | .[-2:] | join("/")) + "\u001b[0m"`).Output()
	if err == nil && len(strings.TrimSpace(string(out3))) > 0 {
		fmt.Println(strings.TrimSpace(string(out3)))
	} else {
		fmt.Printf("    %s(none)%s\n", dim, reset)
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
