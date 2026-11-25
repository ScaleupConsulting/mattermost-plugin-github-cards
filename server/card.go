package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// cardVersion is stored in every card so the webapp can tell which shape it is reading.
const cardVersion = 1

// Card kinds.
const (
	kindPR    = "pr"
	kindIssue = "issue"
)

// Normalized card states.
const (
	stateOpen       = "open"
	stateDraft      = "draft"
	stateClosed     = "closed"
	stateMerged     = "merged"
	stateNotPlanned = "not_planned"
)

// Overall CI results.
const (
	ciNone    = ""
	ciPending = "pending"
	ciSuccess = "success"
	ciFailure = "failure"
)

// Person is a GitHub account or team as shown on a card.
type Person struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url,omitempty"`
	URL       string `json:"url,omitempty"`
	IsBot     bool   `json:"is_bot,omitempty"`
	IsTeam    bool   `json:"is_team,omitempty"`
}

// Label is a GitHub label with its hex colour (no leading #).
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Review is one reviewer's current decision: approved, changes_requested, commented or dismissed.
type Review struct {
	Reviewer Person `json:"reviewer"`
	State    string `json:"state"`
	URL      string `json:"url,omitempty"`
}

// Check is the latest run of one workflow for the card's head commit.
type Check struct {
	WorkflowID int64  `json:"workflow_id"`
	Name       string `json:"name"`
	RunID      int64  `json:"run_id"`
	Attempt    int    `json:"attempt"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"`
	URL        string `json:"url"`
}

// LinkedPR is a pull request whose description says it closes an issue.
type LinkedPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

// Card is the full state of one pull request or issue card. It is stored in KV and copied
// into the root post's props, where the webapp renders it.
type Card struct {
	Version    int      `json:"v"`
	Kind       string   `json:"kind"`
	Repo       string   `json:"repo"`
	RepoURL    string   `json:"repo_url"`
	RepoID     int64    `json:"repo_id"`
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Author     Person   `json:"author"`
	State      string   `json:"state"`
	SnapshotAt string   `json:"snapshot_at,omitempty"`
	Labels     []Label  `json:"labels,omitempty"`
	Assignees  []Person `json:"assignees,omitempty"`
	Comments   int      `json:"comments"`

	// Pull request only.
	Head               string   `json:"head,omitempty"`
	Base               string   `json:"base,omitempty"`
	HeadSHA            string   `json:"head_sha,omitempty"`
	Additions          int      `json:"additions,omitempty"`
	Deletions          int      `json:"deletions,omitempty"`
	ChangedFiles       int      `json:"changed_files,omitempty"`
	Commits            int      `json:"commits,omitempty"`
	RequestedReviewers []Person `json:"requested_reviewers,omitempty"`
	Reviews            []Review `json:"reviews,omitempty"`
	Checks             []Check  `json:"checks,omitempty"`
	LastCIOutcome      string   `json:"last_ci_outcome,omitempty"`
	MergedBy           *Person  `json:"merged_by,omitempty"`
	MergeCommitSHA     string   `json:"merge_commit_sha,omitempty"`
	ClosesIssues       []int    `json:"closes_issues,omitempty"`
	IssueComments      int      `json:"issue_comments,omitempty"`
	ReviewComments     int      `json:"review_comments,omitempty"`

	// Issue only.
	LinkedPRs []LinkedPR `json:"linked_prs,omitempty"`
}

// Reply kinds posted in a card's thread or, for pushes, in the channel.
const (
	replyReview        = "review"
	replyReviewComment = "review_comment"
	replyComment       = "comment"
	replyState         = "state"
	replyCIFailed      = "ci_failed"
	replyCIPassed      = "ci_passed"
	replyPush          = "push"
)

// Reply is one event rendered as a post. CommentKey ties it to a GitHub comment so an edit or
// delete on GitHub can find the post again.
type Reply struct {
	Kind       string       `json:"kind"`
	Actor      Person       `json:"actor"`
	Verb       string       `json:"verb"`
	Tone       string       `json:"tone"`
	Context    string       `json:"context,omitempty"`
	Body       string       `json:"body,omitempty"`
	URL        string       `json:"url,omitempty"`
	Repo       string       `json:"repo,omitempty"`
	RepoURL    string       `json:"repo_url,omitempty"`
	Commits    []PushCommit `json:"commits,omitempty"`
	CommentKey string       `json:"-"`
}

// PushCommit is one commit line of a push reply.
type PushCommit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	URL     string `json:"url"`
	Author  string `json:"author"`
}

// effects is what one event does: the card after the event (nil when the event touches no card),
// thread replies to add, a reply to edit or delete, and issue cards to link to this pull request.
type effects struct {
	card       *Card
	replies    []Reply
	edit       *Reply
	deleteKey  string
	link       *LinkedPR
	linkIssues []int
}

