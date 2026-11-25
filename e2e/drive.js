// E2E driver: replays a issue and PR lifecycle as signed GitHub webhooks into a
// throwaway Mattermost, records the channel in Chromium, and checks the result over the API.
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const {chromium} = require('playwright');

const BASE = 'http://localhost:18165';
const HOOK = `${BASE}/plugins/com.scaleupconsulting.githubcards/webhook`;
const SECRET = 'fake-e2e-secret-not-real-0001';
const FIX = path.join(__dirname, '..', 'server', 'testdata', 'github');
const OUT = path.resolve(process.argv[2] || 'out');
// CHROME may point at an already installed Chromium; otherwise Playwright uses its own.
const CHROME = process.env.CHROME || undefined;
const VIEW = {width: 1280, height: 860};

const log = (...a) => console.log(new Date().toISOString().slice(11, 19), ...a);

/**
 * Loads a GitHub example payload.
 * @param {string} name - path under testdata/github
 * @returns {object} parsed payload
 */
const load = (name) => JSON.parse(fs.readFileSync(path.join(FIX, name), 'utf8'));

const AVATAR = {Codertocat: 21031067, octocat: 583231, hubot: 480938, 'dependabot[bot]': 49699333};

/**
 * Builds a GitHub user object.
 * @param {string} login - account login
 * @returns {object} user
 */
const user = (login) => ({
    login, id: AVATAR[login] || 1, type: login.endsWith('[bot]') ? 'Bot' : 'User',
    avatar_url: `https://avatars.githubusercontent.com/u/${AVATAR[login]}?v=4`, html_url: `https://github.com/${login}`,
});

const REPO = {
    ...load('pull_request/opened.payload.json').repository,
    id: 4242, name: 'webapp', full_name: 'acme/webapp', html_url: 'https://github.com/acme/webapp',
    default_branch: 'main', owner: {...user('octocat'), login: 'acme', type: 'Organization'},
};
const SHA1 = '3f9c2a1be0d4c6f7a8b9c0d1e2f3a4b5c6d7e8f9';
const SHA2 = '7d41e0c2b9a8f7e6d5c4b3a2f1e0d9c8b7a6f5e4';
const MERGE = 'c0ffee12345678901234567890abcdef12345678';
let clock = Date.parse('2026-11-26T01:00:00Z');

/**
 * Advances the fake GitHub clock so each snapshot is newer than the last.
 * @returns {string} ISO timestamp
 */
const tick = () => new Date(clock += 60000).toISOString().replace('.000', '');

const LABELS = [{name: 'bug', color: 'd73a4a'}, {name: 'auth', color: '0e8a16'}];
const pr = {
    ...load('pull_request/opened.payload.json').pull_request,
    number: 42, title: 'Keep ?next= through the login redirect', html_url: 'https://github.com/acme/webapp/pull/42',
    body: 'Carries the original URL through SSO.\n\nFixes #41', user: user('Codertocat'), state: 'open', draft: true,
    merged: false, merged_at: null, merged_by: null, labels: [LABELS[0]], assignees: [user('Codertocat')],
    requested_reviewers: [user('octocat'), user('hubot')], requested_teams: [],
    head: {ref: 'fix/login-next', sha: SHA1}, base: {ref: 'main', sha: 'b'.repeat(40)},
    additions: 48, deletions: 12, changed_files: 3, commits: 2, comments: 0, review_comments: 0,
};
const issue = {
    ...load('issues/opened.payload.json').issue,
    number: 41, title: 'Login redirect drops the ?next= parameter', html_url: 'https://github.com/acme/webapp/issues/41',
    user: user('hubot'), labels: [LABELS[0], LABELS[1]], assignees: [user('Codertocat')], comments: 0, state: 'open', state_reason: null,
    pull_request: undefined,
};

/**
 * Signs and delivers one webhook, returning the HTTP status and body.
 * @param {string} event - X-GitHub-Event
 * @param {object} payload - the body
 * @param {object} [opt] - delivery id or a signature override
 * @returns {Promise<{status: number, text: string, delivery: string}>}
 */
async function send(event, payload, opt = {}) {
    const body = JSON.stringify(payload);
    const delivery = opt.delivery || crypto.randomUUID();
    const sig = opt.sig ?? 'sha256=' + crypto.createHmac('sha256', SECRET).update(body).digest('hex');
    const headers = {'Content-Type': 'application/json', 'X-GitHub-Event': event, 'X-GitHub-Delivery': delivery};
    if (sig) {
        headers['X-Hub-Signature-256'] = sig;
    }
    const res = await fetch(HOOK, {method: 'POST', headers, body});
    const text = (await res.text()).trim();
    log(event, payload.action || '', res.status, text);
    return {status: res.status, text, delivery};
}

