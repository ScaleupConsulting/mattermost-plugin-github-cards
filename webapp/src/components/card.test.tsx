import {render, screen, within} from '@testing-library/react';
import React from 'react';

import type {Post} from '@mattermost/types/posts';

import CardPost from './card';
import EventPost from './event';

import cardProps from '../testdata/card_pr.json';
import ciProps from '../testdata/event_ci_failed.json';
import pushProps from '../testdata/event_push.json';
import reviewProps from '../testdata/event_review.json';

/**
 * Builds a post around props the Go tests produced.
 * @param props - post props
 * @returns a post
 */
function postWith(props: unknown): Post {
    return {id: 'p1', message: 'markdown fallback', props} as unknown as Post;
}

test('pull request card shows state, branches, reviewers, CI and links', () => {
    render(<CardPost post={postWith(cardProps)}/>);
    const card = screen.getByTestId('ghc-card');
    expect(within(card).getByText('Open')).toBeInTheDocument();
    expect(within(card).getByText('Codertocat/Hello-World')).toBeInTheDocument();
    expect(within(card).getByText('#2')).toBeInTheDocument();
    expect(within(card).getByRole('link', {name: 'Update the README with new information.'})).toHaveAttribute('href', 'https://github.com/Codertocat/Hello-World/pull/2');
    expect(within(card).getByText('changes')).toBeInTheDocument();
    expect(within(card).getByText('master')).toBeInTheDocument();
    expect(within(card).getByText('+1')).toBeInTheDocument();

    // Codertocat requested changes; octocat is still awaiting review.
    expect(within(card).getByTitle('Codertocat: Changes requested')).toBeInTheDocument();
    expect(within(card).getByTitle('octocat: Awaiting review')).toBeInTheDocument();

    expect(within(card).getByText('Checks failing')).toBeInTheDocument();
    expect(within(card).getByTitle('CI: failure')).toHaveAttribute('href', 'https://github.com/Codertocat/Hello-World/actions/runs/1');
    expect(within(card).getByTitle('Lint: success')).toBeInTheDocument();

    expect(within(card).getByText('bug')).toBeInTheDocument();
    expect(within(card).getByRole('link', {name: /#1/})).toHaveAttribute('href', 'https://github.com/Codertocat/Hello-World/issues/1');
    expect(within(card).getByRole('link', {name: /Open on GitHub/})).toHaveAttribute('href', 'https://github.com/Codertocat/Hello-World/pull/2');
});

test('merged card turns purple and says merged', () => {
    const props = JSON.parse(JSON.stringify(cardProps));
    props.ghc_card.state = 'merged';
    render(<CardPost post={postWith(props)}/>);
    const pill = screen.getByText('Merged');
    expect(pill).toHaveStyle({backgroundColor: '#8250df'});
    expect(screen.getByText(/merged/, {selector: 'span'})).toBeInTheDocument();
});

test('card without props falls back to the message', () => {
    render(<CardPost post={postWith({})}/>);
    expect(screen.getByText('markdown fallback')).toBeInTheDocument();
});

test('review reply shows reviewer, decision and body', () => {
    render(<EventPost post={postWith(reviewProps)}/>);
    const ev = screen.getByTestId('ghc-event');
    expect(ev).toHaveClass('ghc-tone-changes_requested');
    expect(within(ev).getByText('Codertocat')).toBeInTheDocument();
    expect(within(ev).getByText('requested changes')).toBeInTheDocument();
    expect(within(ev).getByText(/Please add a test/)).toBeInTheDocument();
});

test('CI failure reply names the workflow', () => {
    render(<EventPost post={postWith(ciProps)}/>);
    const ev = screen.getByTestId('ghc-event');
    expect(within(ev).getByText('CI failed')).toBeInTheDocument();
    expect(within(ev).getByText(/CI on `ec26c3e`/)).toBeInTheDocument();
});

test('push post lists commits', () => {
    render(<EventPost post={postWith(pushProps)}/>);
    expect(screen.getByText('6113728')).toHaveAttribute('href', expect.stringContaining('6113728f27ae82c7b1a177c8d03f9e96e0adf246'));
    expect(screen.getByText('Initial commit')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Codertocat/Hello-World'})).toBeInTheDocument();
});
