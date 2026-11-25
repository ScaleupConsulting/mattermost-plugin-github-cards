import React from 'react';

type IconProps = {size?: number};

/**
 * Wraps stroked 16px icon paths in a consistent svg.
 * @param props - size and the paths to draw
 * @returns the svg element
 */
function Svg({size = 16, children}: IconProps & {children: React.ReactNode}) {
    return (
        <svg
            width={size}
            height={size}
            viewBox='0 0 16 16'
            fill='none'
            stroke='currentColor'
            strokeWidth='1.6'
            strokeLinecap='round'
            strokeLinejoin='round'
            aria-hidden='true'
        >
            {children}
        </svg>
    );
}

/**
 * Pull request: two branches with an arrow back to the base.
 * @param props - size
 * @returns icon
 */
export function PullRequestIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='4'
                cy='3.5'
                r='1.7'
            />
            <circle
                cx='4'
                cy='12.5'
                r='1.7'
            />
            <circle
                cx='12'
                cy='12.5'
                r='1.7'
            />
            <path d='M4 5.2v5.6M12 10.8V6.5a2 2 0 0 0-2-2H7.5M9 3l-1.5 1.5L9 6'/>
        </Svg>
    );
}

/**
 * Merged pull request: a branch joining the base.
 * @param props - size
 * @returns icon
 */
export function MergeIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='4'
                cy='3.5'
                r='1.7'
            />
            <circle
                cx='4'
                cy='12.5'
                r='1.7'
            />
            <circle
                cx='12'
                cy='8'
                r='1.7'
            />
            <path d='M4 5.2v5.6M4 5.2c0 2.2 2 2.8 6.3 2.8'/>
        </Svg>
    );
}

/**
 * Open issue: a ring with a dot.
 * @param props - size
 * @returns icon
 */
export function IssueIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='8'
                cy='8'
                r='6.2'
            />
            <circle
                cx='8'
                cy='8'
                r='1.2'
                fill='currentColor'
            />
        </Svg>
    );
}

/**
 * Closed issue: a ring with a tick.
 * @param props - size
 * @returns icon
 */
export function IssueClosedIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='8'
                cy='8'
                r='6.2'
            />
            <path d='M5.4 8.2l1.8 1.8 3.4-3.6'/>
        </Svg>
    );
}

/**
 * A tick.
 * @param props - size
 * @returns icon
 */
export function CheckIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <path d='M3 8.5l3.2 3.2L13 4.8'/>
        </Svg>
    );
}

/**
 * A cross.
 * @param props - size
 * @returns icon
 */
export function CrossIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <path d='M4 4l8 8M12 4l-8 8'/>
        </Svg>
    );
}

/**
 * Pending: a clock face.
 * @param props - size
 * @returns icon
 */
export function PendingIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='8'
                cy='8'
                r='6'
            />
            <path d='M8 4.8V8l2.2 1.4'/>
        </Svg>
    );
}

/**
 * A speech bubble.
 * @param props - size
 * @returns icon
 */
export function CommentIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <path d='M2.5 3.5h11v7.5H7l-3 2.5V11H2.5z'/>
        </Svg>
    );
}

/**
 * A minus sign, for dismissed or neutral results.
 * @param props - size
 * @returns icon
 */
export function NeutralIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <path d='M4 8h8'/>
        </Svg>
    );
}

/**
 * An arrow out of a box, for external links.
 * @param props - size
 * @returns icon
 */
export function ExternalIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <path d='M9 2.5h4.5V7M13.5 2.5L7.5 8.5M11.5 9.5v3.5h-9v-9H6'/>
        </Svg>
    );
}

/**
 * A commit: a dot on a line.
 * @param props - size
 * @returns icon
 */
export function CommitIcon(props: IconProps) {
    return (
        <Svg {...props}>
            <circle
                cx='8'
                cy='8'
                r='2.6'
            />
            <path d='M1.5 8h3.9M10.6 8h3.9'/>
        </Svg>
    );
}
