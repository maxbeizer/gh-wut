package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/prompter"
)

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
