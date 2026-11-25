package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCardMarkdown(t *testing.T) {
	e := fixture(t, "pull_request/opened.payload.json", "pull_request.body", "Fixes #1")
	c := newCard(kindPR, e.Repository, 2)
	c.applyPR(e.PullRequest)
	c.setReview(Person{Login: "hubot"}, "changes_requested", "")
	c.applyRun(&ghWorkflowRun{ID: 1, WorkflowID: 1, Name: "build", HeadSHA: helloHead, Status: "completed", Conclusion: "failure", HTMLURL: "https://ci/1"}, nil)

	got := cardMarkdown(c)
	want := strings.Join([]string{
		"#### [Codertocat/Hello-World#2](https://github.com/Codertocat/Hello-World/pull/2) Update the README with new information.",
		"**Open** pull request by Codertocat · `changes` → `master` · +1 −1 in 1 file",
		"Reviewers: :x: hubot · :hourglass_flowing_sand: octocat (requested)",
		"CI: :x: failing on `ec26c3e` · :x: [build](https://ci/1)",
		"Labels: `bug` · Assignees: Codertocat · Closes [#1](https://github.com/Codertocat/Hello-World/issues/1)",
	}, "\n")
	assert.Equal(t, want, got)
}

func TestIssueCardMarkdown(t *testing.T) {
	e := fixture(t, "issue_comment/created.1.payload.json")
	c := newCard(kindIssue, e.Repository, e.Issue.Number)
	c.applyIssue(e.Issue)
	c.linkPR(LinkedPR{Number: 2, State: stateMerged, URL: "https://github.com/Codertocat/Hello-World/pull/2"})
	got := cardMarkdown(c)
	assert.Contains(t, got, "**Open** issue by Codertocat · 2 comments")
	assert.Contains(t, got, "Linked PR [#2](https://github.com/Codertocat/Hello-World/pull/2) (merged)")
	assert.NotContains(t, got, "Reviewers")
}

func TestTitleIsEscaped(t *testing.T) {
	c := &Card{Kind: kindIssue, Repo: "o/r", Number: 1, Title: "Fix [link](javascript:x) *now* @all", State: stateOpen, Author: Person{Login: "a_b"}}
	got := cardMarkdown(c)
	assert.Contains(t, got, `Fix \[link\](javascript:x) \*now\* @all`)
	assert.Contains(t, got, `by a\_b`)
}

func TestReplyMarkdown(t *testing.T) {
	r := &Reply{Kind: replyReview, Actor: Person{Login: "octocat"}, Verb: "approved", Tone: "approved", Body: "Ship it @here", URL: "https://g/r"}
	assert.Equal(t, ":white_check_mark: **octocat** approved · [view on GitHub](https://g/r)\n\nShip it @\u2060here", replyMarkdown(r))

	long := &Reply{Kind: replyComment, Actor: Person{Login: "a"}, Verb: "commented", Tone: "commented", Body: strings.Repeat("é", 2000), URL: "https://g/c"}
	md := replyMarkdown(long)
	assert.Contains(t, md, "… [read more](https://g/c)")
	assert.Less(t, len(md), maxFallbackBody+200)

	ci := &Reply{Kind: replyCIFailed, Verb: "failed", Tone: "failure", Context: "build on `abc1234` (failure)", URL: "https://ci"}
	assert.Equal(t, ":x: CI failed: build on `abc1234` (failure) · [view on GitHub](https://ci)", replyMarkdown(ci))
}

func TestQuietMentions(t *testing.T) {
	assert.Equal(t, "hi @\u2060bob and @\u2060channel, mail a@b.c", quietMentions("hi @bob and @channel, mail a@b.c"))
}

func TestReplyPostCapsCommits(t *testing.T) {
	r := &Reply{Kind: replyPush, Actor: Person{Login: "a"}, Verb: "pushed 12 commits to `main`", Tone: "push", Repo: "o/r"}
	for range 12 {
		r.Commits = append(r.Commits, PushCommit{SHA: "0123456789", Message: "m"})
	}
	p := replyPost("bot", "ch", "", r)
	props := p.GetProp(propEvent).(map[string]any)
	assert.Len(t, props["commits"], maxPushCommits)
	assert.Equal(t, 12, props["total_commits"])
	assert.Contains(t, p.Message, "- and 2 more")
}
