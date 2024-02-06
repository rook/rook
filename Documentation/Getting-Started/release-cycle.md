# Release Cycle

Rook plans to release a new minor version three times a year, or about every four months.

The most recent two minor Rook releases are actively maintained.

Patch releases for the latest minor release are typically bi-weekly.
Urgent patches may be released sooner.

Patch releases for the previous minor release are commonly monthly, though will vary
depending on the urgency of fixes.

## Definition of Maintenance

The Rook community defines maintenance in that relevant bug fixes that are merged to the main
development branch will be eligible to be back-ported to the release branch of any currently
maintained version. Patches will be released as needed. It is also possible that a fix may
be merged directly to the release branch if no longer applicable on the main development branch.

While Rook maintainers make significant efforts to release urgent issues in a timely manner,
maintenance does not indicate any SLA on response time.

## K8s Versions

The minimum version supported by a Rook release is specified in the
[Quickstart Guide](quickstart.md#minimum-version).

Rook expects to support the most recent six versions of Kubernetes. While these K8s
versions may not all be supported by the K8s release cycle, we understand that
clusters may take time to update.
