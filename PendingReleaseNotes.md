# Major Themes

## Action Required

## Notable Features


### Ceph

- The job for detecting the Ceph version can be started with node affinity or tolerations according to the same settings in the Cluster CR as the mons.
- A new CR property `skipUpgradeChecks` has been added, which allows you force an upgrade by skipping daemon checks. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](Documentation/ceph-upgrade.html#ceph-version-upgrades).
- Ceph OSD's admin socket is now placed under Ceph's default system location `/run/ceph`.
- Mon Quorum Disaster Recovery guide has been updated to work with the latest Rook and Ceph release.
- A new CRD property `PreservePoolsOnDelete` has been added to Filesystem(fs) and Object Store(os) resources in order to increase protection against data loss. if it is set to `true`, associated pools won't be deleted when the main resource(fs/os) is deleted. Creating again the deleted fs/os with the same name will reuse the preserved pools.

### EdgeFS


### YugabyteDB



## Breaking Changes

### <Storage Provider>


## Known Issues

### <Storage Provider>


## Deprecations

### <Storage Provider>
