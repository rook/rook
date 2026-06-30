# Maintenance and Support

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

## Supported Versions

The following Kubernetes and Ceph versions are supported by each Rook minor release series. For
the current release, the authoritative sources are the
[Prerequisites](Prerequisites/prerequisites.md) (Kubernetes) and the
[Ceph Upgrade](../Upgrade/ceph-upgrade.md#supported-versions) guide (Ceph).

| Rook | Kubernetes | Ceph |
| ---- | ---------- | ---- |
| 1.20 | v1.31 – v1.36 | Squid v19.2.0+, Tentacle v20.2.1+ |
| 1.19 | v1.30 – v1.35 | Squid v19.2.0+, Tentacle v20.2.1+ |
| 1.18 | v1.29 – v1.34 | Reef v18.2.0+, Squid v19.2.0+, Tentacle v20.2.0+ |
| 1.17 | v1.28 – v1.33 | Reef v18.2.0+, Squid v19.2.0+ |
| 1.16 | v1.27 – v1.32 | Reef v18.2.0+, Squid v19.2.0+ |
| 1.15 | v1.26 – v1.31 | Quincy v17.2.0+, Reef v18.2.0+, Squid v19.2.0+ |
| 1.14 | v1.25 – v1.30 | Quincy v17.2.0+, Reef v18.2.0+ |
| 1.13 | v1.23 – v1.29 | Quincy v17.2.0+, Reef v18.2.0+ |
| 1.12 | v1.22 – v1.28 | Pacific v16.2.7+, Quincy v17.2.0+, Reef v18.2.0+ |

### Notes

* The table records the versions each series supported at release. Patch
    releases occasionally extend support; consult the documentation for the
    specific release for the definitive list.
* Only the most recent two minor Rook releases are actively maintained. Older
    series are listed for reference.
* Rook expects to support the most recent six versions of Kubernetes. While
    these K8s versions may not all be supported by the K8s release cycle, we
    understand that clusters may take time to update.
* Ceph Tentacle v20.2.0 is not recommended due to a known data-corruption issue
    when the "read affinity" feature is enabled. Use v20.2.1 or newer. See the
    [Ceph Upgrade](../Upgrade/ceph-upgrade.md#supported-versions) guide for
    details.
