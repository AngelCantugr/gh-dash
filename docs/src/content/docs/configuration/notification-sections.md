---
title: Notification Sections
description: Configure notification sections in gh-dash.
---

Notification sections display your GitHub notification inbox. They are configured under the
`notificationsSections` key in your config file.

## Example

```yaml
notificationsSections:
  - title: All Notifications
    filters:
      - reason: subscribed
  - title: Review Requests
    filters:
      - reason: review_requested
```
