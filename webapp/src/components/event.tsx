import React from 'react';

import type {Post} from '@mattermost/types/posts';

import {Avatar} from './card';
import {ExtLink} from './ext_link';
import {CheckIcon, CommentIcon, CommitIcon, CrossIcon, ExternalIcon, IssueClosedIcon, MergeIcon, NeutralIcon, PullRequestIcon} from './icons';
import {Markdown} from './markdown';

import type {EventReply} from '../types';

/**
 * Picks the coloured badge icon for a reply's tone.
 * @param props - the tone
 * @returns icon element
 */
function ToneIcon({tone}: {tone: string}) {
    switch (tone) {
    case 'approved':
    case 'success':
        return <CheckIcon size={14}/>;
    case 'changes_requested':
    case 'failure':
        return <CrossIcon size={14}/>;
    case 'merged':
        return <MergeIcon size={14}/>;
    case 'closed':
    case 'not_planned':
        return <IssueClosedIcon size={14}/>;
    case 'open':
    case 'draft':
        return <PullRequestIcon size={14}/>;
    case 'push':
        return <CommitIcon size={14}/>;
    case 'dismissed':
        return <NeutralIcon size={14}/>;
    }
    return <CommentIcon size={14}/>;
}

/**
 * Renders one thread reply or push post from the post's props.
 * @param props - the Mattermost post
 * @returns the event row, or the markdown message when the props are missing
 */
export default function EventPost({post}: {post: Post}) {
    const ev = post.props?.ghc_event as EventReply | undefined;
    if (!ev) {
        return <span>{post.message}</span>;
    }
    const isCI = ev.kind === 'ci_failed' || ev.kind === 'ci_passed';
    const hidden = (ev.total_commits || 0) - (ev.commits?.length || 0);

    return (
        <div
            className={`ghc-event ghc-tone-${ev.tone}`}
            data-testid='ghc-event'
        >
            <div className='ghc-event-head'>
                <span className='ghc-event-badge'><ToneIcon tone={ev.tone}/></span>
                {isCI ? (
                    <span className='ghc-event-line'>
                        <span className='ghc-strong'>{ev.kind === 'ci_failed' ? 'CI failed' : 'CI passing again'}</span>
                        {ev.context && <span className='ghc-event-context'><Markdown text={ev.context}/></span>}
                    </span>
                ) : (
                    <span className='ghc-event-line'>
                        <Avatar
                            person={ev.actor}
                            size={18}
                        />
                        <span className='ghc-strong'>{ev.actor.login}</span>
                        <span className='ghc-event-verb'><Markdown text={ev.verb}/></span>
                        {ev.context && <span className='ghc-event-context'><Markdown text={ev.context}/></span>}
                        {ev.repo && (
                            <span>
                                {'in '}
                                <ExtLink
                                    href={ev.repo_url}
                                >
                                    {ev.repo}
                                </ExtLink>
                            </span>
                        )}
                    </span>
                )}
                {ev.url && (
                    <ExtLink
                        className='ghc-event-link'
                        href={ev.url}
                        title='Open on GitHub'
                    >
                        {ev.kind === 'push' ? 'Compare' : 'View'}
                        <ExternalIcon size={12}/>
                    </ExtLink>
                )}
            </div>
            {ev.body && ev.body.trim() !== '' && (
                <div className='ghc-event-body'><Markdown text={ev.body}/></div>
            )}
            {ev.commits && ev.commits.length > 0 && (
                <ul className='ghc-commits'>
                    {ev.commits.map((c) => (
                        <li key={c.sha}>
                            <ExtLink
                                className='ghc-sha'
                                href={c.url}
                            >
                                {c.sha.slice(0, 7)}
                            </ExtLink>
                            <span className='ghc-commit-msg'>{c.message}</span>
                            <span className='ghc-muted'>{c.author}</span>
                        </li>
                    ))}
                    {hidden > 0 && <li className='ghc-muted'>{`and ${hidden} more`}</li>}
                </ul>
            )}
        </div>
    );
}
