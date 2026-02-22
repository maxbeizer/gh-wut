package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- parseSince ---

func TestParseSince_Yesterday(t *testing.T) {
	expected := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := parseSince("yesterday"); got != expected {
		t.Errorf("parseSince(\"yesterday\") = %q, want %q", got, expected)
	}
}

func TestParseSince_Empty(t *testing.T) {
	expected := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := parseSince(""); got != expected {
		t.Errorf("parseSince(\"\") = %q, want %q", got, expected)
	}
}

func TestParseSince_LastWeek(t *testing.T) {
	expected := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	if got := parseSince("last-week"); got != expected {
		t.Errorf("parseSince(\"last-week\") = %q, want %q", got, expected)
	}
}

func TestParseSince_Weekdays(t *testing.T) {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	for _, day := range days {
		t.Run(day, func(t *testing.T) {
			got := parseSince(day)
			parsed, err := time.Parse("2006-01-02", got)
			if err != nil {
				t.Fatalf("parseSince(%q) returned invalid date: %q", day, got)
			}
			if parsed.After(time.Now()) {
				t.Errorf("parseSince(%q) returned future date: %s", day, got)
			}
			now := time.Now()
			diff := int(now.Sub(parsed).Hours() / 24)
			if diff < 1 || diff > 7 {
				t.Errorf("parseSince(%q) = %s, expected 1-7 days ago (got %d days)", day, got, diff)
			}
		})
	}
}

func TestParseSince_ISODate(t *testing.T) {
	if got := parseSince("2025-01-15"); got != "2025-01-15" {
		t.Errorf("parseSince(\"2025-01-15\") = %q, want \"2025-01-15\"", got)
	}
}

func TestParseSince_Invalid(t *testing.T) {
	expected := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := parseSince("not-a-date"); got != expected {
		t.Errorf("parseSince(\"not-a-date\") = %q, want %q (yesterday)", got, expected)
	}
}

// --- findLinkedIssues ---

func TestFindLinkedIssues_HashRef(t *testing.T) {
	got := findLinkedIssues("fixes #123 in this PR")
	if len(got) != 1 || got[0] != "#123" {
		t.Errorf("findLinkedIssues with #123 = %v, want [#123]", got)
	}
}

func TestFindLinkedIssues_HeadingNotMatched(t *testing.T) {
	got := findLinkedIssues("## heading should not match")
	for _, ref := range got {
		if strings.HasPrefix(ref, "#") && !strings.HasPrefix(ref, "http") {
			t.Errorf("findLinkedIssues matched markdown heading: %v", got)
		}
	}
}

func TestFindLinkedIssues_TripleHashNotMatched(t *testing.T) {
	got := findLinkedIssues("### also not a ref")
	for _, ref := range got {
		if strings.HasPrefix(ref, "#") && !strings.HasPrefix(ref, "http") {
			t.Errorf("findLinkedIssues matched triple-hash heading: %v", got)
		}
	}
}

func TestFindLinkedIssues_FullURL(t *testing.T) {
	body := "See https://github.com/owner/repo/issues/42 for details"
	got := findLinkedIssues(body)
	found := false
	for _, ref := range got {
		if ref == "https://github.com/owner/repo/issues/42" {
			found = true
		}
	}
	if !found {
		t.Errorf("findLinkedIssues did not find full URL: got %v", got)
	}
}

func TestFindLinkedIssues_FixesKeyword(t *testing.T) {
	got := findLinkedIssues("Fixes #456")
	if len(got) != 1 || got[0] != "#456" {
		t.Errorf("findLinkedIssues(\"Fixes #456\") = %v, want [#456]", got)
	}
}

func TestFindLinkedIssues_Mixed(t *testing.T) {
	body := "Closes #10\nSee https://github.com/o/r/pull/20 and #30"
	got := findLinkedIssues(body)
	if len(got) < 3 {
		t.Errorf("findLinkedIssues mixed = %v, want at least 3 refs", got)
	}
}

func TestFindLinkedIssues_EmptyBody(t *testing.T) {
	got := findLinkedIssues("")
	if len(got) != 0 {
		t.Errorf("findLinkedIssues(\"\") = %v, want empty", got)
	}
}

// --- truncate ---

func TestTruncate_Short(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short = %q, want \"hello\"", got)
	}
}

func TestTruncate_ExactMax(t *testing.T) {
	if got := truncate("hello", 5); got != "hello" {
		t.Errorf("truncate exact = %q, want \"hello\"", got)
	}
}

func TestTruncate_NeedsTruncation(t *testing.T) {
	got := truncate("hello world", 6)
	if got != "hello…" {
		t.Errorf("truncate long = %q, want \"hello…\"", got)
	}
}

// --- timeAgo ---

func TestTimeAgo_Minutes(t *testing.T) {
	ts := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	got := timeAgo(ts)
	if got != "5m ago" {
		t.Errorf("timeAgo(5min) = %q, want \"5m ago\"", got)
	}
}

func TestTimeAgo_Hours(t *testing.T) {
	ts := time.Now().Add(-3 * time.Hour).Format(time.RFC3339)
	got := timeAgo(ts)
	if got != "3h ago" {
		t.Errorf("timeAgo(3h) = %q, want \"3h ago\"", got)
	}
}

func TestTimeAgo_Days(t *testing.T) {
	ts := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	got := timeAgo(ts)
	if got != "2d ago" {
		t.Errorf("timeAgo(2d) = %q, want \"2d ago\"", got)
	}
}

func TestTimeAgo_Invalid(t *testing.T) {
	if got := timeAgo("not-a-timestamp"); got != "" {
		t.Errorf("timeAgo(invalid) = %q, want \"\"", got)
	}
}

// --- searchScopeArgs ---

func TestSearchScopeArgs_NilNil(t *testing.T) {
	got := searchScopeArgs(nil, nil)
	if got != nil {
		t.Errorf("searchScopeArgs(nil, nil) = %v, want nil", got)
	}
}

func TestSearchScopeArgs_ReposOnly(t *testing.T) {
	got := searchScopeArgs([]string{"owner/repo"}, nil)
	expected := fmt.Sprintf("%v", []string{"--repo", "owner/repo"})
	if fmt.Sprintf("%v", got) != expected {
		t.Errorf("searchScopeArgs repos = %v, want %v", got, expected)
	}
}

func TestSearchScopeArgs_OrgsOnly(t *testing.T) {
	got := searchScopeArgs(nil, []string{"myorg"})
	expected := fmt.Sprintf("%v", []string{"--owner", "myorg"})
	if fmt.Sprintf("%v", got) != expected {
		t.Errorf("searchScopeArgs orgs = %v, want %v", got, expected)
	}
}

func TestSearchScopeArgs_Both(t *testing.T) {
	got := searchScopeArgs([]string{"o/r"}, []string{"org1"})
	expected := fmt.Sprintf("%v", []string{"--repo", "o/r", "--owner", "org1"})
	if fmt.Sprintf("%v", got) != expected {
		t.Errorf("searchScopeArgs both = %v, want %v", got, expected)
	}
}
