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
	"golang.org/x/term"
)

func printJSON(v interface{}) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}

// searchScopeArgs builds --repo and --owner args for gh search commands.
func searchScopeArgs(repos, orgs []string) []string {
	var args []string
	for _, r := range repos {
		args = append(args, "--repo", r)
	}
	for _, o := range orgs {
		args = append(args, "--owner", o)
	}
	return args
}

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

// isInteractive returns true if stdout is a terminal (not piped).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// selectableItem represents an item that can be opened in the browser.
type selectableItem struct {
	Label string
	URL   string
}

// offerSelection shows an interactive select prompt and opens the chosen item.
// Only shows when running in a terminal. Can be called repeatedly until user exits.
func offerSelection(items []selectableItem, interactive bool) {
	if !interactive || !isInteractive() || len(items) == 0 {
		return
	}

	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}

	p := prompter.New(os.Stdin, os.Stdout, os.Stderr)
	for {
		idx, err := p.Select("Open", "", labels)
		if err != nil {
			return
		}
		openURL(items[idx].URL)
	}
}

// openURL opens a URL in the default browser, cross-platform.
func openURL(url string) {
	for _, cmd := range [][]string{
		{"xdg-open", url},
		{"open", url},
		{"cmd", "/c", "start", url},
	} {
		if err := exec.Command(cmd[0], cmd[1:]...).Start(); err == nil {
			return
		}
	}
}
