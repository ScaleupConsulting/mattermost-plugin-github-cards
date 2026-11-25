package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPRFromFixture(t *testing.T) {
	e := fixture(t, "pull_request/opened.payload.json")
	c := newCard(kindPR, e.Repository, 2)
	c.applyPR(e.PullRequest)

	assert.Equal(t, "Update the README with new information.", c.Title)
	assert.Equal(t, stateOpen, c.State)
	assert.Equal(t, "changes", c.Head)
	assert.Equal(t, "master", c.Base)
	assert.Equal(t, helloHead, c.HeadSHA)
	assert.Equal(t, []Label{{Name: "bug", Color: "d73a4a"}}, c.Labels)
	require.Len(t, c.RequestedReviewers, 1)
	assert.Equal(t, "octocat", c.RequestedReviewers[0].Login)
	assert.Equal(t, "Codertocat", c.Assignees[0].Login)
	assert.Equal(t, 1, c.Additions)
	assert.Equal(t, 1, c.ChangedFiles)
}

func TestPRStates(t *testing.T) {
	assert.Equal(t, stateDraft, prState(fixture(t, "pull_request/converted_to_draft.payload.json").PullRequest))
	assert.Equal(t, stateClosed, prState(fixture(t, "pull_request/closed.payload.json").PullRequest))
	assert.Equal(t, stateMerged, prState(fixture(t, "pull_request/closed.payload.json", "pull_request.merged", true).PullRequest))
	assert.Equal(t, stateOpen, prState(fixture(t, "pull_request/reopened.payload.json").PullRequest))
}

func TestOlderSnapshotDoesNotUndoNewer(t *testing.T) {
	closed := fixture(t, "pull_request/closed.payload.json")
	labeled := fixture(t, "pull_request/labeled.payload.json", "pull_request.labels", []any{})
	c := newCard(kindPR, closed.Repository, 2)
	c.applyPR(closed.PullRequest)
	c.applyPR(labeled.PullRequest) // delivered late, taken earlier
	assert.Equal(t, stateClosed, c.State)
	assert.Len(t, c.Labels, 1)
}

func TestReviewStateReduction(t *testing.T) {
	c := &Card{}
	alice := Person{Login: "alice"}
	c.setReview(alice, "changes_requested", "u1")
	c.setReview(alice, "commented", "u2")
	assert.Equal(t, "changes_requested", c.Reviews[0].State, "a comment keeps the earlier decision")
	c.setReview(alice, "approved", "u3")
	assert.Equal(t, "approved", c.Reviews[0].State)
	c.setReview(Person{Login: "bob"}, "commented", "u4")
	require.Len(t, c.Reviews, 2)
	assert.Equal(t, "commented", c.Reviews[1].State)
}

func TestReviewWithoutSummaryIsQuiet(t *testing.T) {
	// GitHub's own example: a comment-only review with a null body wraps its line comments.
	e := fixture(t, "pull_request_review/submitted.payload.json")
	fx := reduce(eventPullRequestReview, e, nil, 2, false)
	assert.Empty(t, fx.replies)
	require.Len(t, fx.card.Reviews, 1)
	assert.Equal(t, "commented", fx.card.Reviews[0].State)

	e = fixture(t, "pull_request_review/dismissed.payload.json")
	fx = reduce(eventPullRequestReview, e, fx.card, 2, false)
	assert.Equal(t, "dismissed", fx.card.Reviews[0].State)
	require.Len(t, fx.replies, 1)
	assert.Equal(t, "dismissed the review by Codertocat", fx.replies[0].Verb)
}

func TestCIReduction(t *testing.T) {
	c := &Card{HeadSHA: helloHead, URL: "https://github.com/o/r/pull/1"}
	runOf := func(id, wf int64, status, conclusion string) *ghWorkflowRun {
		return &ghWorkflowRun{ID: id, WorkflowID: wf, Name: map[int64]string{1: "CI", 2: "Lint"}[wf], HeadSHA: helloHead, Status: status, Conclusion: conclusion, RunAttempt: 1}
	}
	assert.Empty(t, c.applyRun(runOf(10, 1, "queued", ""), nil))
	assert.Equal(t, ciPending, c.ciStatus())
	assert.Empty(t, c.applyRun(runOf(11, 2, "completed", "success"), nil))
	assert.Equal(t, ciPending, c.ciStatus())

	replies := c.applyRun(runOf(10, 1, "completed", "timed_out"), nil)
	require.Len(t, replies, 1)
	assert.Equal(t, replyCIFailed, replies[0].Kind)
	assert.Equal(t, "CI on `ec26c3e` (timed out)", replies[0].Context)
	assert.Equal(t, ciFailure, c.ciStatus())

	assert.Empty(t, c.applyRun(runOf(10, 1, "in_progress", ""), nil), "a stale status for a finished run is ignored")
	assert.Empty(t, c.applyRun(runOf(9, 1, "completed", "success"), nil), "an older run is ignored")
	assert.Equal(t, ciFailure, c.ciStatus())

	assert.Empty(t, c.applyRun(runOf(12, 1, "in_progress", ""), nil), "a rerun starting is not news")
	replies = c.applyRun(runOf(12, 1, "completed", "success"), nil)
	require.Len(t, replies, 1)
	assert.Equal(t, replyCIPassed, replies[0].Kind)

	assert.Empty(t, c.applyRun(runOf(13, 2, "completed", "success"), nil), "passing while already passing is quiet")
	assert.Empty(t, c.applyRun(&ghWorkflowRun{ID: 99, WorkflowID: 1, HeadSHA: "other", Status: "completed", Conclusion: "failure"}, nil), "another commit is ignored")
	assert.Equal(t, ciSuccess, c.ciStatus())
}

func TestCIFlipAcrossNewCommit(t *testing.T) {
	c := &Card{HeadSHA: "aaa"}
	c.applyRun(&ghWorkflowRun{ID: 1, WorkflowID: 1, Name: "CI", HeadSHA: "aaa", Status: "completed", Conclusion: "failure"}, nil)
	pr := &ghPullRequest{Head: ghRef{SHA: "bbb"}, UpdatedAt: "2026-01-01T00:00:00Z"}
	c.applyPR(pr)
	assert.Empty(t, c.Checks, "a new head commit clears the checks")
	replies := c.applyRun(&ghWorkflowRun{ID: 2, WorkflowID: 1, Name: "CI", HeadSHA: "bbb", Status: "completed", Conclusion: "success"}, nil)
	require.Len(t, replies, 1, "fixing CI on a new commit still reports the flip")
	assert.Equal(t, replyCIPassed, replies[0].Kind)
}

func TestClosingRefs(t *testing.T) {
	body := "Fixes #12, closes Codertocat/Hello-World#13 and resolves https://github.com/Codertocat/Hello-World/issues/14.\n" +
		"Refs #15. Fixes other/repo#16. fixed: #12"
	assert.Equal(t, []int{12, 13, 14}, closingRefs(body, "codertocat/hello-world"))
	assert.Empty(t, closingRefs("", "a/b"))
}