/**
 * Builds an event payload around the shared repository and organization.
 * @param {string} sender - login of the acting user
 * @param {object} extra - event fields
 * @returns {object} payload
 */
const ev = (sender, extra) => ({...extra, repository: REPO, organization: {login: 'acme'}, sender: user(sender)});

/**
 * Snapshot of the pull request at the current fake time.
 * @param {object} changes - fields to change first
 * @returns {object} pull request
 */
const prNow = (changes = {}) => Object.assign(pr, changes, {updated_at: tick()}) && {...pr};

/**
 * Snapshot of the issue at the current fake time.
 * @param {object} changes - fields to change first
 * @returns {object} issue
 */
const issueNow = (changes = {}) => Object.assign(issue, changes, {updated_at: tick()}) && {...issue};

/**
 * A workflow_run event for the pull request's head commit.
 * @param {number} id - run id
 * @param {string} name - workflow name
 * @param {string} status - run status
 * @param {string|null} conclusion - run conclusion
 * @returns {object} payload
 */
const run = (id, name, status, conclusion) => ev('Codertocat', {
    action: status === 'completed' ? 'completed' : 'in_progress',
    workflow: {name, path: `.github/workflows/${name.toLowerCase()}.yml`},
    workflow_run: {
        ...load('workflow_run/completed.with-pull-requests.payload.json').workflow_run,
        id, name, workflow_id: name === 'CI' ? 11 : 12, head_sha: pr.head.sha, head_branch: pr.head.ref, status, conclusion,
        run_attempt: 1, html_url: `https://github.com/acme/webapp/actions/runs/${id}`,
        pull_requests: [{number: 42, head: {ref: pr.head.ref, sha: pr.head.sha}, base: {ref: 'main', sha: pr.base.sha}}],
    },
});

let commentID = 9000;

/**
 * The pull request as the issues API shows it, which is what issue_comment payloads carry.
 * @param {number} comments - comment count
 * @returns {object} issue
 */
const prIssue = (comments) => ({
    ...issue, number: 42, title: pr.title, html_url: pr.html_url, user: pr.user, labels: pr.labels, assignees: pr.assignees,
    comments, state: 'open', updated_at: tick(), pull_request: {html_url: pr.html_url},
});

/**
 * A comment object.
 * @param {string} login - author
 * @param {string} body - markdown
 * @param {object} extra - review comment fields
 * @returns {object} comment
 */
const comment = (login, body, extra = {}) => ({
    id: ++commentID, body, user: user(login), html_url: `https://github.com/acme/webapp/pull/42#discussion_r${commentID}`, ...extra,
});

/**
 * Calls the Mattermost REST API as the admin.
 * @param {string} token - session token
 * @param {string} p - path under /api/v4
 * @returns {Promise<object>} JSON response
 */
async function api(token, p) {
    const res = await fetch(`${BASE}/api/v4${p}`, {headers: {Authorization: `Bearer ${token}`}});
    return res.json();
}

