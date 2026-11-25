package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testChannel = "channel00000000000000000001"
	helloHead   = "ec26c3e57ca3a959ca5aad62de7213c562f8c821"
)

var testRoute = route{Owner: "codertocat", channelID: testChannel, Events: map[string]bool{
	eventPullRequest: true, eventPullRequestReview: true, eventPullRequestReviewComment: true,
	eventIssueComment: true, eventIssues: true, eventWorkflowRun: true, eventPush: true,
}}

// run feeds one event through a processor on the fake backend.
func run(t *testing.T, f *fakeBackend, event string, e *ghEvent) {
	t.Helper()
	h := &processor{b: f, botID: "bot"}
	require.NoError(t, h.process(event, e, []route{testRoute}))
}

// workflowRun re-homes the workflow_run fixture onto Codertocat/Hello-World PR #2.
func workflowRun(t *testing.T, id int64, status, conclusion, name string) *ghEvent {
	m := rawFixture(t, "workflow_run/completed.with-pull-requests.payload.json")
	m["repository"] = helloRepo(t)
	set(m, "workflow_run.id", id)
	set(m, "workflow_run.name", name)
	set(m, "workflow_run.workflow_id", map[string]int{"CI": 1, "Lint": 2}[name])
	set(m, "workflow_run.status", status)
	set(m, "workflow_run.conclusion", conclusion)
	set(m, "workflow_run.head_sha", helloHead)
	set(m, "workflow_run.html_url", "https://github.com/Codertocat/Hello-World/actions/runs/1")
	e, _ := toEvent(t, m)
	return e
}

// cardOf decodes the card stored in a root post's props.
func cardOf(t *testing.T, f *fakeBackend, key string) *Card {
	var rec cardRecord
	found, err := f.kvGet(key, &rec)
	require.NoError(t, err)
	require.True(t, found, key)
	return rec.Card
}

func TestPullRequestLifecycle(t *testing.T) {
	f := newFakeBackend()
	key := cardKey(testChannel, 186853002, 2)

	run(t, f, eventPullRequest, fixture(t, "pull_request/opened.payload.json"))
	require.Len(t, f.live(), 1)
	root := f.live()[0]
	assert.Equal(t, postTypeCard, root.Type)
	assert.Equal(t, "", root.RootId)

	run(t, f, eventPullRequestReview, fixture(t, "pull_request_review/submitted.payload.json",
		"review.state", "approved", "review.body", "Looks good. @channel @octocat",
		"review.user.login", "octocat", "pull_request.updated_at", "2019-05-15T15:20:40Z",
		"pull_request.requested_reviewers", []any{}))
	run(t, f, eventWorkflowRun, workflowRun(t, 100, "in_progress", "", "CI"))
	run(t, f, eventWorkflowRun, workflowRun(t, 101, "completed", "success", "Lint"))
	run(t, f, eventWorkflowRun, workflowRun(t, 100, "completed", "failure", "CI"))
	assert.Equal(t, ciFailure, cardOf(t, f, key).ciStatus())
	run(t, f, eventWorkflowRun, workflowRun(t, 100, "completed", "failure", "CI")) // GitHub resends
	run(t, f, eventWorkflowRun, workflowRun(t, 102, "completed", "success", "CI"))
	run(t, f, eventPullRequest, fixture(t, "pull_request/closed.payload.json",
		"pull_request.merged", true, "pull_request.merged_at", "2019-05-15T15:21:18Z",
		"pull_request.merged_by", map[string]any{"login": "octocat", "type": "User"}))

	// The card is the only root post and was edited in place every time it changed.
	var roots []*model.Post
	for _, p := range f.live() {
		if p.RootId == "" {
			roots = append(roots, p)
		}
	}
	require.Len(t, roots, 1)
	assert.Equal(t, root.Id, roots[0].Id)
	assert.GreaterOrEqual(t, f.edits[root.Id], 5)

	var kinds []string
	for _, p := range f.thread(root.Id) {
		kinds = append(kinds, p.GetProp(propEvent).(map[string]any)["kind"].(string))
	}
	assert.Equal(t, []string{replyReview, replyCIFailed, replyCIPassed, replyState}, kinds)

	merged := f.thread(root.Id)[3].Message
	assert.Contains(t, merged, "**octocat** merged `changes` into `master` as [`c4295bd`](https://github.com/Codertocat/Hello-World/commit/c4295bd74fb0f4fda03689c3df3f2803b658fd85)")
	review := f.thread(root.Id)[0].Message
	assert.Contains(t, review, "@⁠channel", "group mention is defused")
	assert.NotContains(t, review, "@channel")

	c := cardOf(t, f, key)
	assert.Equal(t, stateMerged, c.State)
	assert.Equal(t, ciSuccess, c.ciStatus())
	assert.Equal(t, []Review{{Reviewer: Person{Login: "octocat", AvatarURL: c.Reviews[0].Reviewer.AvatarURL, URL: c.Reviews[0].Reviewer.URL}, State: "approved", URL: c.Reviews[0].URL}}, c.Reviews)
	assert.Equal(t, "octocat", c.MergedBy.Login)
	assert.Equal(t, 1, c.ChangedFiles, "review payloads omit diff counts; the card keeps them")
	assert.Contains(t, roots[0].Message, "**Merged** pull request by Codertocat")
	assert.Equal(t, "merged", roots[0].GetProp(propCard).(map[string]any)["state"])
}

