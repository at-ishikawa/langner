---
title: "Note ID Identity"
date: 2026-07-21
weight: 12
bookCollapseSection: true
---

# Note ID Identity

Give every vocabulary entry a stable, globally-unique `id` and make that `id` the **primary key** for a word's learning-log series and database identity — instead of the expression string (optionally plus part-of-speech). Because the id is opaque and independent of the spelling, part of speech, and (editable) meaning, it separates *every* kind of collision: two spellings, a noun vs a verb, or two entries with the same spelling **and** the same part of speech but different meanings in one notebook.

- [Product Requirements]({{< relref "requirements" >}})
- [Technical Design]({{< relref "design" >}})
