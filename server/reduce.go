package main

import (
	"fmt"
	"strconv"
	"strings"
)

// cardNumbers returns the pull request or issue numbers an event concerns. A workflow run can
// concern several pull requests; a push concerns none.
func cardNumbers(event string, e *ghEvent) []int {
	switch event {
	case eventPullRequest, eventPullRequestReview, eventPullRequestReviewComment:
		if e.PullRequest != nil {
			return []int{e.PullRequest.Number}
		}
	case eventIssueComment, eventIssues:
		if e.Issue != nil {
			return []int{e.Issue.Number}
		}
	case eventWorkflowRun:
		if e.WorkflowRun == nil {
			return nil
		}
		var out []int
		for _, pr := range e.WorkflowRun.PullRequests {
			out = append(out, pr.Number)
		}
		return out
	}
	return nil
}

// reduce applies one event to the card for number, which is nil when no card exists yet.
// It is pure: the caller persists the card and posts the replies.
func reduce(event string, e *ghEvent, card *Card, number int, includeBots bool) effects {
	var fx effects
	if card == nil {
		if event == eventWorkflowRun {
			// A CI run alone says too little to draw a card; the next pull request event will.
			return fx
		}
		kind := kindPR
		if e.Issue != nil && e.Issue.PullRequest == nil {
			kind = kindIssue
		}
		card = newCard(kind, e.Repository, number)
	}
	fx.card = card
	actor := e.Sender.person()
	quiet := actor.IsBot && !includeBots

	switch event {
	case eventPullRequest:
		card.applyPR(e.PullRequest)
		fx.replies = prStateReplies(e, card)
	case eventPullRequestReview:
		card.applyPR(e.PullRequest)
		reduceReview(e, card, quiet, &fx)
	case eventPullRequestReviewComment:
		card.applyPR(e.PullRequest)
		c := e.Comment
		verb := "commented on"
		if c.InReplyTo != 0 {
			verb = "replied on"
		}
		where := "`" + c.Path + "`"
		if c.Line != nil {
			where += " line " + strconv.Itoa(*c.Line)
		}
		reduceComment(e, "r", Reply{
			Kind: replyReviewComment, Actor: c.User.person(), Verb: verb, Tone: "commented",
			Context: where, Body: c.Body, URL: c.HTMLURL,
		}, quiet, &fx)
	case eventIssueComment:
		card.applyIssue(e.Issue)
		c := e.Comment
		reduceComment(e, "c", Reply{
			Kind: replyComment, Actor: c.User.person(), Verb: "commented", Tone: "commented", Body: c.Body, URL: c.HTMLURL,
		}, quiet, &fx)
	case eventIssues:
		card.applyIssue(e.Issue)
		fx.replies = issueStateReplies(e, card)
	case eventWorkflowRun:
		fx.replies = card.applyRun(e.WorkflowRun, e.Workflow)
	}

	if card.Kind == kindPR && event != eventWorkflowRun {
		fx.link = &LinkedPR{Number: card.Number, Title: card.Title, State: card.State, URL: card.URL}
		fx.linkIssues = card.ClosesIssues
	}
	return fx
}

// prStateReplies describes a pull request state transition in one thread reply.
func prStateReplies(e *ghEvent, card *Card) []Reply {
	pr := e.PullRequest
	r := Reply{Kind: replyState, Actor: e.Sender.person(), URL: card.URL}
	switch e.Action {
	case "closed":
		if card.State != stateMerged {
			r.Verb, r.Tone = "closed this pull request without merging", stateClosed
			break
		}
		if pr.MergedBy != nil {
			r.Actor = pr.MergedBy.person()
		}
		r.Verb, r.Tone = "merged", stateMerged
		r.Context = fmt.Sprintf("`%s` into `%s`", card.Head, card.Base)
		if pr.MergeCommitSHA != "" {
			r.Context += fmt.Sprintf(" as [`%s`](%s/commit/%s)", shortSHA(pr.MergeCommitSHA), card.RepoURL, pr.MergeCommitSHA)
		}
	case "reopened":
		r.Verb, r.Tone = "reopened this pull request", stateOpen
	case "ready_for_review":
		r.Verb, r.Tone = "marked this pull request ready for review", stateOpen
	case "converted_to_draft":
		r.Verb, r.Tone = "converted this pull request to a draft", stateDraft
	default:
		return nil
	}
	return []Reply{r}
}

