package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testSecret = "fake-test-secret-not-real-0001"

// sign computes the X-Hub-Signature-256 header GitHub would send.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "github", "pull_request", "opened.payload.json"))
	require.NoError(t, err)
	good := sign(testSecret, body)

	assert.True(t, verifySignature(testSecret, body, good))
	assert.False(t, verifySignature("fake-other-secret-not-real-02", body, good), "wrong secret")
	assert.False(t, verifySignature(testSecret, append(bytes.Clone(body), ' '), good), "tampered body")
	assert.False(t, verifySignature(testSecret, body, ""), "unsigned")
	assert.False(t, verifySignature(testSecret, body, good[len("sha256="):]), "missing sha256= prefix")
	assert.False(t, verifySignature(testSecret, body, "sha1="+good[len("sha256="):]), "SHA-1 header")
	assert.False(t, verifySignature(testSecret, body, "sha256=zz"), "not hex")
	assert.False(t, verifySignature(testSecret, body, good[:20]), "truncated")
	assert.False(t, verifySignature("", body, sign("", body)), "no secret configured")
}

func TestPayloadOwner(t *testing.T) {
	assert.Equal(t, "codertocat", payloadOwner(mustRead(t, "pull_request/opened.payload.json")))
	assert.Equal(t, "octocoders", payloadOwner(mustRead(t, "ping/payload.json")))
	assert.Equal(t, "", payloadOwner([]byte("not json")))
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "github", name))
	require.NoError(t, err)
	return data
}

// newTestPlugin returns a plugin with one secret and route and a permissive log mock.
func newTestPlugin(t *testing.T) *Plugin {
	api := &plugintest.API{}
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	p := &Plugin{}
	p.SetAPI(api)
	c := &configuration{WebhookSecrets: "Codertocat = " + testSecret, Routes: "Codertocat -> team/github"}
	require.NoError(t, parseConfiguration(c))
	p.setConfiguration(c)
	return p
}

func TestServeHTTPRejectsBadRequests(t *testing.T) {
	p := newTestPlugin(t)
	body := mustRead(t, "pull_request/opened.payload.json")
	post := func(sig, event string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		if sig != "" {
			r.Header.Set("X-Hub-Signature-256", sig)
		}
		r.Header.Set("X-GitHub-Event", event)
		r.Header.Set("X-GitHub-Delivery", "00000000-0000-0000-0000-000000000001")
		w := httptest.NewRecorder()
		p.ServeHTTP(nil, w, r)
		return w
	}

	assert.Equal(t, http.StatusUnauthorized, post("", "pull_request", body).Code, "unsigned")
	assert.Equal(t, http.StatusUnauthorized, post(sign("fake-wrong-secret-not-real-3", body), "pull_request", body).Code, "wrong secret")
	assert.Equal(t, http.StatusUnauthorized, post(sign(testSecret, body), "pull_request", append(bytes.Clone(body), '\n')).Code, "tampered")

	other := mustRead(t, "ping/payload.json") // owned by Octocoders, which has no secret
	assert.Equal(t, http.StatusUnauthorized, post(sign(testSecret, other), "ping", other).Code, "unknown owner")

	w := post(sign(testSecret, body), "star", body)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ignored\n", w.Body.String())

	r := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w = httptest.NewRecorder()
	p.ServeHTTP(nil, w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
