package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"
)

const (
	botUsername    = "github-cards"
	botDisplayName = "GitHub"
	// lockTimeout bounds how long one delivery waits for another touching the same card.
	lockTimeout = 30 * time.Second
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	client    *pluginapi.Client
	botUserID string

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex
	configuration     *configuration
}

// OnActivate creates the bot, then resolves the routes so a bad channel name stops activation.
func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)
	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    botUsername,
		DisplayName: botDisplayName,
		Description: "Posts GitHub pull requests, issues and CI results as live cards.",
	}, pluginapi.ProfileImagePath("assets/bot.png"))
	if err != nil {
		return errors.Wrap(err, "ensure bot")
	}
	p.botUserID = botID
	return p.loadConfiguration()
}

// kvSetIfAbsent stores a key only when it does not exist yet, expiring it after ttl seconds.
func kvSetIfAbsent(ttl int64) model.PluginKVSetOptions {
	return model.PluginKVSetOptions{Atomic: true, OldValue: nil, ExpireInSeconds: ttl}
}

// mmBackend is the production backend, over the plugin API.
type mmBackend struct {
	p *Plugin
}

func (m *mmBackend) kvGet(key string, v any) (bool, error) {
	data, appErr := m.p.API.KVGet(key)
	if appErr != nil {
		return false, appErr
	}
	if data == nil {
		return false, nil
	}
	return true, errors.Wrap(json.Unmarshal(data, v), key)
}

func (m *mmBackend) kvSet(key string, v any) error {
	if appErr := m.p.API.KVSet(key, []byte(mustJSON(v))); appErr != nil {
		return appErr
	}
	return nil
}

func (m *mmBackend) kvDelete(key string) error {
	if appErr := m.p.API.KVDelete(key); appErr != nil {
		return appErr
	}
	return nil
}

func (m *mmBackend) createPost(post *model.Post) (*model.Post, error) {
	created, appErr := m.p.API.CreatePost(post)
	if appErr != nil {
		return nil, appErr
	}
	return created, nil
}

func (m *mmBackend) updatePost(post *model.Post) (*model.Post, error) {
	updated, appErr := m.p.API.UpdatePost(post)
	if appErr != nil {
		// The server reports a deleted post as a bad request, so ask about the post itself.
		if old, getErr := m.p.API.GetPost(post.Id); getErr != nil || old.DeleteAt != 0 {
			return nil, errPostGone
		}
		return nil, appErr
	}
	return updated, nil
}

func (m *mmBackend) deletePost(postID string) error {
	if appErr := m.p.API.DeletePost(postID); appErr != nil && appErr.StatusCode != http.StatusNotFound {
		return appErr
	}
	return nil
}

func (m *mmBackend) lock(key string) (func(), error) {
	mutex, err := cluster.NewMutex(m.p.API, "lock_"+key)
	if err != nil {
		return nil, errors.Wrap(err, "create lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	if err := mutex.LockWithContext(ctx); err != nil {
		return nil, errors.Wrap(err, "wait for lock")
	}
	return mutex.Unlock, nil
}
