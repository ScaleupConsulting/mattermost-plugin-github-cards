import type {Card, Check, Person, Review} from './types';

export type StateMeta = {label: string; color: string};

const STATES: Record<string, StateMeta> = {
    open: {label: 'Open', color: '#1a7f37'},
    draft: {label: 'Draft', color: '#6e7781'},
    merged: {label: 'Merged', color: '#8250df'},
    closed: {label: 'Closed', color: '#cf222e'},
    not_planned: {label: 'Not planned', color: '#6e7781'},
};

/**
 * Looks up the pill label and colour for a card state. GitHub's own colours, so people read it at a glance.
 * @param kind - 'pr' or 'issue'
 * @param state - normalized card state
 * @returns label and colour
 */
export function stateMeta(kind: string, state: string): StateMeta {
    if (kind === 'issue' && state === 'closed') {
        return {label: 'Closed', color: '#8250df'};
    }
    return STATES[state] || STATES.open;
}

export type CIResult = '' | 'pending' | 'success' | 'failure';

const FAILED = new Set(['failure', 'timed_out', 'startup_failure', 'action_required']);

/**
 * Classifies one workflow run as the card shows it.
 * @param check - the run
 * @returns its result
 */
export function checkResult(check: Check): CIResult | 'neutral' {
    if (check.status !== 'completed') {
        return 'pending';
    }
    if (FAILED.has(check.conclusion || '')) {
        return 'failure';
    }
    return check.conclusion === 'success' ? 'success' : 'neutral';
}

/**
 * Reduces all checks to one result, the same way the server does.
 * @param checks - runs for the head commit
 * @returns overall result, or '' with no runs
 */
export function ciResult(checks: Check[] | undefined): CIResult {
    if (!checks || checks.length === 0) {
        return '';
    }
    let result: CIResult = 'success';
    for (const c of checks) {
        const r = checkResult(c);
        if (r === 'failure') {
            return 'failure';
        }
        if (r === 'pending') {
            result = 'pending';
        }
    }
    return result;
}

export type ReviewerChip = {person: Person; state: string; url?: string};

/**
 * Merges review decisions and pending requests into one chip per reviewer.
 * A re-requested reviewer shows as awaiting, as on GitHub.
 * @param card - the card
 * @returns chips, decisions first
 */
export function reviewerChips(card: Card): ReviewerChip[] {
    const requested = new Set((card.requested_reviewers || []).map((p) => p.login.toLowerCase()));
    const chips: ReviewerChip[] = (card.reviews || []).
        filter((r: Review) => !requested.has(r.reviewer.login.toLowerCase())).
        map((r: Review) => ({person: r.reviewer, state: r.state, url: r.url}));
    for (const p of card.requested_reviewers || []) {
        chips.push({person: p, state: 'requested'});
    }
    return chips;
}

/**
 * Picks black or white text for a GitHub label colour so the name stays readable.
 * @param hex - six hex digits without #
 * @returns a CSS colour
 */
export function labelTextColor(hex: string): string {
    const n = parseInt(hex, 16);
    if (isNaN(n) || hex.length !== 6) {
        return 'inherit';
    }
    const r = (n >> 16) & 255;
    const g = (n >> 8) & 255;
    const b = n & 255;
    const luminance = ((0.299 * r) + (0.587 * g) + (0.114 * b)) / 255;
    return luminance > 0.6 ? '#1f2328' : '#ffffff';
}

/**
 * Formats a count with its noun.
 * @param n - the count
 * @param one - singular noun
 * @param many - plural noun
 * @returns e.g. "3 files"
 */
export function plural(n: number, one: string, many: string): string {
    return `${n} ${n === 1 ? one : many}`;
}
