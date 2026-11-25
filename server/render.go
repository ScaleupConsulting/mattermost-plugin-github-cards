package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Custom post types. The webapp draws them; every other client shows the markdown Message.
const (
	postTypeCard  = "custom_ghc_card"
	postTypeEvent = "custom_ghc_event"

	propCard  = "ghc_card"
	propEvent = "ghc_event"

	// maxPropBody caps a comment body kept in props; the webapp links to GitHub for the rest.
	maxPropBody = 6000
	// maxFallbackBody caps a comment body in the markdown fallback.
	maxFallbackBody = 1500
	// maxPushCommits caps the commits listed for one push.
	maxPushCommits = 10
)

var markdownSpecial = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`, "<", "&lt;", ">", "&gt;", "~", `\~`, "#", `\#`, "|", `\|`)

// escapeMarkdown makes GitHub-supplied text such as a title render literally.
func escapeMarkdown(s string) string {
	return markdownSpecial.Replace(s)
}

var mentionRE = regexp.MustCompile(`(^|[^\w])@([\w.-])`)

// quietMentions stops GitHub @handles in a body from notifying Mattermost users who happen to share
// the name, and stops @channel or @here from paging a channel, by putting a word joiner after the @.
func quietMentions(s string) string {
	return mentionRE.ReplaceAllString(s, "$1@\u2060$2")
}

// truncate shortens s to at most n bytes on a rune boundary, adding an ellipsis when it cuts.
func truncate(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	cut := n
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut]) + "…", true
}

// isRuneStart reports whether b begins a UTF-8 sequence.
func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

var stateWords = map[string]string{
	stateOpen: "Open", stateDraft: "Draft", stateClosed: "Closed", stateMerged: "Merged", stateNotPlanned: "Closed as not planned",
}

var reviewEmoji = map[string]string{
	"approved": ":white_check_mark:", "changes_requested": ":x:", "commented": ":speech_balloon:", "dismissed": ":heavy_minus_sign:",
}

// plural formats a count with a singular or plural noun.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// login renders a GitHub login as plain bold text, never as a Mattermost mention.
func login(p Person) string {
	return "**" + escapeMarkdown(p.Login) + "**"
}

