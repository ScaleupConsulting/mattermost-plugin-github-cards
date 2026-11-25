// Shapes the server stores in post props. Keep in step with server/card.go.

export type Person = {
    login: string;
    avatar_url?: string;
    url?: string;
    is_bot?: boolean;
    is_team?: boolean;
};

export type Label = {name: string; color: string};

export type Review = {reviewer: Person; state: string; url?: string};

export type Check = {
    workflow_id: number;
    name: string;
    run_id: number;
    attempt: number;
    status: string;
    conclusion?: string;
    url: string;
};

export type LinkedPR = {number: number; title: string; state: string; url: string};

export type Card = {
    v: number;
    kind: 'pr' | 'issue';
    repo: string;
    repo_url: string;
    number: number;
    title: string;
    url: string;
    author: Person;
    state: string;
    labels?: Label[];
    assignees?: Person[];
    comments: number;
    head?: string;
    base?: string;
    head_sha?: string;
    additions?: number;
    deletions?: number;
    changed_files?: number;
    commits?: number;
    requested_reviewers?: Person[];
    reviews?: Review[];
    checks?: Check[];
    merged_by?: Person;
    merge_commit_sha?: string;
    closes_issues?: number[];
    linked_prs?: LinkedPR[];
};

export type PushCommit = {sha: string; message: string; url: string; author: string};

export type EventReply = {
    kind: string;
    actor: Person;
    verb: string;
    tone: string;
    context?: string;
    body?: string;
    url?: string;
    repo?: string;
    repo_url?: string;
    commits?: PushCommit[];
    total_commits?: number;
};
