package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

// Event names a route can subscribe to. Every other GitHub event is ignored.
const (
	eventPullRequest              = "pull_request"
	eventPullRequestReview        = "pull_request_review"
	eventPullRequestReviewComment = "pull_request_review_comment"
	eventIssueComment             = "issue_comment"
	eventIssues                   = "issues"
	eventWorkflowRun              = "workflow_run"
	eventPush                     = "push"
)

var supportedEvents = []string{
	eventPullRequest, eventPullRequestReview, eventPullRequestReviewComment,
	eventIssueComment, eventIssues, eventWorkflowRun, eventPush,
}

// configuration holds the System Console settings. Public fields are loaded from the server
// configuration; the private fields are derived from them by parseConfiguration.
// A configuration is never modified after setConfiguration, so readers share one pointer.
type configuration struct {
	WebhookSecrets string
	Routes         string

	secrets map[string]string // lowercase owner login -> secret
	routes  []route
}

// route sends one GitHub org or repository to one channel.
type route struct {
	Line        int
	Owner       string // lowercase
	Repo        string // lowercase, empty for the whole owner
	TeamName    string
	ChannelName string
	Events      map[string]bool
	IncludeBots bool

	channelID string // resolved at activation
}

// parseSecrets reads `owner = secret` lines. Blank lines and lines starting with # are skipped.
func parseSecrets(text string) (map[string]string, error) {
	secrets := map[string]string{}
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		owner, secret, ok := strings.Cut(line, "=")
		owner = strings.ToLower(strings.TrimSpace(owner))
		secret = strings.TrimSpace(secret)
		if !ok || owner == "" || strings.ContainsAny(owner, " /") {
			return nil, errors.Errorf("webhook secrets line %d: expected `org = secret`", i+1)
		}
		if len(secret) < 16 {
			return nil, errors.Errorf("webhook secrets line %d: the secret for %q must be at least 16 characters", i+1, owner)
		}
		if _, dup := secrets[owner]; dup {
			return nil, errors.Errorf("webhook secrets line %d: %q has a second secret", i+1, owner)
		}
		secrets[owner] = secret
	}
	return secrets, nil
}

// parseRoutes reads `org[/repo] -> team/channel [events=a,b] [bots=include]` lines.
func parseRoutes(text string) ([]route, error) {
	var routes []route
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r, err := parseRoute(line)
		if err != nil {
			return nil, errors.Wrapf(err, "routes line %d", i+1)
		}
		r.Line = i + 1
		routes = append(routes, r)
	}
	return routes, nil
}

// parseRoute parses one non-empty routes line.
func parseRoute(line string) (route, error) {
	source, rest, ok := strings.Cut(line, "->")
	if !ok {
		return route{}, errors.New("expected `org[/repo] -> team/channel`")
	}
	var r route
	source = strings.ToLower(strings.TrimSpace(source))
	owner, repo, hasRepo := strings.Cut(source, "/")
	if owner == "" || strings.Contains(source, " ") || (hasRepo && (repo == "" || strings.Contains(repo, "/"))) {
		return route{}, errors.Errorf("source %q must be `org` or `org/repo`", source)
	}
	r.Owner, r.Repo = owner, repo

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return route{}, errors.New("missing `team/channel` after ->")
	}
	team, channel, ok := strings.Cut(fields[0], "/")
	if !ok || team == "" || channel == "" || strings.Contains(channel, "/") {
		return route{}, errors.Errorf("destination %q must be `team/channel` (the names in the channel URL)", fields[0])
	}
	r.TeamName, r.ChannelName = strings.ToLower(team), strings.ToLower(channel)

	r.Events = map[string]bool{}
	for _, e := range supportedEvents {
		r.Events[e] = true
	}
	for _, opt := range fields[1:] {
		key, value, ok := strings.Cut(opt, "=")
		switch {
		case ok && key == "events":
			r.Events = map[string]bool{}
			for e := range strings.SplitSeq(value, ",") {
				if !isSupportedEvent(e) {
					return route{}, errors.Errorf("unknown event %q; supported: %s", e, strings.Join(supportedEvents, ","))
				}
				r.Events[e] = true
			}
		case ok && key == "bots" && (value == "include" || value == "exclude"):
			r.IncludeBots = value == "include"
		default:
			return route{}, errors.Errorf("unknown option %q; use events=a,b or bots=include|exclude", opt)
		}
	}
	return r, nil
}

