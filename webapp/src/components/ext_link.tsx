import React from 'react';

/**
 * A link that opens GitHub in a new tab. Mattermost's ExternalLink is not exported to plugins.
 * @param props - anchor attributes
 * @returns the anchor
 */
export function ExtLink(props: React.AnchorHTMLAttributes<HTMLAnchorElement>) {
    return (

        // eslint-disable-next-line @mattermost/use-external-link
        <a
            {...props}
            target='_blank'
            rel='noopener noreferrer'
        />
    );
}