func TestCardCreatedOnFirstSight(t *testing.T) {
	f := newFakeBackend()
	// A review on a pull request opened before the plugin existed draws the card first.
	run(t, f, eventPullRequestReviewComment, fixture(t, "pull_request_review_comment/created.payload.json"))
	posts := f.live()
	require.Len(t, posts, 2)
	assert.Equal(t, postTypeCard, posts[0].Type)
	assert.Equal(t, posts[0].Id, posts[1].RootId)
	assert.Contains(t, posts[1].Message, "**Codertocat** commented on `README.md` line 265")

	// A CI run alone does not draw a card for an unknown pull request.
	f = newFakeBackend()
	run(t, f, eventWorkflowRun, workflowRun(t, 1, "completed", "failure", "CI"))
	assert.Empty(t, f.live())
}

func TestCommentEditAndDelete(t *testing.T) {
	f := newFakeBackend()
	run(t, f, eventIssues, fixture(t, "issues/opened.payload.json"))
	run(t, f, eventIssueComment, fixture(t, "issue_comment/created.payload.json"))
	root := f.live()[0]
	require.Len(t, f.thread(root.Id), 1)
	reply := f.thread(root.Id)[0]
	assert.Contains(t, reply.Message, "right away")

	run(t, f, eventIssueComment, fixture(t, "issue_comment/edited.payload.json"))
	require.Len(t, f.thread(root.Id), 1, "an edit changes the reply rather than adding one")
	assert.Equal(t, 1, f.edits[reply.Id])
	assert.Contains(t, f.posts[reply.Id].Message, "fixed today")

	run(t, f, eventIssueComment, fixture(t, "issue_comment/deleted.payload.json"))
	assert.Empty(t, f.thread(root.Id), "a deleted comment leaves no stub")
	assert.Empty(t, f.kvKeys("reply_"))
}

func TestBotCommentsAreLeftOutByDefault(t *testing.T) {
	bot := []any{"comment.user.login", "dependabot[bot]", "comment.user.type", "Bot", "sender.login", "dependabot[bot]", "sender.type", "Bot"}
	f := newFakeBackend()
	run(t, f, eventIssueComment, fixture(t, "issue_comment/created.payload.json", bot...))
	require.Len(t, f.live(), 1, "the card is drawn but the bot comment is not posted")

	f = newFakeBackend()
	h := &processor{b: f, botID: "bot"}
	r := testRoute
	r.IncludeBots = true
	require.NoError(t, h.process(eventIssueComment, fixture(t, "issue_comment/created.payload.json", bot...), []route{r}))
	assert.Len(t, f.live(), 2)
}

func TestIssueLifecycleAndLinkedPR(t *testing.T) {
	f := newFakeBackend()
	run(t, f, eventIssues, fixture(t, "issues/opened.payload.json"))
	run(t, f, eventPullRequest, fixture(t, "pull_request/opened.payload.json", "pull_request.body", "This fixes #1 properly."))
	issue := cardOf(t, f, cardKey(testChannel, 186853002, 1))
	require.Len(t, issue.LinkedPRs, 1)
	assert.Equal(t, LinkedPR{Number: 2, Title: "Update the README with new information.", State: stateOpen, URL: "https://github.com/Codertocat/Hello-World/pull/2"}, issue.LinkedPRs[0])
	assert.Equal(t, []int{1}, cardOf(t, f, cardKey(testChannel, 186853002, 2)).ClosesIssues)

	run(t, f, eventIssues, fixture(t, "issues/opened.payload.json", "action", "closed", "issue.state", "closed",
		"issue.state_reason", "not_planned", "issue.updated_at", "2019-05-15T15:30:00Z"))
	issue = cardOf(t, f, cardKey(testChannel, 186853002, 1))
	assert.Equal(t, stateNotPlanned, issue.State)
	root := f.live()[0]
	thread := f.thread(root.Id)
	require.Len(t, thread, 1)
	assert.Contains(t, thread[0].Message, "closed this issue as not planned")
}

func TestPushOnlyForDefaultBranch(t *testing.T) {
	f := newFakeBackend()
	run(t, f, eventPush, fixture(t, "push/payload.json")) // a tag deletion
	assert.Empty(t, f.live())
	run(t, f, eventPush, fixture(t, "push/with-new-branch.payload.json", "ref", "refs/heads/feature"))
	assert.Empty(t, f.live())
	run(t, f, eventPush, fixture(t, "push/with-new-branch.payload.json"))
	require.Len(t, f.live(), 1)
	p := f.live()[0]
	assert.Equal(t, "", p.RootId, "a push is a channel post, never a thread reply")
	assert.True(t, strings.HasPrefix(p.Message, ":arrow_up: **Codertocat** pushed 1 commit to `master` in [Codertocat/Hello-World]"), p.Message)
}

func TestRouteEventFilter(t *testing.T) {
	f := newFakeBackend()
	h := &processor{b: f, botID: "bot"}
	r := testRoute
	r.Events = map[string]bool{eventPush: true}
	require.NoError(t, h.process(eventPullRequest, fixture(t, "pull_request/opened.payload.json"), []route{r}))
	assert.Empty(t, f.live())
}

func TestDeletedCardIsRedrawn(t *testing.T) {
	f := newFakeBackend()
	run(t, f, eventPullRequest, fixture(t, "pull_request/opened.payload.json"))
	first := f.live()[0].Id
	f.deleted[first] = true
	run(t, f, eventPullRequest, fixture(t, "pull_request/labeled.payload.json"))
	require.Len(t, f.live(), 1)
	assert.NotEqual(t, first, f.live()[0].Id)
}
