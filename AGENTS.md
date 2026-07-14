# AGENTS.md

Guidance for AI coding agents working in this repository.

This file is a map, not a rulebook. The contributor documentation is the authority, and everything
below points into it rather than restating it, so that the two cannot disagree.

Read the [AI guidelines](Documentation/Contributing/ai-guidelines.md) first. Rook does not accept
contributions that lack human oversight.

## Where the rules live

Read the relevant section of the
[developer guide](Documentation/Contributing/development-flow.md) before making a change:

| To find out how to | Read |
| ------------------ | ---- |
| build, test, and lint, and what CI enforces | [Checks that gate a pull request](Documentation/Contributing/development-flow.md#checks-that-gate-a-pull-request) |
| change anything under `pkg/apis` | [Generated Code and CRDs](Documentation/Contributing/development-flow.md#generated-code-and-crds) |
| write a commit message | [Commit structure](Documentation/Contributing/development-flow.md#commit-structure) |
| fill in the pull request checklist | [the pull request template](.github/PULL_REQUEST_TEMPLATE.md), which says what each item claims |
| write tests | [Assertions](Documentation/Contributing/rook-test-framework.md#assertions) |

Two of these are easy to miss. Building or testing with a bare `go` command does not necessarily
match what CI runs, so use `make`. Changing a godoc comment under `pkg/apis` counts as changing the
API, because those comments become the CRD descriptions.

## Regardless of what else is read

* Sign off every commit with `git commit -s`. A bot enforces the DCO.
* Do not add `co-authored-by`, `assisted-by`, `generated-by`, or similar AI attribution trailers.
    Human sign-off is the mechanism Rook uses.
* Open pull requests against `master`, from a branch in a fork. Rebase onto upstream; never merge
    upstream into a branch.
* Opening a pull request non-interactively, such as by passing a body to `gh pr create`, skips the
    template that GitHub would otherwise supply. Read
    [the template](.github/PULL_REQUEST_TEMPLATE.md) and fill in its checklist regardless.
* Disclose in the pull request description how AI tools assisted with the change.
* Commit messages and review comments are the contributor's own reasoning to explain and own. Do
    not submit raw model output as either.
* Discuss significant changes in an issue before writing code. Large AI-generated pull requests
    without prior design discussion will be rejected.
