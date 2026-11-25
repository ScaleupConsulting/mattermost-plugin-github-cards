import React from 'react';

import type {Post} from '@mattermost/types/posts';

import {ExtLink} from './ext_link';
import {CheckIcon, CommentIcon, CrossIcon, ExternalIcon, IssueClosedIcon, IssueIcon, MergeIcon, NeutralIcon, PendingIcon, PullRequestIcon} from './icons';

import {checkResult, ciResult, labelTextColor, plural, reviewerChips, stateMeta} from '../format';
import type {ReviewerChip} from '../format';
import type {Card, Check, Person} from '../types';

/**
 * Picks the card's header icon for its kind and state.
 * @param card - the card
 * @returns icon element
 */
function KindIcon({card}: {card: Card}) {
    if (card.kind === 'issue') {
        return card.state === 'open' ? <IssueIcon size={18}/> : <IssueClosedIcon size={18}/>;
    }
    return card.state === 'merged' ? <MergeIcon size={18}/> : <PullRequestIcon size={18}/>;
}

/**
 * A GitHub avatar, falling back to the first letter when there is no image.
 * @param props - the person and pixel size
 * @returns avatar element
 */
export function Avatar({person, size = 20}: {person: Person; size?: number}) {
    if (person.avatar_url && !person.is_team) {
        return (
            <img
                className='ghc-avatar'
                src={person.avatar_url}
                width={size}
                height={size}
                alt=''
            />
        );
    }
    return (
        <span
            className='ghc-avatar ghc-avatar-letter'
            style={{width: size, height: size, lineHeight: `${size}px`}}
        >
            {person.login.slice(0, 1).toUpperCase()}
        </span>
    );
}

const REVIEW_LABEL: Record<string, string> = {
    approved: 'Approved',
    changes_requested: 'Changes requested',
    commented: 'Commented',
    dismissed: 'Dismissed',
    requested: 'Awaiting review',
};

/**
 * The small status icon on a reviewer or check chip.
 * @param props - the review state or check result
 * @returns icon element
 */
function StatusIcon({state}: {state: string}) {
    switch (state) {
    case 'approved':
    case 'success':
        return <CheckIcon size={12}/>;
    case 'changes_requested':
    case 'failure':
        return <CrossIcon size={12}/>;
    case 'commented':
        return <CommentIcon size={12}/>;
    case 'requested':
    case 'pending':
        return <PendingIcon size={12}/>;
    }
    return <NeutralIcon size={12}/>;
}

/**
 * One reviewer with their decision.
 * @param props - the chip
 * @returns chip element
 */
function Reviewer({chip}: {chip: ReviewerChip}) {
    const name = chip.person.is_team ? `@${chip.person.login} team` : chip.person.login;
    return (
        <ExtLink
            className={`ghc-chip ghc-tone-${chip.state}`}
            href={chip.url || chip.person.url}
            title={`${name}: ${REVIEW_LABEL[chip.state] || chip.state}`}
        >
            <Avatar person={chip.person}/>
            <span className='ghc-chip-name'>{name}</span>
            <span className='ghc-chip-status'>
                <StatusIcon state={chip.state}/>
                {REVIEW_LABEL[chip.state] || chip.state}
            </span>
        </ExtLink>
    );
}

/**
 * One workflow run for the head commit.
 * @param props - the check
 * @returns chip element
 */
function CheckChip({check}: {check: Check}) {
    const result = checkResult(check);
    return (
        <ExtLink
            className={`ghc-chip ghc-tone-${result}`}
            href={check.url}
            title={`${check.name}: ${check.status === 'completed' ? check.conclusion : check.status.replace('_', ' ')}`}
        >
            <span className='ghc-chip-status'><StatusIcon state={result}/></span>
            <span className='ghc-chip-name'>{check.name}</span>
        </ExtLink>
    );
}

const CI_LABEL: Record<string, string> = {success: 'Checks passing', failure: 'Checks failing', pending: 'Checks running'};

/**
 * Renders a pull request or issue card from the post's props.
 * @param props - the Mattermost post
 * @returns the card, or the markdown message when the props are missing
 */
