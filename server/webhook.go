package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	// maxPayloadBytes is GitHub's own cap on a webhook payload.
	maxPayloadBytes = 25 << 20
	// deliveryTTLSeconds keeps each delivery ID long enough to catch GitHub retries and manual redeliveries.
	deliveryTTLSeconds = 7 * 24 * 60 * 60
	webhookPath        = "/webhook"
)

// verifySignature checks a GitHub X-Hub-Signature-256 header against the body in constant time.
func verifySignature(secret string, body []byte, header string) bool {
	hexSig, ok := strings.CutPrefix(header, "sha256=")
	if !ok || secret == "" {
		return false
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// payloadOwner reads which account a payload belongs to, which picks the secret to verify it with.
// The body is untrusted here; nothing else is read from it until the signature checks out.
func payloadOwner(body []byte) string {
	var p struct {
		Repository *struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
		Organization *struct {
			Login string `json:"login"`
		} `json:"organization"`
	}
	if json.Unmarshal(body, &p) != nil {
		return ""
	}
	if p.Repository != nil && p.Repository.Owner.Login != "" {
		return strings.ToLower(p.Repository.Owner.Login)
	}
	if p.Organization != nil {
		return strings.ToLower(p.Organization.Login)
	}
	return ""
}

// ServeHTTP handles GitHub webhook deliveries at /plugins/<id>/webhook.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != webhookPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxPayloadBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	conf := p.getConfiguration()
	owner := payloadOwner(body)
	if !verifySignature(conf.secrets[owner], body, r.Header.Get("X-Hub-Signature-256")) {
		p.API.LogWarn("Rejected a webhook delivery with a missing or bad signature", "owner", owner, "remote", r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")
	if event == "" || delivery == "" || len(delivery) > 64 {
		http.Error(w, "missing X-GitHub-Event or X-GitHub-Delivery", http.StatusBadRequest)
		return
	}
	if event == "ping" {
		_, _ = io.WriteString(w, "pong\n")
		return
	}
	if !isSupportedEvent(event) {
		p.API.LogDebug("Ignored GitHub event", "event", event, "delivery", delivery)
		_, _ = io.WriteString(w, "ignored\n")
		return
	}

	var e ghEvent
	if err := json.Unmarshal(body, &e); err != nil || e.Repository == nil {
		http.Error(w, "payload is not a repository event", http.StatusBadRequest)
		return
	}
	routes := conf.matchRoutes(e.Repository.FullName)
	if len(routes) == 0 {
		p.API.LogDebug("No route for repository", "repo", e.Repository.FullName, "event", event)
		_, _ = io.WriteString(w, "no route\n")
		return
	}

	claimKey := "delivery_" + delivery
	claimed, appErr := p.API.KVSetWithOptions(claimKey, []byte{1}, kvSetIfAbsent(deliveryTTLSeconds))
	if appErr != nil {
		p.API.LogError("Could not record webhook delivery", "delivery", delivery, "error", appErr.Error())
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !claimed {
		_, _ = io.WriteString(w, "duplicate\n")
		return
	}

	proc := &processor{b: &mmBackend{p: p}, botID: p.botUserID}
	if err := proc.process(event, &e, routes); err != nil {
		// Forget the delivery so GitHub's redeliver button can retry it.
		_ = p.API.KVDelete(claimKey)
		p.API.LogError("Could not process webhook delivery", "event", event, "delivery", delivery, "error", err.Error())
		http.Error(w, "processing error", http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, "ok\n")
}
