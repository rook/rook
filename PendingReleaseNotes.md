# Major Themes

## Action Required

## Notable Features

- Added K8s 1.16 to the test matrix and removed K8s 1.11 from the test matrix.
- Discover daemon:
  - When the Storage Operator is deleted, the Discover Daemon will also be deleted, as well as its Config Map
  - Device filtering is now configurable for the user by adding an environment variable
    - A new environment variable `DISCOVER_DAEMON_UDEV_BLACKLIST` is added through which the user can blacklist the devices
    - If no device is specified, the default values will be used to blacklist the devices

### Ceph

- The job for detecting the Ceph version can be started with node affinity or tolerations according to the same settings in the Cluster CR as the mons.
- A new CR property `skipUpgradeChecks` has been added, which allows you force an upgrade by skipping daemon checks. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](Documentation/ceph-upgrade.html#ceph-version-upgrades).
- Mon Quorum Disaster Recovery guide has been updated to work with the latest Rook and Ceph release.
- A new CRD property `PreservePoolsOnDelete` has been added to Filesystem(fs) and Object Store(os) resources in order to increase protection against data loss. if it is set to `true`, associated pools won't be deleted when the main resource(fs/os) is deleted. Creating again the deleted fs/os with the same name will reuse the preserved pools.
- A new ceph-crashcollector controller has been added. These new deployments will run on any node where a Ceph pod is running. Read more about this in the [doc](Documentation/ceph-cluster-crd.html#cluster-wide-resources-configuration-settings)
- PriorityClassNames can now be added to the Rook/Ceph components to influence the scheduler's pod preemption.
  - mgr/mon/osd/rbdmirror: [priority class names configuration settings](Documentation/ceph-cluster-crd.md#priority-class-names-configuration-settings)
  - filesystem: [metadata server settings](Documentation/ceph-filesystem-crd.md#metadata-server-settings)
  - rgw: [gateway settings](Documentation/ceph-object-store-crd.md#gateway-settings)
  - nfs: [samples](Documentation/ceph-nfs-crd.md#samples)
- When the operator is upgraded, the mgr and OSDs (not running on PVC) won't be restarted if the Rook binary version changes
- Rook is now able to create and manage Ceph clients [client crd](Documentation/ceph-client-crd.html).
- The `Status.Phase` property has been introduced for Rook-Ceph CRDs. The current possible values of status are `Processing`, `Ready` and `Failed`. If the operator is performing any task regarding a Ceph related CR, its status will be reflected as `Processing`. The Status will be changed to `Failed` if the operator fails at some task related to the CR and will change to `Ready` once the Rook-Ceph operator finishes all the tasks related to the CR.
- OSDs:
  - Rook will no longer automatically remove OSDs if nodes are removed from the cluster CR to avoid the risk of destroying OSDs unintentionally.
To remove OSDs manually, see the new doc on [OSD Management](Documentation/ceph-osd-mgmt.md)
  - Ceph OSD's admin socket is now placed under Ceph's default system location `/run/ceph`.
  - The on-host log directory for OSDs was set incorrectly to `<dataDirHostPath>/<namespace>/log`;
    fix this to be `<dataDirHostPath>/log/<namespace>`, the same as other daemons.
  - Do not generate a config (during pod init) for directory-based or legacy filestore OSDs
  - Add a new CRD property `devicePathFilter` to support device filtering with path names, e.g. `/dev/disk/by-path/pci-.*-sas-.*`.
  - Support PersistentVolume backed by LVM Logical Volume for "OSD on PVC".
  - Creation of new `Filestore` OSDs on disks is now deprecated. `Filestore` is in sustaining mode in Ceph.
    - The `storeType` storage config setting is now ignored
      - New OSDs created in directories are always `Filestore` type
      - New OSDs created on disks are always `Bluestore` type
    - Preexisting disks provisioned as `Filestore` OSDs will remain as `Filestore` OSDs
  - When running on PVC, the OSD can be on a slow device class, Rook can adapt to that by tuning the OSD. This can be enabled by the CR setting `tuneSlowDeviceClass`
- RGWs:
  - Ceph Object Gateway are automatically configured to not run on the same host if hostNetwork is activated
- New CR property available in the Operator: `ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS` (5 seconds by default). Represents the time to wait until the node controller will move Rook pods to other nodes after detecting an unreachable node. Pods affected by this setting are: mgr, rbd, mds, rgw, nfs, PVC based mons and osds, and ceph toolbox. The value used in this variable replaces the default value of 300 seconds added automatically by k8s as Pod Toleration for `node.kubernetes.io/unreachable`.
Now the total amount of time to reschedule Rook pods in healthy nodes before detecting a `not ready node` condition will be the sum of `node-monitor-grace-period` (k8s kube-controller-manager flag, 40 seconds by default) and `ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS` (5 seconds by default)
- New CR property `continueUpgradeAfterChecksEvenIfNotHealthy`, if set to 'true', Rook will continue to upgrade OSDs even if PGs are not clean and MDS daemons even if the file system is not healthy.

### EdgeFS

- Rook EdgeFS operator adds support for single node, single device deployments. This is to enable embedded and remote developer use cases.
- Support for new EdgeFS backend, rtkvs, enables ability to operate on top of any key-value capable interface. Initial integration adds support for Samsung KV-SSD devices.
- Enhanced support for running EdgeFS in the AWS cloud. It is now possible to store data payload chunks directly in AWS S3 buckets, thus greatly reducing storage billing cost. Metadata chunks still will be in AWS EBS, thus provide low-latency and high-performance.
- It is now possible to configure ISGW Full-Mesh functionality without the need to create multiple ISGW services. Please read more about ISGW Full-Mesh functionality [here](http://highpeakdata.com).
- EdgeFS now capable of creating instant snapshots of S3 buckets. It supports billion of objects per-bucket use cases. A snapshot's metadata gets distributed among all the connected EdgeFS segments, such that cloning or accessing of snapshotted objects can be done without the need of full-delta transferring, i.e. on-demand.

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