// ciStatus reduces the per-workflow checks to one result.
func (c *Card) ciStatus() string {
	if len(c.Checks) == 0 {
		return ciNone
	}
	status := ciSuccess
	for _, ch := range c.Checks {
		switch {
		case ch.Status != "completed":
			if status != ciFailure {
				status = ciPending
			}
		case isFailedConclusion(ch.Conclusion):
			return ciFailure
		}
	}
	return status
}

// isFailedConclusion reports whether a completed run counts as a CI failure.
// Cancelled, skipped and neutral runs are not failures.
func isFailedConclusion(conclusion string) bool {
	switch conclusion {
	case "failure", "timed_out", "startup_failure", "action_required":
		return true
	}
	return false
}

// prState normalizes a pull request's state.
func prState(pr *ghPullRequest) string {
	switch {
	case pr.Merged || pr.MergedAt != "":
		return stateMerged
	case pr.State == "closed":
		return stateClosed
	case pr.Draft:
		return stateDraft
	}
	return stateOpen
}

// issueState normalizes an issue's state; an issue standing in for a pull request may be merged.
func issueState(is *ghIssue) string {
	switch {
	case is.PullRequest != nil && is.PullRequest.MergedAt != "":
		return stateMerged
	case is.State == "closed" && is.StateReason == "not_planned":
		return stateNotPlanned
	case is.State == "closed":
		return stateClosed
	}
	return stateOpen
}

// newCard starts an empty card for a repository and number.
func newCard(kind string, repo *ghRepo, number int) *Card {
	return &Card{Version: cardVersion, Kind: kind, Repo: repo.FullName, RepoURL: repo.HTMLURL, RepoID: repo.ID, Number: number}
}

// isNewer reports whether a payload snapshot taken at updatedAt may overwrite the card.
// GitHub can deliver events out of order, and an older snapshot must not undo a newer one.
func (c *Card) isNewer(updatedAt string) bool {
	return updatedAt == "" || c.SnapshotAt == "" || updatedAt >= c.SnapshotAt
}

// applyPR copies a pull request snapshot onto the card.
func (c *Card) applyPR(pr *ghPullRequest) {
	if !c.isNewer(pr.UpdatedAt) {
		return
	}
	c.Kind = kindPR
	c.SnapshotAt = pr.UpdatedAt
	c.Title, c.URL, c.Author = pr.Title, pr.HTMLURL, pr.User.person()
	c.State = prState(pr)
	c.Head, c.Base = pr.Head.Ref, pr.Base.Ref
	if pr.Head.SHA != "" && pr.Head.SHA != c.HeadSHA {
		c.HeadSHA = pr.Head.SHA
		c.Checks = nil
	}
	c.Labels = labels(pr.Labels)
	c.Assignees = people(pr.Assignees)
	c.RequestedReviewers = people(pr.RequestedReviewers)
	for _, t := range pr.RequestedTeams {
		c.RequestedReviewers = append(c.RequestedReviewers, Person{Login: t.Slug, URL: t.HTMLURL, IsTeam: true})
	}
	setIfKnown(&c.Additions, pr.Additions)
	setIfKnown(&c.Deletions, pr.Deletions)
	setIfKnown(&c.ChangedFiles, pr.ChangedFiles)
	setIfKnown(&c.Commits, pr.Commits)
	setIfKnown(&c.IssueComments, pr.Comments)
	setIfKnown(&c.ReviewComments, pr.ReviewComments)
	c.Comments = c.IssueComments + c.ReviewComments
	if pr.MergedBy != nil {
		p := pr.MergedBy.person()
		c.MergedBy = &p
	}
	c.MergeCommitSHA = pr.MergeCommitSHA
	c.ClosesIssues = closingRefs(pr.Body, c.Repo)
}

// setIfKnown copies a count the payload carries and keeps the old one when it does not.
func setIfKnown(dst, src *int) {
	if src != nil {
		*dst = *src
	}
}

// applyIssue copies an issue snapshot onto the card. On a pull request card only the fields an
// issue payload knows reliably are taken, and the state only while no pull request payload has.
func (c *Card) applyIssue(is *ghIssue) {
	if !c.isNewer(is.UpdatedAt) {
		return
	}
	c.SnapshotAt = is.UpdatedAt
	c.Title, c.URL, c.Author = is.Title, is.HTMLURL, is.User.person()
	c.Labels = labels(is.Labels)
	c.Assignees = people(is.Assignees)
	if is.PullRequest == nil {
		c.Kind = kindIssue
		c.State = issueState(is)
		c.Comments = is.Comments
		return
	}
	c.Kind = kindPR
	if c.Head == "" {
		c.State = issueState(is)
	}
	c.IssueComments = is.Comments
	c.Comments = c.IssueComments + c.ReviewComments
}