// issueStateReplies describes an issue being closed or reopened.
func issueStateReplies(e *ghEvent, card *Card) []Reply {
	r := Reply{Kind: replyState, Actor: e.Sender.person(), URL: card.URL}
	switch e.Action {
	case "closed":
		r.Tone = card.State
		r.Verb = "closed this issue as completed"
		if card.State == stateNotPlanned {
			r.Verb = "closed this issue as not planned"
		}
	case "reopened":
		r.Verb, r.Tone = "reopened this issue", stateOpen
	default:
		return nil
	}
	return []Reply{r}
}

// reduceReview records a review decision and adds, edits or announces its thread reply.
func reduceReview(e *ghEvent, card *Card, quiet bool, fx *effects) {
	rv := e.Review
	reviewer := rv.User.person()
	state := strings.ToLower(rv.State)
	switch e.Action {
	case "submitted":
		card.setReview(reviewer, state, rv.HTMLURL)
	case "dismissed":
		card.setReview(reviewer, "dismissed", rv.HTMLURL)
		if !quiet {
			fx.replies = append(fx.replies, Reply{
				Kind: replyReview, Actor: e.Sender.person(), Verb: "dismissed the review by " + escapeMarkdown(reviewer.Login),
				Tone: "dismissed", URL: rv.HTMLURL,
			})
		}
		return
	}
	verb, tone := "reviewed", "commented"
	switch state {
	case "approved":
		verb, tone = "approved", "approved"
	case "changes_requested":
		verb, tone = "requested changes", "changes_requested"
	}
	r := Reply{
		Kind: replyReview, Actor: reviewer, Verb: verb, Tone: tone, Body: rv.Body, URL: rv.HTMLURL,
		CommentKey: "v" + strconv.FormatInt(rv.ID, 10),
	}
	switch {
	case quiet:
	case e.Action == "edited":
		fx.edit = &r
	case e.Action == "submitted" && (tone != "commented" || strings.TrimSpace(rv.Body) != ""):
		// A comment-only review with no summary is just a wrapper for its line comments.
		fx.replies = append(fx.replies, r)
	}
}

// reduceComment turns a comment's created, edited or deleted action into the matching effect.
func reduceComment(e *ghEvent, prefix string, r Reply, quiet bool, fx *effects) {
	r.CommentKey = prefix + strconv.FormatInt(e.Comment.ID, 10)
	switch {
	case e.Action == "deleted":
		fx.deleteKey = r.CommentKey
	case quiet:
	case e.Action == "created":
		fx.replies = append(fx.replies, r)
	case e.Action == "edited":
		fx.edit = &r
	}
}

// pushReply renders a push to the repository's default branch, or returns nil for any other push.
func pushReply(e *ghEvent) *Reply {
	if e.Repository == nil || e.Deleted || len(e.Commits) == 0 || e.Ref != "refs/heads/"+e.Repository.DefaultBranch {
		return nil
	}
	n := len(e.Commits)
	noun := "commits"
	if n == 1 {
		noun = "commit"
	}
	verb := fmt.Sprintf("pushed %d %s to `%s`", n, noun, e.Repository.DefaultBranch)
	if e.Forced {
		verb = "force-" + verb
	}
	r := &Reply{
		Kind: replyPush, Actor: e.Sender.person(), Verb: verb, Tone: "push", URL: e.Compare,
		Repo: e.Repository.FullName, RepoURL: e.Repository.HTMLURL,
	}
	for _, c := range e.Commits {
		msg, _, _ := strings.Cut(c.Message, "\n")
		author := c.Author.Username
		if author == "" {
			author = c.Author.Name
		}
		r.Commits = append(r.Commits, PushCommit{SHA: c.ID, Message: msg, URL: c.URL, Author: author})
	}
	return r
}

// linkPR records a pull request on an issue card. It reports whether the card changed.
func (c *Card) linkPR(pr LinkedPR) bool {
	for i := range c.LinkedPRs {
		if c.LinkedPRs[i].Number == pr.Number {
			if c.LinkedPRs[i] == pr {
				return false
			}
			c.LinkedPRs[i] = pr
			return true
		}
	}
	c.LinkedPRs = append(c.LinkedPRs, pr)
	return true
}