// isSupportedEvent reports whether name is one of the GitHub events this plugin renders.
func isSupportedEvent(name string) bool {
	for _, e := range supportedEvents {
		if e == name {
			return true
		}
	}
	return false
}

// parseConfiguration validates the raw settings and fills the derived fields.
func parseConfiguration(c *configuration) error {
	secrets, err := parseSecrets(c.WebhookSecrets)
	if err != nil {
		return err
	}
	routes, err := parseRoutes(c.Routes)
	if err != nil {
		return err
	}
	seen := map[string]int{}
	for _, r := range routes {
		if _, ok := secrets[r.Owner]; !ok {
			return errors.Errorf("routes line %d: no webhook secret for %q; add `%s = <secret>` to Webhook secrets", r.Line, r.Owner, r.Owner)
		}
		key := fmt.Sprintf("%s/%s>%s/%s", r.Owner, r.Repo, r.TeamName, r.ChannelName)
		if prev, dup := seen[key]; dup {
			return errors.Errorf("routes line %d repeats line %d", r.Line, prev)
		}
		seen[key] = r.Line
	}
	c.secrets, c.routes = secrets, routes
	return nil
}

// matchRoutes returns the routes for a repository: the repo's own routes when it has any,
// otherwise its owner's routes. Several lines for the same source all apply.
func (c *configuration) matchRoutes(fullName string) []route {
	owner, repo, _ := strings.Cut(strings.ToLower(fullName), "/")
	var exact, byOwner []route
	for _, r := range c.routes {
		switch {
		case r.Owner == owner && r.Repo == repo:
			exact = append(exact, r)
		case r.Owner == owner && r.Repo == "":
			byOwner = append(byOwner, r)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return byOwner
}

// getConfiguration returns the active configuration under lock.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}
	return p.configuration
}

// setConfiguration replaces the active configuration under lock. It panics when handed the
// active pointer itself, which would mean a caller mutated it instead of building a new one.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}
		panic("setConfiguration called with the existing configuration")
	}
	p.configuration = configuration
}

// loadConfiguration reads, parses and resolves the settings and makes them active, or changes
// nothing on error. Channels are resolved only once the bot exists, which OnActivate arranges.
func (p *Plugin) loadConfiguration() error {
	c := new(configuration)
	if err := p.API.LoadPluginConfiguration(c); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}
	if err := parseConfiguration(c); err != nil {
		return err
	}
	if p.botUserID != "" {
		if err := p.resolveRoutes(c.routes); err != nil {
			return err
		}
	}
	if len(c.routes) == 0 {
		p.API.LogWarn("GitHub Cards has no routes, so every webhook delivery is ignored")
	}
	p.setConfiguration(c)
	return nil
}

// resolveRoutes looks up each route's channel and adds the bot to its team and channel.
func (p *Plugin) resolveRoutes(routes []route) error {
	for i := range routes {
		r := &routes[i]
		team, appErr := p.API.GetTeamByName(r.TeamName)
		if appErr != nil {
			return errors.Errorf("routes line %d: team %q not found", r.Line, r.TeamName)
		}
		channel, appErr := p.API.GetChannelByName(team.Id, r.ChannelName, false)
		if appErr != nil {
			return errors.Errorf("routes line %d: channel %q not found in team %q", r.Line, r.ChannelName, r.TeamName)
		}
		if _, appErr := p.API.CreateTeamMember(team.Id, p.botUserID); appErr != nil {
			return errors.Wrapf(appErr, "routes line %d: add the bot to team %q", r.Line, r.TeamName)
		}
		if _, appErr := p.API.AddChannelMember(channel.Id, p.botUserID); appErr != nil {
			return errors.Wrapf(appErr, "routes line %d: add the bot to channel %q", r.Line, r.ChannelName)
		}
		r.channelID = channel.Id
	}
	return nil
}

// OnConfigurationChange keeps the previous settings when the new ones are invalid.
func (p *Plugin) OnConfigurationChange() error {
	return p.loadConfiguration()
}