// cardMarkdown is the card as plain markdown, for mobile and any client without the webapp plugin.
func cardMarkdown(c *Card) string {
	var b strings.Builder
	noun := "pull request"
	if c.Kind == kindIssue {
		noun = "issue"
	}
	fmt.Fprintf(&b, "#### [%s#%d](%s) %s\n", escapeMarkdown(c.Repo), c.Number, c.URL, escapeMarkdown(c.Title))

	line := []string{fmt.Sprintf("**%s** %s by %s", stateWords[c.State], noun, escapeMarkdown(c.Author.Login))}
	if c.Kind == kindPR {
		if c.Head != "" {
			line = append(line, fmt.Sprintf("`%s` → `%s`", c.Head, c.Base))
		}
		if c.ChangedFiles > 0 {
			line = append(line, fmt.Sprintf("+%d −%d in %s", c.Additions, c.Deletions, plural(c.ChangedFiles, "file", "files")))
		}
	}
	if c.Comments > 0 {
		line = append(line, plural(c.Comments, "comment", "comments"))
	}
	b.WriteString(strings.Join(line, " · ") + "\n")

	if c.Kind == kindPR {
		if r := reviewersMarkdown(c); r != "" {
			b.WriteString("Reviewers: " + r + "\n")
		}
		if ci := ciMarkdown(c); ci != "" {
			b.WriteString("CI: " + ci + "\n")
		}
	}

	var meta []string
	if len(c.Labels) > 0 {
		var ls []string
		for _, l := range c.Labels {
			ls = append(ls, "`"+strings.ReplaceAll(l.Name, "`", "'")+"`")
		}
		meta = append(meta, "Labels: "+strings.Join(ls, " "))
	}
	if len(c.Assignees) > 0 {
		var as []string
		for _, a := range c.Assignees {
			as = append(as, escapeMarkdown(a.Login))
		}
		meta = append(meta, "Assignees: "+strings.Join(as, ", "))
	}
	if len(c.ClosesIssues) > 0 {
		var is []string
		for _, n := range c.ClosesIssues {
			is = append(is, fmt.Sprintf("[#%d](%s/issues/%d)", n, c.RepoURL, n))
		}
		meta = append(meta, "Closes "+strings.Join(is, ", "))
	}
	for _, pr := range c.LinkedPRs {
		meta = append(meta, fmt.Sprintf("Linked PR [#%d](%s) (%s)", pr.Number, pr.URL, strings.ToLower(stateWords[pr.State])))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " · ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// reviewersMarkdown lists each reviewer's decision, then reviews still requested.
func reviewersMarkdown(c *Card) string {
	var parts []string
	requested := map[string]bool{}
	for _, p := range c.RequestedReviewers {
		requested[strings.ToLower(p.Login)] = true
	}
	for _, r := range c.Reviews {
		if requested[strings.ToLower(r.Reviewer.Login)] {
			continue
		}
		parts = append(parts, reviewEmoji[r.State]+" "+escapeMarkdown(r.Reviewer.Login))
	}
	for _, p := range c.RequestedReviewers {
		name := escapeMarkdown(p.Login)
		if p.IsTeam {
			name = "team " + name
		}
		parts = append(parts, ":hourglass_flowing_sand: "+name+" (requested)")
	}
	return strings.Join(parts, " · ")
}

// ciMarkdown gives the overall CI result and each workflow for the head commit.
func ciMarkdown(c *Card) string {
	if len(c.Checks) == 0 {
		return ""
	}
	icon := map[string]string{ciSuccess: ":white_check_mark: passing", ciFailure: ":x: failing", ciPending: ":hourglass_flowing_sand: running"}
	parts := []string{icon[c.ciStatus()] + " on `" + shortSHA(c.HeadSHA) + "`"}
	for _, ch := range c.Checks {
		mark := ":hourglass_flowing_sand:"
		switch {
		case ch.Status != "completed":
		case isFailedConclusion(ch.Conclusion):
			mark = ":x:"
		case ch.Conclusion == "success":
			mark = ":white_check_mark:"
		default:
			mark = ":heavy_minus_sign:"
		}
		parts = append(parts, fmt.Sprintf("%s [%s](%s)", mark, escapeMarkdown(ch.Name), ch.URL))
	}
	return strings.Join(parts, " · ")
}

var replyEmoji = map[string]string{
	"approved": ":white_check_mark:", "changes_requested": ":x:", "commented": ":speech_balloon:", "dismissed": ":heavy_minus_sign:",
	stateMerged: ":twisted_rightwards_arrows:", stateClosed: ":no_entry_sign:", stateNotPlanned: ":no_entry_sign:", stateOpen: ":arrows_counterclockwise:",
	stateDraft: ":pencil2:", "failure": ":x:", "success": ":white_check_mark:", "push": ":arrow_up:",
}

// replyMarkdown is a reply as plain markdown, for clients without the webapp plugin.
func replyMarkdown(r *Reply) string {
	var b strings.Builder
	b.WriteString(replyEmoji[r.Tone] + " ")
	switch r.Kind {
	case replyCIFailed:
		fmt.Fprintf(&b, "CI failed: %s", r.Context)
	case replyCIPassed:
		fmt.Fprintf(&b, "CI passing again: %s", r.Context)
	case replyPush:
		fmt.Fprintf(&b, "%s %s in [%s](%s)", login(r.Actor), r.Verb, escapeMarkdown(r.Repo), r.RepoURL)
	default:
		b.WriteString(login(r.Actor) + " " + r.Verb)
		if r.Context != "" {
			b.WriteString(" " + r.Context)
		}
	}
	if r.URL != "" {
		fmt.Fprintf(&b, " · [view on GitHub](%s)", r.URL)
	}
	if r.Kind == replyPush {
		for i, c := range r.Commits {
			if i == maxPushCommits {
				fmt.Fprintf(&b, "\n- and %d more", len(r.Commits)-maxPushCommits)
				break
			}
			fmt.Fprintf(&b, "\n- [`%s`](%s) %s (%s)", shortSHA(c.SHA), c.URL, escapeMarkdown(c.Message), escapeMarkdown(c.Author))
		}
	}
	if trimmed := strings.TrimSpace(r.Body); trimmed != "" {
		body, cut := truncate(trimmed, maxFallbackBody)
		if cut {
			body += fmt.Sprintf(" [read more](%s)", r.URL)
		}
		b.WriteString("\n\n" + quietMentions(body))
	}
	return b.String()
}

// toProps converts a value to the plain JSON map that post props carry over plugin RPC.
func toProps(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // the card and reply types always marshal
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

// cardPost builds the root post for a card.
func cardPost(botID, channelID string, c *Card) *model.Post {
	post := &model.Post{UserId: botID, ChannelId: channelID, Type: postTypeCard, Message: cardMarkdown(c)}
	post.AddProp(propCard, toProps(c))
	post.AddProp("disable_group_highlight", true)
	return post
}

// replyPost builds a thread reply, or a channel post when rootID is empty.
func replyPost(botID, channelID, rootID string, r *Reply) *model.Post {
	prop := *r
	if len(r.Commits) > maxPushCommits {
		prop.Commits = r.Commits[:maxPushCommits]
	}
	prop.Body, _ = truncate(r.Body, maxPropBody)
	post := &model.Post{UserId: botID, ChannelId: channelID, RootId: rootID, Type: postTypeEvent, Message: replyMarkdown(r)}
	props := toProps(&prop)
	props["total_commits"] = len(r.Commits)
	post.AddProp(propEvent, props)
	post.AddProp("disable_group_highlight", true)
	return post
}
