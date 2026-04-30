---
title: Issue Sections
description: Configure issue sections in gh-dash.
---

Issue sections display filtered lists of GitHub issues. They are configured under the
`issuesSections` key in your config file.

## Example

```yaml
issuesSections:
  - title: My Issues
    filters: is:open assignee:@me
  - title: Needs Triage
    filters: is:open label:needs-triage
```

Each section accepts any [GitHub search query](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
under `filters`.
