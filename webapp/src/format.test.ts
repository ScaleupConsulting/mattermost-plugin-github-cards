import {ciResult, labelTextColor, reviewerChips, stateMeta} from './format';
import type {Card, Check} from './types';

/**
 * Builds a check with the given status and conclusion.
 * @param status - run status
 * @param conclusion - run conclusion
 * @returns a check
 */
function check(status: string, conclusion = ''): Check {
    return {workflow_id: 1, name: 'CI', run_id: 1, attempt: 1, status, conclusion, url: ''};
}

test('ciResult matches the server reduction', () => {
    expect(ciResult([])).toBe('');
    expect(ciResult([check('completed', 'success'), check('in_progress')])).toBe('pending');
    expect(ciResult([check('in_progress'), check('completed', 'timed_out')])).toBe('failure');
    expect(ciResult([check('completed', 'success'), check('completed', 'skipped')])).toBe('success');
});

test('a re-requested reviewer shows as awaiting', () => {
    const card = {
        reviews: [{reviewer: {login: 'Alice'}, state: 'approved'}, {reviewer: {login: 'bob'}, state: 'changes_requested'}],
        requested_reviewers: [{login: 'alice'}],
    } as unknown as Card;
    expect(reviewerChips(card).map((c) => `${c.person.login}:${c.state}`)).toEqual(['bob:changes_requested', 'alice:requested']);
});

test('label text stays readable', () => {
    expect(labelTextColor('ffffff')).toBe('#1f2328');
    expect(labelTextColor('d73a4a')).toBe('#ffffff');
    expect(labelTextColor('zz')).toBe('inherit');
});

test('closed issues are purple, closed pull requests red', () => {
    expect(stateMeta('issue', 'closed').color).toBe('#8250df');
    expect(stateMeta('pr', 'closed').color).toBe('#cf222e');
});
