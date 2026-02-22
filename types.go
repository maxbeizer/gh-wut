package main

// JSON output types shared across commands.

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
