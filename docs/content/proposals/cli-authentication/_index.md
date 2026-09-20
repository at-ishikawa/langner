---
title: "CLI Authentication (Device Flow)"
weight: 12
bookCollapseSection: true
---

# CLI Authentication (Device Flow)

Authenticate the `langner` CLI without API keys via the OAuth 2.0 Device Authorization Grant: a short-lived (24h) langner-issued access JWT plus a refresh token stored hashed in the DB, reusing the existing Google web sign-in for approval.

- [Design]({{< relref "design" >}})