export default function CardPost({post}: {post: Post}) {
    const card = post.props?.ghc_card as Card | undefined;
    if (!card) {
        return <span>{post.message}</span>;
    }
    const meta = stateMeta(card.kind, card.state);
    const isPR = card.kind === 'pr';
    const chips = isPR ? reviewerChips(card) : [];
    const ci = isPR ? ciResult(card.checks) : '';

    return (
        <div
            className='ghc-card'
            style={{borderLeftColor: meta.color}}
            data-testid='ghc-card'
        >
            <div className='ghc-header'>
                <span
                    className='ghc-kind'
                    style={{color: meta.color}}
                >
                    <KindIcon card={card}/>
                </span>
                <ExtLink
                    className='ghc-repo'
                    href={card.repo_url}
                >
                    {card.repo}
                </ExtLink>
                <span className='ghc-number'>{`#${card.number}`}</span>
                <span
                    className='ghc-state'
                    style={{backgroundColor: meta.color}}
                >
                    {meta.label}
                </span>
            </div>

            <ExtLink
                className='ghc-title'
                href={card.url}
            >
                {card.title}
            </ExtLink>

            <div className='ghc-byline'>
                <Avatar
                    person={card.author}
                    size={18}
                />
                <span className='ghc-strong'>{card.author.login}</span>
                {isPR && card.head && (
                    <span>
                        {card.state === 'merged' ? ' merged ' : ' wants to merge '}
                        <code className='ghc-branch'>{card.head}</code>
                        {' → '}
                        <code className='ghc-branch'>{card.base}</code>
                    </span>
                )}
                {!isPR && <span>{' opened this issue'}</span>}
                {isPR && Boolean(card.changed_files) && (
                    <span className='ghc-diff'>
                        <span className='ghc-add'>{`+${card.additions}`}</span>
                        <span className='ghc-del'>{`−${card.deletions}`}</span>
                        <span className='ghc-muted'>{plural(card.changed_files || 0, 'file', 'files')}</span>
                        {Boolean(card.commits) && <span className='ghc-muted'>{plural(card.commits || 0, 'commit', 'commits')}</span>}
                    </span>
                )}
            </div>

            {chips.length > 0 && (
                <div className='ghc-row'>
                    <span className='ghc-row-label'>{'Reviews'}</span>
                    <div className='ghc-chips'>
                        {chips.map((c) => (
                            <Reviewer
                                key={c.person.login}
                                chip={c}
                            />
                        ))}
                    </div>
                </div>
            )}

            {ci && (
                <div className='ghc-row'>
                    <span className='ghc-row-label'>{'CI'}</span>
                    <div className='ghc-chips'>
                        <span className={`ghc-ci ghc-tone-${ci}`}>
                            <StatusIcon state={ci}/>
                            {CI_LABEL[ci]}
                            {card.head_sha && <code>{card.head_sha.slice(0, 7)}</code>}
                        </span>
                        {(card.checks || []).map((c) => (
                            <CheckChip
                                key={c.workflow_id}
                                check={c}
                            />
                        ))}
                    </div>
                </div>
            )}

            {((card.labels?.length ?? 0) > 0 || (card.assignees?.length ?? 0) > 0) && (
                <div className='ghc-row'>
                    <span className='ghc-row-label'>{'Labels'}</span>
                    <div className='ghc-chips'>
                        {(card.labels || []).map((l) => (
                            <span
                                key={l.name}
                                className='ghc-label'
                                style={{backgroundColor: `#${l.color}`, color: labelTextColor(l.color)}}
                            >
                                {l.name}
                            </span>
                        ))}
                        {(card.assignees?.length ?? 0) > 0 && (
                            <span className='ghc-assignees'>
                                <span className='ghc-muted'>{'Assigned'}</span>
                                {(card.assignees || []).map((a) => (
                                    <span
                                        key={a.login}
                                        className='ghc-assignee'
                                        title={a.login}
                                    >
                                        <Avatar person={a}/>
                                        {a.login}
                                    </span>
                                ))}
                            </span>
                        )}
                    </div>
                </div>
            )}

            {((card.closes_issues?.length ?? 0) > 0 || (card.linked_prs?.length ?? 0) > 0) && (
                <div className='ghc-row'>
                    <span className='ghc-row-label'>{isPR ? 'Closes' : 'Fixed by'}</span>
                    <div className='ghc-chips'>
                        {(card.closes_issues || []).map((n) => (
                            <ExtLink
                                key={n}
                                className='ghc-link-chip'
                                href={`${card.repo_url}/issues/${n}`}
                            >
                                <IssueIcon size={12}/>
                                {`#${n}`}
                            </ExtLink>
                        ))}
                        {(card.linked_prs || []).map((pr) => (
                            <ExtLink
                                key={pr.number}
                                className='ghc-link-chip'
                                href={pr.url}
                                style={{color: stateMeta('pr', pr.state).color}}
                            >
                                {pr.state === 'merged' ? <MergeIcon size={12}/> : <PullRequestIcon size={12}/>}
                                {`#${pr.number} ${pr.title}`}
                            </ExtLink>
                        ))}
                    </div>
                </div>
            )}

            <div className='ghc-footer'>
                <ExtLink
                    className='ghc-button'
                    href={card.url}
                >
                    {'Open on GitHub'}
                    <ExternalIcon size={13}/>
                </ExtLink>
                {card.comments > 0 && (
                    <span className='ghc-muted ghc-comments'>
                        <CommentIcon size={13}/>
                        {plural(card.comments, 'comment', 'comments')}
                    </span>
                )}
            </div>
        </div>
    );
}
