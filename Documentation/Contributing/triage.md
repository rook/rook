---
title: Issue and PR Triage
---

Triage keeps the backlog actionable: every open issue and pull request gets a
clear category, complete information, correct links, and the right people
looking at it. Triage is deliberately shallow — sorting and routing based on
an item's metadata and thread, not code review and not technical support.
Anyone can help triage; write actions (labels, closes) require write access
(reviewers and maintainers — see the Change Approval section of
[CONTRIBUTING.md](https://github.com/rook/rook/blob/master/CONTRIBUTING.md)).

## Principles

* **Prefer linking over closing.** A wrong close loses a real report; a wrong link costs nothing. When unsure, cross-link and leave the item open.
* **Fewer, correct labels.** Label only what is known from the report. Never invent labels; use the existing label set.
* **Comments are rare and purposeful.** At most one triage comment per state change: ask for exactly what is missing, or explain exactly what happens next. Do not debug or answer support questions in the thread — redirect.
* **One targeted ping beats a broadcast.** Mention the one or two people most active in the affected area, and do not re-ping the same person within a week.

## Issue triage

1. **Classify** the kind: bug, feature request, support question, documentation, or project/meta.
2. **Check completeness** against the issue template: Rook/Ceph/Kubernetes versions, reproduction steps, the cluster CR, operator and crashing-pod logs, and `kubectl rook-ceph health` / `ceph status` output. If required information is missing, ask for exactly what is missing and stop — do not categorize an incomplete bug report. A "+1" or "me too" comment is not information.
3. **Label** from the existing label set (component and kind). Keep it to a few labels that are certain.
4. **Find duplicates and cross-links.** Search open and recently closed issues and PRs. Link the issue to a PR that fixes it (and vice versa). Treat two issues as duplicates only when they share the same root cause — the same symptom is not enough.
5. **Route.** Mention one or two maintainers who are most active in the affected component; recent git history for the relevant paths is the best guide.
6. **Set the disposition:**
    * *Actionable* — keep open, labeled and routed.
    * *Needs information* — ask once; the stale bot follows up on prolonged silence.
    * *Fixed by a merged PR* — verify the merged change actually addresses the reported mechanism (not just a similar symptom), note the release that carries the fix, then close with the reference.
    * *Support question* — redirect to [Slack](https://slack.rook.io) or [GitHub Discussions](https://github.com/rook/rook/discussions) and close courteously.
    * *Feature needing design* — suggest a design discussion or design doc rather than leaving a large request implicitly parked.

## PR triage

1. **Skip** draft PRs and PRs labeled `do-not-merge`.
2. **Check the basics:** CI state, merge conflicts (ask for a rebase onto master), and that the PR template checklist honestly matches the diff (documentation, unit tests, integration tests, pending release notes).
3. **Link the issue** the PR resolves, and confirm the linked issue exists and is still open. If the PR fixes a bug with no issue, that is fine — the description must then carry the full context itself.
4. **Read the description against the diff** at metadata depth: does it say what changed and why, and do the commits follow the [commit conventions](development-flow.md#commit-structure)? Correctness is the reviewers' job, not triage's.
5. **Route reviewers.** Request two or three reviewers (one to five is acceptable), always including at least one approver from [CODE-OWNERS](https://github.com/rook/rook/blob/master/CODE-OWNERS). Pick people who actually review that component — recent review history on merged PRs in the same paths is the best signal. Triage does not apply type or component labels to PRs (automation such as dependabot manages its own labels).
6. **Stalled PRs.** If the author is unresponsive to review feedback after a ping and a couple of weeks, a maintainer may adopt the PR (push the needed fixes to it, preserving the author's commits) or supersede it with a new PR that credits the original author; when superseding, close the original with a comment pointing to the replacement.

## Closing discipline

Closing is the highest-risk triage action. Before closing:

* **Duplicate** — confirm the same root cause, then close with a link to the canonical issue.
* **Fixed** — confirm the merged PR addresses the reported mechanism and name the release carrying it.
* **Support / out of scope** — close with the redirect or reasoning stated.

If any of these cannot be confirmed, cross-link instead and leave the item
open.

## Escalation

Suspected security vulnerabilities follow the
[security policy](https://github.com/rook/rook/blob/master/SECURITY.md) —
report privately, never triage in public. Suspected data loss or a
regression in a current release should be raised with the maintainers
directly (Slack) rather than waiting in the queue.

## What triage is not

* **Not code review.** Reviewers own correctness; triage routes to them.
* **Not support.** Redirect support questions; do not debug in the issue.
* **Not staleness management.** The stale bot owns inactivity timelines; items labeled `keepalive`, `security`, or `reliability` are exempt and should not be nudged toward closure by hand.
