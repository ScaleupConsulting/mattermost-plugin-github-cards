# GitHub Cards (com.scaleupconsulting.githubcards)

Turns signed GitHub webhooks into one live card per pull request or issue. Requires Mattermost 12.0+.

- Each pull request and issue gets one root post, edited in place as it changes: state (open, draft, merged, closed), `head → base`, diff size, labels, assignees, requested reviewers, each reviewer's decision, CI per workflow for the head commit, comment count, and the issues it closes (or, on an issue card, the pull requests that close it).
- Its thread carries reviews, review comments, comments, close, reopen and merge, and CI failures. A CI success is posted only when it turns a failing card green.
- Editing a comment on GitHub edits its reply; deleting it deletes the reply. Comments from bot accounts are left out unless the route says `bots=include`.
- A push to the default branch is a compact post in the channel. Pushes to other branches only move the card's head commit.
- A card appears the first time any event for that pull request or issue arrives, so work opened before the plugin was installed is picked up too.
- Desktop and web draw the card with the plugin's renderer. Mobile and other clients show the same card as markdown.

## Setup

1. Build with `build/docker-build.sh` (needs only Docker), then upload `dist/com.scaleupconsulting.githubcards-<version>.tar.gz` in System Console > Plugins > Plugin Management.
2. Under System Console > Plugins > GitHub Cards, add one line per GitHub org to **Webhook secrets** (`org = secret`, at least 16 characters) and one line per destination to **Routes**:
   ```
   acme/webapp -> engineering/github
   acme -> engineering/bots events=pull_request,pull_request_review,workflow_run bots=include
   ```
   The plugin's bot, `github-cards`, joins each routed team and channel itself.
3. On GitHub, add an org (or repository) webhook: payload URL `https://<your Mattermost>/plugins/com.scaleupconsulting.githubcards/webhook`, content type `application/json`, the same secret, and the events Issues, Issue comments, Pull requests, Pull request reviews, Pull request review comments, Workflow runs and Pushes.

Deliveries without a valid `X-Hub-Signature-256` get 401. Each `X-GitHub-Delivery` is processed once; a delivery that fails is forgotten, so GitHub's Redeliver button retries it.

## Tests

- `go test ./server/...` covers signature checks, config parsing and routing, state reduction and the markdown fallback, using GitHub's own example payloads in `server/testdata/github`. It also writes the post props the webapp tests render (`UPDATE_GOLDEN=1` refreshes them).
- `cd webapp && npm test` renders those props.
- After `build/docker-build.sh`, `cd e2e && npm install && npm run e2e` starts a throwaway Mattermost 12.0.2 in Docker (compose project `ghc-e2e`, port 18165 on loopback), installs the bundle, posts a signed issue and pull request lifecycle to the webhook, records the channel in Chromium and burns captions into `e2e/out/github-cards-e2e.mp4`. Remove it afterwards with `docker compose -f e2e/compose.yml down -v`.
