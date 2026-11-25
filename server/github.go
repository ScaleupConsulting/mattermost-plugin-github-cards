package main

import "strings"

// The structs below decode only the webhook fields this plugin reads.
// Field names follow the GitHub REST webhook payloads.

type ghUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
	Type      string `json:"type"`
}

type ghTeam struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

type ghLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type ghRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         ghUser `json:"owner"`
}

type ghRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type ghPullRequest struct {
	Number             int       `json:"number"`
	HTMLURL            string    `json:"html_url"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	State              string    `json:"state"`
	Draft              bool      `json:"draft"`
	Merged             bool      `json:"merged"`
	MergedAt           string    `json:"merged_at"`
	MergedBy           *ghUser   `json:"merged_by"`
	MergeCommitSHA     string    `json:"merge_commit_sha"`
	User               ghUser    `json:"user"`
	Labels             []ghLabel `json:"labels"`
	Assignees          []ghUser  `json:"assignees"`
	RequestedReviewers []ghUser  `json:"requested_reviewers"`
	RequestedTeams     []ghTeam  `json:"requested_teams"`
	Head               ghRef     `json:"head"`
	Base               ghRef     `json:"base"`
	// The counts below are absent from the pull request inside review and review comment
	// payloads, so nil means unknown rather than zero.
	Additions      *int   `json:"additions"`
	Deletions      *int   `json:"deletions"`
	ChangedFiles   *int   `json:"changed_files"`
	Commits        *int   `json:"commits"`
	Comments       *int   `json:"comments"`
	ReviewComments *int   `json:"review_comments"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ghIssue struct {
	Number      int       `json:"number"`
	HTMLURL     string    `json:"html_url"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	StateReason string    `json:"state_reason"`
	User        ghUser    `json:"user"`
	Labels      []ghLabel `json:"labels"`
	Assignees   []ghUser  `json:"assignees"`
	Comments    int       `json:"comments"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	// PullRequest is present only when the issue is a pull request.
	PullRequest *struct {
		HTMLURL  string `json:"html_url"`
		MergedAt string `json:"merged_at"`
	} `json:"pull_request"`
}

type ghComment struct {
	ID        int64  `json:"id"`
	HTMLURL   string `json:"html_url"`
	Body      string `json:"body"`
	User      ghUser `json:"user"`
	Path      string `json:"path"`
	Line      *int   `json:"line"`
	DiffHunk  string `json:"diff_hunk"`
	InReplyTo int64  `json:"in_reply_to_id"`
}

type ghReview struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	State   string `json:"state"`
	User    ghUser `json:"user"`
}

type ghWorkflowRun struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	WorkflowID   int64  `json:"workflow_id"`
	HeadSHA      string `json:"head_sha"`
	HeadBranch   string `json:"head_branch"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HTMLURL      string `json:"html_url"`
	RunAttempt   int    `json:"run_attempt"`
	RunNumber    int    `json:"run_number"`
	DisplayTitle string `json:"display_title"`
	PullRequests []struct {
		Number int   `json:"number"`
		Head   ghRef `json:"head"`
		Base   ghRef `json:"base"`
	} `json:"pull_requests"`
}

type ghWorkflow struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ghCommit struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	URL     string `json:"url"`
	Author  struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"author"`
}

// ghEvent is the union of every payload shape this plugin handles; absent parts stay nil or zero.
type ghEvent struct {
	Action       string         `json:"action"`
	Sender       ghUser         `json:"sender"`
	Repository   *ghRepo        `json:"repository"`
	Organization *ghUser        `json:"organization"`
	PullRequest  *ghPullRequest `json:"pull_request"`
	Issue        *ghIssue       `json:"issue"`
	Comment      *ghComment     `json:"comment"`
	Review       *ghReview      `json:"review"`
	WorkflowRun  *ghWorkflowRun `json:"workflow_run"`
	Workflow     *ghWorkflow    `json:"workflow"`
	Changes      *struct {
		Body *struct {
			From string `json:"from"`
		} `json:"body"`
	} `json:"changes"`

	// push fields
	Ref     string     `json:"ref"`
	Before  string     `json:"before"`
	After   string     `json:"after"`
	Compare string     `json:"compare"`
	Forced  bool       `json:"forced"`
	Deleted bool       `json:"deleted"`
	Commits []ghCommit `json:"commits"`
	Pusher  struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

// isBot reports whether a GitHub account is an app or bot account.
func (u ghUser) isBot() bool {
	return u.Type == "Bot" || strings.HasSuffix(strings.ToLower(u.Login), "[bot]")
}

// person converts a GitHub account to the card's compact form.
func (u ghUser) person() Person {
	return Person{Login: u.Login, AvatarURL: u.AvatarURL, URL: u.HTMLURL, IsBot: u.isBot()}
}
