# Maintainers Rules

This document lays out some basic rules and guidelines all maintainers are expected to follow.
Changes to the Acceptance Criteria for merging PRs require a ceiling(two-thirds) supermajority from the maintainers.

## Hard Acceptance Criteria for merging a PR:

- 1 LGTM from a maintainer is required when merging a PR
- All checks must be green and if [skip ci] has been used there is justification.

## Process for becoming a maintainer:

- Invitation is proposed by an existing maintainer.
- Ceiling(two-thirds) supermajority approval from existing maintainers (including vote of proposing maintainer) required to accept proposal.
- Newly approved maintainer submits PR adding themselves to the MAINTAINERS file.
- Existing maintainers publicly mark their approval on the PR.
- Existing maintainer updates repository permissions to grant write access to new maintainer.
- New maintainer merges their PR.

## Removing maintainers

It is preferrable that a maintainer gracefully removes themselves from the MAINTAINERS file if they are
aware they will no longer have the time or motivation to contribute to the project. Maintainers that
have been inactive in the repo for a period of at least one year should be contacted to ask if they
wish to be removed.

In the case that an inactive maintainer is unresponsive for any reason, a ceiling(two-thirds) supermajority
vote of the existing maintainers can be used to approve their removal from the MAINTAINERS file, and revoke
their merge permissions on the repository.
