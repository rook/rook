# Major Themes

## Action Required

## Notable Features


### Ceph

- The job for detecting the Ceph version can be started with node affinity or tolerations according to the same settings in the Cluster CR as the mons.
- A new CR property `skipUpgradeChecks` has been added, which allows you force an upgrade by skipping daemon checks. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](Documentation/ceph-upgrade.html#ceph-version-upgrades).
- Ceph OSD's admin socket is now placed under Ceph's default system location `/run/ceph`.

### EdgeFS


### YugabyteDB



## Breaking Changes

### <Storage Provider>


## Known Issues

### <Storage Provider>


## Deprecations

### <Storage Provider>
