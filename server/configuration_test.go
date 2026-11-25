package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfiguration(t *testing.T) {
	c := &configuration{ //nolint:gosec // fake secrets for the parser
		WebhookSecrets: "# comment\nacme = fake-acme-secret-00000001\nglobex=fake-globex-secret-0000002\n",
		Routes: `
# acme
acme/webapp -> engineering/github
acme -> engineering/github-other events=push
globex -> globex/bots events=pull_request,workflow_run bots=include
`,
	}
	require.NoError(t, parseConfiguration(c))
	assert.Equal(t, "fake-acme-secret-00000001", c.secrets["acme"])
	require.Len(t, c.routes, 3)

	r := c.routes[0]
	assert.Equal(t, 3, r.Line)
	assert.Equal(t, "acme", r.Owner)
	assert.Equal(t, "webapp", r.Repo)
	assert.Equal(t, "engineering", r.TeamName)
	assert.Equal(t, "github", r.ChannelName)
	assert.Len(t, r.Events, len(supportedEvents), "default is every event")
	assert.False(t, r.IncludeBots, "bot comments are left out by default")

	f := c.routes[2]
	assert.Equal(t, map[string]bool{"pull_request": true, "workflow_run": true}, f.Events)
	assert.True(t, f.IncludeBots)

	// A repo's own route wins over its org's; another repo in the org gets the org route.
	got := c.matchRoutes("ACME/Webapp")
	require.Len(t, got, 1)
	assert.Equal(t, "github", got[0].ChannelName)
	got = c.matchRoutes("acme/website")
	require.Len(t, got, 1)
	assert.Equal(t, "github-other", got[0].ChannelName)
	assert.Empty(t, c.matchRoutes("someone/else"))
}

func TestParseConfigurationErrors(t *testing.T) {
	cases := map[string]struct{ secrets, routes, want string }{
		"secret without equals":  {"acme fake-secret-0000000001", "", "webhook secrets line 1: expected `org = secret`"},
		"short secret":           {"acme = short", "", "at least 16 characters"},
		"duplicate secret":       {"a = fake-secret-0000000001\nA = fake-secret-0000000002", "", `line 2: "a" has a second secret`},
		"route without arrow":    {"a = fake-secret-0000000001", "a team/chan", "routes line 1: expected"},
		"route without channel":  {"a = fake-secret-0000000001", "a -> team", "must be `team/channel`"},
		"unknown event":          {"a = fake-secret-0000000001", "a -> t/c events=star", `unknown event "star"`},
		"unknown option":         {"a = fake-secret-0000000001", "a -> t/c verbose", `unknown option "verbose"`},
		"route without a secret": {"a = fake-secret-0000000001", "\nb/repo -> t/c", `routes line 2: no webhook secret for "b"`},
		"duplicate route":        {"a = fake-secret-0000000001", "a -> t/c\na -> t/c", "routes line 2 repeats line 1"},
		"bad source":             {"a = fake-secret-0000000001", "a/b/c -> t/c", "must be `org` or `org/repo`"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := parseConfiguration(&configuration{WebhookSecrets: tc.secrets, Routes: tc.routes})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestEmptyConfigurationIsValid(t *testing.T) {
	require.NoError(t, parseConfiguration(&configuration{}))
}
