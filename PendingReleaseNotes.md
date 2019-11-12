# Major Themes

## Action Required

## Notable Features
- Added K8s 1.16 to the test matrix and removed K8s 1.11 from the test matrix.

### Ceph

- The job for detecting the Ceph version can be started with node affinity or tolerations according to the same settings in the Cluster CR as the mons.
- A new CR property `skipUpgradeChecks` has been added, which allows you force an upgrade by skipping daemon checks. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](Documentation/ceph-upgrade.html#ceph-version-upgrades).
- Mon Quorum Disaster Recovery guide has been updated to work with the latest Rook and Ceph release.
- A new CRD property `PreservePoolsOnDelete` has been added to Filesystem(fs) and Object Store(os) resources in order to increase protection against data loss. if it is set to `true`, associated pools won't be deleted when the main resource(fs/os) is deleted. Creating again the deleted fs/os with the same name will reuse the preserved pools.
- OSDs:
  - Ceph OSD's admin socket is now placed under Ceph's default system location `/run/ceph`.
  - The on-host log directory for OSDs was set incorrectly to `<dataDirHostPath>/<namespace>/log`;
    fix this to be `<dataDirHostPath>/log/<namespace>`, the same as other daemons.
  - Use the mon configuration database for directory-based OSDs, and do not generate a config
- A new ceph-crashcollector controller has been added, that new pod will run on any node where a Ceph pod is running. Read more about this in the [doc](Documentation/ceph-cluster-crd.html#cluster-wide-resources-configuration-settings)


### EdgeFS


### YugabyteDB



## Breaking Changes

### Ceph
- The `topology` setting has been removed from the CephCluster CR. To configure the OSD topology, node labels must be applied.
See the [OSD topology topic](ceph-cluster-crd.md#osd-topology). This setting only affects OSDs when they are first created, thus OSDs will not be impacted during upgrade.
The topology settings only apply to bluestore OSDs on raw devices. The topology labels are not applied to directory-based OSDs.


## Known Issues

### <Storage Provider>


## Deprecations

### <Storage Provider>
