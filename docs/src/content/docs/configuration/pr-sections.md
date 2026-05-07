---
title: PR Sections
description: Configure pull request sections in gh-dash.
---

PR sections display filtered lists of pull requests. They are configured under the
`prSections` key in your config file.

## Example

```yaml
prSections:
  - title: My Open PRs
    filters: is:open author:@me
  - title: Needs Review
    filters: is:open review-requested:@me
```

Each section accepts any [GitHub search query](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
under `filters`.

## Filtering by merge queue

PRs sitting in a [GitHub merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue)
render with a distinct yellow icon and a `Queued` pill. You can also narrow a
section to (or away from) queued PRs with two extra filter tokens:

- `is:queued` — keep only PRs currently in a merge queue.
- `-is:queued` — drop PRs currently in a merge queue.

```yaml
prSections:
  - title: Awaiting Merge Queue
    filters: is:open is:queued author:@me
  - title: Needs Review (excluding queue)
    filters: is:open review-requested:@me -is:queued
```

GitHub's search syntax does not expose merge-queue membership server-side, so
gh-dash applies these tokens client-side after fetching results. The total
count shown in the section title reflects the unfiltered server response and
may exceed the rows actually rendered when one of these tokens is in use.
