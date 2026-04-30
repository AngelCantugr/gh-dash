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
