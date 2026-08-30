# No unverified merges to main

Never **suggest, offer, or perform** merging a PR or branch into `main` (or any shared/protected base) unless the change is **verified**. "CI is green" alone is not verification. "A subagent reported it works" is not verification. Reading the diff is not verification.

## What counts as verified

A change is merge-eligible only when BOTH hold:

1. **CI is green on the exact head commit** that would be merged (not an older commit on the same branch — branches move; check the SHA).
2. **The specific behavior was exercised end-to-end against a realistic input and observed to do the right thing** — i.e. reproduce the reported problem and confirm this change fixes it, or drive the changed code path with data in the *real* shape the user uses (parse a file in their actual format, run the affected quiz flow, hit the RPC). Observing behavior beats asserting from code.

If the change **cannot** be exercised in this environment (e.g. Postgres-/browser-only paths), say so explicitly and treat it as **unverified** — do not suggest merging it.

## Rules

- Do **not** propose a merge to `main` for an unverified branch/PR. If the user asks to merge, verify first; if you cannot verify, state exactly what you could not verify and do **not** recommend the merge.
- Never rely on an unverified subagent claim (or a green CI badge) to call something merge-ready. Independently confirm the actual behavior — subagents have returned empty/garbage/injected results and CI does not cover every path (e.g. no-DB e2e skips).
- Merging to `main` is hard to reverse: it requires BOTH verification **and** an explicit user go-ahead. Absent either, hold.
- When a user reports a runtime error on a branch, that is a signal to **verify** (reproduce against the actual commit + their data), never to reassure or to push the merge. A parse/build/behavior failure the user sees is "unverified/broken until proven otherwise", regardless of CI.

## Worked example — the trigger for this rule

While iterating a feature branch, I offered to merge it into `main` on the strength of green CI and my reading of the structs, while the user was hitting a `cannot unmarshal !!map into string` runtime error. The error turned out to be a stale local checkout / old binary, but I had **not proven that** when I proposed the merge. The correct move (done only after the user pushed back): build the exact head commit and parse a file in the user's real definitions-book shape (`scenes → expressions → examples: [{text, highlight}]`), observe it parse both the object and plain-string forms, and only then state the branch is sound — while still leaving the merge decision to the user. Verify, then let the user decide; never suggest a merge you haven't exercised.
