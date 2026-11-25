package main

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"
)

// errPostGone is returned by backend.updatePost when the post was deleted in Mattermost.
var errPostGone = errors.New("post no longer exists")

// backend is the slice of the Mattermost API the processor needs, so tests can supply a fake.
type backend interface {
	kvGet(key string, v any) (bool, error)
	kvSet(key string, v any) error
	kvDelete(key string) error
	createPost(post *model.Post) (*model.Post, error)
	updatePost(post *model.Post) (*model.Post, error)
	deletePost(postID string) error
	// lock serializes work on one key across the cluster and returns its unlock function.
	lock(key string) (func(), error)
}

// cardRecord is what KV holds for one card: its root post and its state.
type cardRecord struct {
	PostID string `json:"post_id"`
	Card   *Card  `json:"card"`
}

// processor turns verified webhook events into posts.
type processor struct {
	b     backend
	botID string
}

// cardKey names a card's KV record. Repository IDs survive renames, and issues and pull requests
// share one number space, so channel, repository and number identify a card.
func cardKey(channelID string, repoID int64, number int) string {
	return fmt.Sprintf("card_%s_%d_%d", channelID, repoID, number)
}

// replyKey names the KV record mapping a GitHub comment or review to its thread reply.
func replyKey(channelID, commentKey string) string {
	return fmt.Sprintf("reply_%s_%s", channelID, commentKey)
}

// process delivers one event to every route that wants it.
func (h *processor) process(event string, e *ghEvent, routes []route) error {
	for _, r := range routes {
		if !r.Events[event] {
			continue
		}
		if event == eventPush {
			if reply := pushReply(e); reply != nil {
				if _, err := h.b.createPost(replyPost(h.botID, r.channelID, "", reply)); err != nil {
					return errors.Wrap(err, "post push")
				}
			}
			continue
		}
		for _, n := range cardNumbers(event, e) {
			if err := h.processCard(event, e, r, n); err != nil {
				return errors.Wrapf(err, "%s#%d", e.Repository.FullName, n)
			}
		}
	}
	return nil
}

// processCard applies one event to one card in one channel, under that card's lock.
func (h *processor) processCard(event string, e *ghEvent, r route, number int) error {
	key := cardKey(r.channelID, e.Repository.ID, number)
	unlock, err := h.b.lock(key)
	if err != nil {
		return err
	}
	var rec cardRecord
	found, err := h.b.kvGet(key, &rec)
	if err != nil {
		unlock()
		return errors.Wrap(err, "load card")
	}
	before := ""
	if found {
		before = mustJSON(rec.Card)
	} else {
		rec.Card = nil
	}
	fx := reduce(event, e, rec.Card, number, r.IncludeBots)
	if fx.card == nil {
		unlock()
		return nil
	}
	rec.Card = fx.card
	if err = h.saveCard(key, r.channelID, &rec, before); err != nil {
		unlock()
		return err
	}
	err = h.applyReplies(r.channelID, rec.PostID, &fx)
	unlock()
	if err != nil {
		return err
	}

	if fx.link == nil {
		return nil
	}
	for _, n := range fx.linkIssues {
		if n == number {
			continue
		}
		if err := h.linkIssue(cardKey(r.channelID, e.Repository.ID, n), r.channelID, *fx.link); err != nil {
			return errors.Wrapf(err, "link issue #%d", n)
		}
	}
	return nil
}

// saveCard creates or updates the root post when the card changed, then stores the record.
// A root post someone deleted is drawn again rather than failing every later event.
func (h *processor) saveCard(key, channelID string, rec *cardRecord, before string) error {
	if rec.PostID != "" && mustJSON(rec.Card) == before {
		return nil
	}
	post := cardPost(h.botID, channelID, rec.Card)
	if rec.PostID != "" {
		post.Id = rec.PostID
		_, err := h.b.updatePost(post)
		if err == nil {
			return errors.Wrap(h.b.kvSet(key, rec), "save card")
		}
		if !errors.Is(err, errPostGone) {
			return errors.Wrap(err, "update card post")
		}
		post.Id = ""
	}
	created, err := h.b.createPost(post)
	if err != nil {
		return errors.Wrap(err, "create card post")
	}
	rec.PostID = created.Id
	return errors.Wrap(h.b.kvSet(key, rec), "save card")
}

// applyReplies posts new thread replies and applies a comment edit or delete.
func (h *processor) applyReplies(channelID, rootID string, fx *effects) error {
	for i := range fx.replies {
		r := &fx.replies[i]
		post, err := h.b.createPost(replyPost(h.botID, channelID, rootID, r))
		if err != nil {
			return errors.Wrap(err, "post reply")
		}
		if r.CommentKey != "" {
			if err := h.b.kvSet(replyKey(channelID, r.CommentKey), post.Id); err != nil {
				return errors.Wrap(err, "save reply mapping")
			}
		}
	}
	if fx.edit != nil {
		var postID string
		found, err := h.b.kvGet(replyKey(channelID, fx.edit.CommentKey), &postID)
		if err != nil {
			return errors.Wrap(err, "load reply mapping")
		}
		if found {
			post := replyPost(h.botID, channelID, rootID, fx.edit)
			post.Id = postID
			if _, err := h.b.updatePost(post); err != nil && !errors.Is(err, errPostGone) {
				return errors.Wrap(err, "edit reply")
			}
		}
	}
	if fx.deleteKey != "" {
		key := replyKey(channelID, fx.deleteKey)
		var postID string
		found, err := h.b.kvGet(key, &postID)
		if err != nil {
			return errors.Wrap(err, "load reply mapping")
		}
		if found {
			if err := h.b.deletePost(postID); err != nil {
				return errors.Wrap(err, "delete reply")
			}
			if err := h.b.kvDelete(key); err != nil {
				return errors.Wrap(err, "delete reply mapping")
			}
		}
	}
	return nil
}

// linkIssue shows a pull request on an existing issue card that it closes.
func (h *processor) linkIssue(key, channelID string, pr LinkedPR) error {
	unlock, err := h.b.lock(key)
	if err != nil {
		return err
	}
	defer unlock()
	var rec cardRecord
	found, err := h.b.kvGet(key, &rec)
	if err != nil || !found || rec.Card == nil || rec.Card.Kind != kindIssue {
		return err
	}
	before := mustJSON(rec.Card)
	if !rec.Card.linkPR(pr) {
		return nil
	}
	return h.saveCard(key, channelID, &rec, before)
}

// mustJSON serializes a card for change detection.
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
