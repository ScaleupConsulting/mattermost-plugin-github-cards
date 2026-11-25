package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
)

// The payloads under testdata/github are GitHub's own examples from octokit/webhooks (MIT).
// Tests patch individual fields to build a consistent lifecycle out of them.

// rawFixture loads a payload as a generic map so a test can patch it.
func rawFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "github", name))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// set assigns a value at a dotted path inside a payload map, creating objects as needed.
func set(m map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}

// toEvent re-encodes a patched payload and decodes it the way the webhook does.
func toEvent(t *testing.T, m map[string]any) (*ghEvent, []byte) {
	t.Helper()
	body, err := json.Marshal(m)
	require.NoError(t, err)
	var e ghEvent
	require.NoError(t, json.Unmarshal(body, &e))
	return &e, body
}

// fixture loads a payload, applies patches as path/value pairs, and decodes it.
func fixture(t *testing.T, name string, patches ...any) *ghEvent {
	t.Helper()
	m := rawFixture(t, name)
	for i := 0; i < len(patches); i += 2 {
		set(m, patches[i].(string), patches[i+1])
	}
	e, _ := toEvent(t, m)
	return e
}

// helloRepo is the repository object of the Codertocat/Hello-World fixtures, for re-homing others.
func helloRepo(t *testing.T) map[string]any {
	return rawFixture(t, "pull_request/opened.payload.json")["repository"].(map[string]any)
}

// fakeBackend keeps KV and posts in memory and counts edits, standing in for Mattermost.
type fakeBackend struct {
	kv      map[string]string
	posts   map[string]*model.Post
	order   []string
	edits   map[string]int
	deleted map[string]bool
	nextID  int
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{kv: map[string]string{}, posts: map[string]*model.Post{}, edits: map[string]int{}, deleted: map[string]bool{}}
}

func (f *fakeBackend) kvGet(key string, v any) (bool, error) {
	data, ok := f.kv[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal([]byte(data), v)
}

func (f *fakeBackend) kvSet(key string, v any) error {
	f.kv[key] = mustJSON(v)
	return nil
}

func (f *fakeBackend) kvDelete(key string) error {
	delete(f.kv, key)
	return nil
}

func (f *fakeBackend) createPost(post *model.Post) (*model.Post, error) {
	f.nextID++
	p := post.Clone()
	p.Id = fmt.Sprintf("post%03d", f.nextID)
	// Round-trip props through JSON as the server does, so tests read what the webapp reads.
	var props model.StringInterface
	_ = json.Unmarshal([]byte(mustJSON(p.GetProps())), &props)
	p.SetProps(props)
	f.posts[p.Id] = p
	f.order = append(f.order, p.Id)
	return p, nil
}

func (f *fakeBackend) updatePost(post *model.Post) (*model.Post, error) {
	old, ok := f.posts[post.Id]
	if !ok || f.deleted[post.Id] {
		return nil, errPostGone
	}
	p := post.Clone()
	p.RootId = old.RootId
	f.posts[p.Id] = p
	f.edits[p.Id]++
	return p, nil
}

func (f *fakeBackend) deletePost(postID string) error {
	f.deleted[postID] = true
	return nil
}

func (f *fakeBackend) lock(string) (func(), error) { return func() {}, nil }

// live returns posts not deleted, in creation order.
func (f *fakeBackend) live() []*model.Post {
	var out []*model.Post
	for _, id := range f.order {
		if !f.deleted[id] {
			out = append(out, f.posts[id])
		}
	}
	return out
}

// thread returns the live replies under a root post.
func (f *fakeBackend) thread(rootID string) []*model.Post {
	var out []*model.Post
	for _, p := range f.live() {
		if p.RootId == rootID {
			out = append(out, p)
		}
	}
	return out
}

// kvKeys lists KV keys with a prefix, sorted.
func (f *fakeBackend) kvKeys(prefix string) []string {
	var out []string
	for k := range f.kv {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