(async () => {
    fs.mkdirSync(OUT, {recursive: true});
    const login = await fetch(`${BASE}/api/v4/users/login`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({login_id: 'admin', password: 'Fake-e2e-Pass-123!'}),
    });
    const token = login.headers.get('token');
    if (!token) {
        throw new Error(`login failed: ${login.status}`);
    }

    // Security checks before the story: unsigned and wrongly signed deliveries are refused.
    const probe = ev('Codertocat', {action: 'opened', pull_request: prNow()});
    const unsigned = await send('pull_request', probe, {sig: ''});
    const forged = await send('pull_request', probe, {sig: 'sha256=' + crypto.createHmac('sha256', 'fake-wrong-secret-000000').update(JSON.stringify(probe)).digest('hex')});
    if (unsigned.status !== 401 || forged.status !== 401) {
        throw new Error('signature checks did not refuse');
    }

    const browser = await chromium.launch({executablePath: CHROME, headless: true});
    // Sign in through the real login page in an unrecorded context, then record with that session.
    const pre = await browser.newContext({viewport: VIEW});
    const lp = await pre.newPage();
    await lp.goto(`${BASE}/login`);
    const viewInBrowser = lp.getByText('View in Browser');
    await viewInBrowser.waitFor({timeout: 20000});
    await viewInBrowser.click();
    await lp.locator('#input_loginId').fill('admin');
    await lp.locator('#input_password-input').fill('Fake-e2e-Pass-123!');
    await lp.keyboard.press('Enter');
    await lp.waitForURL(/channels/, {timeout: 60000});
    const state = await pre.storageState();
    await pre.close();
    const context = await browser.newContext({viewport: VIEW, recordVideo: {dir: OUT, size: VIEW}, deviceScaleFactor: 1, storageState: state});
    const page = await context.newPage();
    const t0 = Date.now();
    const captions = [];
    let shot = 0;

    /**
     * Shows a caption from now until the next one.
     * @param {string} text - caption
     */
    const caption = (text) => {
        const t = (Date.now() - t0) / 1000;
        if (captions.length) {
            captions[captions.length - 1].end = t;
        }
        captions.push({start: t, end: t + 60, text});
        log('CAPTION', text);
    };

    /**
     * Waits for the UI to settle and saves a screenshot named after the step.
     * @param {string} name - file stem
     * @param {number} ms - settle time
     */
    const hold = async (name, ms = 4500) => {
        await page.mouse.move(8, 600);
        await page.waitForTimeout(ms);
        if (name) {
            await page.screenshot({path: path.join(OUT, `${String(++shot).padStart(2, '0')}-${name}.png`)});
        }
    };

    await page.goto(`${BASE}/engineering/channels/github`);
    await page.waitForSelector('#post_textbox, [data-testid="post_textbox"]', {timeout: 60000});
    caption('An empty ~github channel in Mattermost 12.0.2, with GitHub Cards installed');
    await hold('empty', 3000);

    caption('Issue #41 is opened on GitHub: one card, with labels and assignee');
    await send('issues', ev('hubot', {action: 'opened', issue: issueNow()}));
    await hold('issue-card');

    caption('PR #42 opens as a draft: branches, diff size, reviewers requested, "Closes #41"');
    await send('pull_request', ev('Codertocat', {action: 'opened', pull_request: prNow()}));
    await send('workflow_run', run(501, 'Lint', 'completed', 'success'));
    await send('workflow_run', run(502, 'CI', 'in_progress', null));
    await hold('pr-draft', 5000);

    caption('The issue card now links PR #42; the PR is marked ready for review');
    await send('pull_request', ev('Codertocat', {action: 'ready_for_review', pull_request: prNow({draft: false})}));
    await hold('pr-ready');

    caption('hubot requests changes and CI fails: chips and badge turn red in place');
    await send('pull_request_review', ev('hubot', {
        action: 'submitted', pull_request: prNow({requested_reviewers: [user('octocat')]}),
        review: {id: 7001, state: 'changes_requested', body: 'The `next` value needs to be checked against our own origin, otherwise this is an open redirect.', user: user('hubot'), html_url: 'https://github.com/acme/webapp/pull/42#pullrequestreview-7001'},
    }));
    await send('pull_request_review_comment', ev('octocat', {
        action: 'created', pull_request: prNow(),
        comment: comment('octocat', 'Could we reuse `isSafeRedirect()` from `src/auth/url.ts` here?', {path: 'src/auth/redirect.ts', line: 27}),
    }));
    await send('workflow_run', run(502, 'CI', 'completed', 'failure'));
    await hold('changes-ci-fail', 5500);

    // A bot comment is dropped by default, and GitHub retrying a delivery is deduplicated.
    await send('issue_comment', ev('dependabot[bot]', {action: 'created', issue: prIssue(0), comment: comment('dependabot[bot]', 'Bumps lodash from 4.17.20 to 4.17.21.')}));
    const first = await send('workflow_run', run(503, 'CI', 'in_progress', null));
    const dup = await send('workflow_run', run(503, 'CI', 'in_progress', null), {delivery: first.delivery});
    if (dup.text !== 'duplicate') {
        throw new Error('duplicate delivery was processed again');
    }

    caption('A fix is pushed: CI goes green, and one reply says CI is passing again');
    await send('pull_request', ev('Codertocat', {action: 'synchronize', pull_request: prNow({head: {ref: 'fix/login-next', sha: SHA2}, commits: 3, additions: 61, deletions: 14})}));
    await send('workflow_run', run(504, 'Lint', 'completed', 'success'));
    await send('workflow_run', run(505, 'CI', 'completed', 'success'));
    await hold('ci-pass');

    caption('Both reviewers approve; an edited comment updates its reply, never a second post');
    await send('pull_request_review', ev('hubot', {
        action: 'submitted', pull_request: prNow({requested_reviewers: []}),
        review: {id: 7002, state: 'approved', body: 'Thanks, the origin check covers it.', user: user('hubot'), html_url: 'https://github.com/acme/webapp/pull/42#pullrequestreview-7002'},
    }));
    await send('pull_request_review', ev('octocat', {
        action: 'submitted', pull_request: prNow(),
        review: {id: 7003, state: 'approved', body: '', user: user('octocat'), html_url: 'https://github.com/acme/webapp/pull/42#pullrequestreview-7003'},
    }));
    const thanks = comment('Codertocat', 'Thanks both! Mergng once CI is green.');
    await send('issue_comment', ev('Codertocat', {action: 'created', issue: prIssue(1), comment: thanks}));
    await send('issue_comment', ev('Codertocat', {action: 'edited', issue: prIssue(1), comment: {...thanks, body: 'Thanks both! Merging once CI is green.'}, changes: {body: {from: thanks.body}}}));
    await hold('approved');

    caption('octocat merges: the card turns purple with who merged, into main, as which commit');
    await send('pull_request', ev('octocat', {action: 'closed', pull_request: prNow({state: 'closed', merged: true, merged_at: new Date(clock).toISOString(), merged_by: user('octocat'), merge_commit_sha: MERGE, comments: 1, review_comments: 1})}));
    await send('issues', ev('Codertocat', {action: 'closed', issue: issueNow({state: 'closed', state_reason: 'completed', comments: 0})}));
    await send('push', ev('octocat', {
        ref: 'refs/heads/main', before: 'b'.repeat(40), after: MERGE, compare: 'https://github.com/acme/webapp/compare/bbbbbbb...c0ffee1',
        forced: false, deleted: false, created: false, pusher: {name: 'octocat'},
        commits: [{id: MERGE, message: 'Merge pull request #42 from acme/fix/login-next\n\nKeep ?next= through the login redirect', url: `https://github.com/acme/webapp/commit/${MERGE}`, author: {name: 'The Octocat', username: 'octocat'}}],
    }));
    await hold('merged', 5000);

    caption('The push to main is its own compact post, never inside a PR thread');
    await page.locator('[data-testid="ghc-event"]').last().scrollIntoViewIfNeeded();
    await hold('push', 2500);

    caption('Opening the card\'s thread: reviews, CI failure, recovery and merge, in order');
    const card = page.locator('[data-testid="ghc-card"]').filter({has: page.locator('.ghc-title', {hasText: 'Keep ?next='})});
    await card.scrollIntoViewIfNeeded();
    const post = page.locator('.post').filter({has: card});
    await post.hover();
    await post.locator('button[aria-label*="reply" i], .ThreadFooter button.ReplyButton, .ThreadFooter button').first().click();
    await page.waitForSelector('#rhsContainer [data-testid="ghc-event"]', {timeout: 15000});
    await hold('thread', 6000);

    caption('Slack\'s GitHub app showed none of this: no reviews, no CI, no branches or diff size');
    await hold('final', 5000);

    // Turn the webapp renderer off to show what the mobile app and other clients draw from the same post.
    require('child_process').execFileSync('docker', ['exec', 'ghc-e2e-mattermost-1', 'mmctl', '--local', 'plugin', 'disable', 'com.scaleupconsulting.githubcards']);
    await page.reload();
    await page.waitForSelector('#post_textbox, [data-testid="post_textbox"]', {timeout: 60000});
    caption('Without the plugin\'s renderer (mobile app, other clients) the same posts read as markdown');
    await page.locator('.post').filter({hasText: 'Keep ?next= through the login redirect'}).last().scrollIntoViewIfNeeded();
    await hold('markdown-fallback', 6000);
    captions[captions.length - 1].end = (Date.now() - t0) / 1000;

    await context.close();
    await browser.close();
    fs.writeFileSync(path.join(OUT, 'captions.json'), JSON.stringify(captions, null, 2));

    // API evidence: one root per card, edited in place, and the expected replies.
    const team = await api(token, '/teams/name/engineering');
    const ch = await api(token, `/teams/${team.id}/channels/name/github`);
    const posts = await api(token, `/channels/${ch.id}/posts?per_page=200`);
    const all = posts.order.map((id) => posts.posts[id]).reverse();
    const roots = all.filter((p) => !p.root_id);
    const summary = roots.map((r) => ({
        type: r.type, edited: r.edit_at > 0, title: r.props.ghc_card?.title || r.props.ghc_event?.verb,
        state: r.props.ghc_card?.state, ci: (r.props.ghc_card?.checks || []).map((c) => `${c.name}:${c.conclusion || c.status}`),
        reviews: (r.props.ghc_card?.reviews || []).map((v) => `${v.reviewer.login}:${v.state}`),
        replies: all.filter((p) => p.root_id === r.id).map((p) => `${p.props.ghc_event.kind}:${p.props.ghc_event.actor.login || ''} ${p.props.ghc_event.verb}${p.edit_at ? ' (edited)' : ''}`),
    }));
    fs.writeFileSync(path.join(OUT, 'api-summary.json'), JSON.stringify(summary, null, 2));
    const prRoot = roots.find((r) => r.props.ghc_card?.number === 42);
    fs.writeFileSync(path.join(OUT, 'pr-card-fallback.md'), prRoot.message + '\n');
    console.log(JSON.stringify(summary, null, 2));
})().catch((e) => {
    console.error(e);
    process.exit(1);
});
