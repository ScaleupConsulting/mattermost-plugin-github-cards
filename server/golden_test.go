package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// goldenDir holds post props the webapp tests render, so both sides agree on the shape.
var goldenDir = filepath.Join("..", "webapp", "src", "testdata")

// checkGolden compares v with a golden file, or rewrites it when UPDATE_GOLDEN=1.
func checkGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')
	path := filepath.Join(goldenDir, name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(goldenDir, 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with UPDATE_GOLDEN=1 to create %s", path)
	require.JSONEq(t, string(want), string(got))
}

func TestWebappGolden(t *testing.T) {
	f := newFakeBackend()
	run(t, f, eventPullRequest, fixture(t, "pull_request/opened.payload.json", "pull_request.body", "Fixes #1"))
	run(t, f, eventPullRequestReview, fixture(t, "pull_request_review/submitted.payload.json",
		"review.state", "changes_requested", "review.body", "Please add a test for the **new** section.",
		"pull_request.updated_at", "2019-05-15T15:20:40Z", "pull_request.body", "Fixes #1"))
	run(t, f, eventWorkflowRun, workflowRun(t, 101, "completed", "success", "Lint"))
	run(t, f, eventWorkflowRun, workflowRun(t, 100, "completed", "failure", "CI"))

	root := f.live()[0]
	checkGolden(t, "card_pr.json", root.GetProps())
	thread := f.thread(root.Id)
	require.Len(t, thread, 2)
	checkGolden(t, "event_review.json", thread[0].GetProps())
	checkGolden(t, "event_ci_failed.json", thread[1].GetProps())

	run(t, f, eventPush, fixture(t, "push/with-new-branch.payload.json"))
	posts := f.live()
	checkGolden(t, "event_push.json", posts[len(posts)-1].GetProps())
}
