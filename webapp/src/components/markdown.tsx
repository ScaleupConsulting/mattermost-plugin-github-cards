import React from 'react';

type PostUtils = {
    formatText: (text: string, options?: Record<string, unknown>) => string;
    messageHtmlToComponent: (html: string, options?: Record<string, unknown>) => React.ReactNode;
};

/**
 * Renders GitHub markdown with Mattermost's own formatter, so bodies look like any other post.
 * Mentions are not linked, since a GitHub login is not a Mattermost user.
 * @param props - the markdown text
 * @returns the rendered text, or plain text when the formatter is unavailable
 */
export function Markdown({text}: {text: string}) {
    const utils = (window as unknown as {PostUtils?: PostUtils}).PostUtils;
    if (!utils) {
        return <span className='ghc-plain'>{text}</span>;
    }
    const html = utils.formatText(text, {atMentions: false, mentionHighlight: false, markdown: true});
    return <>{utils.messageHtmlToComponent(html, {mentionHighlight: false})}</>;
}