// labels converts GitHub labels.
func labels(in []ghLabel) []Label {
	var out []Label
	for _, l := range in {
		out = append(out, Label(l))
	}
	return out
}

// people converts GitHub accounts.
func people(in []ghUser) []Person {
	var out []Person
	for _, u := range in {
		out = append(out, u.person())
	}
	return out
}

var closingRefRE = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+(?:https://github\.com/([\w.-]+/[\w.-]+)/issues/|([\w.-]+/[\w.-]+)#|#)(\d+)`)

// closingRefs finds the issue numbers in this repository that a pull request body says it closes.
func closingRefs(body, repo string) []int {
	var out []int
	for _, m := range closingRefRE.FindAllStringSubmatch(body, -1) {
		other := m[1] + m[2]
		if other != "" && !strings.EqualFold(other, repo) {
			continue
		}
		n, err := strconv.Atoi(m[3])
		if err == nil && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// setReview records a reviewer's decision. A plain comment does not replace an earlier decision,
// matching how GitHub shows review status.
func (c *Card) setReview(reviewer Person, state, url string) {
	for i := range c.Reviews {
		if !strings.EqualFold(c.Reviews[i].Reviewer.Login, reviewer.Login) {
			continue
		}
		if state == "commented" && (c.Reviews[i].State == "approved" || c.Reviews[i].State == "changes_requested") {
			return
		}
		c.Reviews[i] = Review{Reviewer: reviewer, State: state, URL: url}
		return
	}
	c.Reviews = append(c.Reviews, Review{Reviewer: reviewer, State: state, URL: url})
}

// workflowName picks a readable name for a workflow run.
func workflowName(run *ghWorkflowRun, workflow *ghWorkflow) string {
	switch {
	case run.Name != "":
		return run.Name
	case workflow != nil && workflow.Name != "":
		return workflow.Name
	case workflow != nil && workflow.Path != "":
		return workflow.Path[strings.LastIndex(workflow.Path, "/")+1:]
	}
	return fmt.Sprintf("workflow %d", run.WorkflowID)
}

// applyRun records a workflow run on the card and returns the thread replies it causes:
// one for a newly failed run, and one when the card goes from failing to passing.
func (c *Card) applyRun(run *ghWorkflowRun, workflow *ghWorkflow) []Reply {
	if c.HeadSHA == "" {
		c.HeadSHA = run.HeadSHA
	}
	if run.HeadSHA != c.HeadSHA {
		return nil
	}
	check := Check{
		WorkflowID: run.WorkflowID, Name: workflowName(run, workflow), RunID: run.ID, Attempt: run.RunAttempt,
		Status: run.Status, Conclusion: run.Conclusion, URL: run.HTMLURL,
	}
	idx := slices.IndexFunc(c.Checks, func(ch Check) bool { return ch.WorkflowID == run.WorkflowID })
	var prev Check
	if idx >= 0 {
		prev = c.Checks[idx]
		if prev.RunID > run.ID || (prev.RunID == run.ID && prev.Attempt > run.RunAttempt) {
			return nil
		}
		if prev.RunID == run.ID && prev.Attempt == run.RunAttempt && prev.Status == "completed" && run.Status != "completed" {
			return nil
		}
		c.Checks[idx] = check
	} else {
		c.Checks = append(c.Checks, check)
		slices.SortFunc(c.Checks, func(a, b Check) int { return strings.Compare(a.Name, b.Name) })
	}

	var replies []Reply
	sha := shortSHA(run.HeadSHA)
	alreadyFailed := prev.RunID == run.ID && prev.Attempt == run.RunAttempt && isFailedConclusion(prev.Conclusion)
	if run.Status == "completed" && isFailedConclusion(run.Conclusion) && !alreadyFailed {
		replies = append(replies, Reply{
			Kind: replyCIFailed, Verb: "failed", Tone: "failure", URL: run.HTMLURL,
			Context: fmt.Sprintf("%s on `%s` (%s)", check.Name, sha, strings.ReplaceAll(run.Conclusion, "_", " ")),
		})
	}
	switch status := c.ciStatus(); status {
	case ciSuccess:
		if c.LastCIOutcome == ciFailure {
			replies = append(replies, Reply{
				Kind: replyCIPassed, Verb: "passing again", Tone: "success", URL: c.URL + "/checks",
				Context: fmt.Sprintf("every workflow passed on `%s`", sha),
			})
		}
		c.LastCIOutcome = status
	case ciFailure:
		c.LastCIOutcome = status
	}
	return replies
}

// shortSHA abbreviates a commit SHA the way GitHub does.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
