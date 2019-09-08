# Major Themes

## Action Required

- With Rook [EdgeFS](http://edgefs.io) operator CRDs graduated to v1 stable (WooHoo!), please follow [upgrade procedure](Documentation/edgefs-upgrade.md) on how to get CRDs and running setups converted.

## Notable Rook Framework Features
- Added K8s 1.15 to the test matrix and removed K8s 1.10 from the test matrix.
- OwnerReferences are created with the fully qualified `apiVersion` such that the references will work properly on OpenShift.
- The integration tests can be triggered for specific storage providers rather than always running all tests. See the [dev guide](INSTALL.md#test-storage-provider) for more details.
- Provisioning will fail if the user specifies a `metadataDevice` but that device is not used as a metadata device by Ceph.
- [YugabyteDB](https://www.yugabyte.com/) is now supported by Rook with a new operator. You can deploy, configure and manage instances of this high-performance distributed SQL database. Create an instance of the new `ybcluster.yugabytedb.rook.io` custom resource to easily deploy a cluster of YugabyteDB Database. Checkout its [user guide](Documentation/yugabytedb.md) to get started with YugabyteDB.
- Rook now supports Multi-Homed networking. This feature significantly improves performance and security by isolating a backend network as a separate network. Read more on Multi-Homed networking with Rook EdgeFS in this [presentation at KubeCon China 2019](https://www.youtube.com/watch?v=h38FCAuOehc&list=PLj6h78yzYM2Njj5PvNc4Mtcril2YyR95d&index=76&t=0s).
- Git tags can be added for alpha, beta, or rc releases. For example: v1.1.0-alpha.0

### Ceph

- Creation of storage pools through the custom resource definitions (CRDs) now allows users to optionally specify `deviceClass` property to enable
distribution of the data only across the specified device class. See [Ceph Block Pool CRD](Documentation/ceph-pool-crd.md#ceph-block-pool-crd) for
an example usage
- Linear disk device can now be used for Ceph OSDs.
- Allow `metadataDevice` to be set per OSD device in the device specific `config` section.
- Added `deviceClass` to the per OSD device specific `config` section for setting custom crush device class per OSD.
- Use `--db-devices` with Ceph 14.2.1 and newer clusters to explicitly set `metadataDevice` per OSD.
- The minimum version supported by Rook is now Ceph Mimic v13.2.4.
- The Ceph CSI driver is enabled by default and preferred over the flex driver
   - The flex driver can be disabled in operator.yaml by setting ROOK_ENABLE_FLEX_DRIVER=false
   - The CSI drivers can be disabled by setting ROOK_CSI_ENABLE_CEPHFS=false and ROOK_CSI_ENABLE_RBD=false
- The device discovery daemon can be disabled in operator.yaml by setting ROOK_ENABLE_DISCOVERY_DAEMON=false
- Rook can now be configured to read "region" and "zone" labels on Kubernetes nodes and use that information as part of the CRUSH location for the OSDs.
- Rgw pods have liveness probe enabled
- Rgw is now configured with the Beast backend as of the Nautilus release
- OSD: newly updated cluster from 0.9 to 1.0.3 and thus Ceph Nautilus will have their OSDs allowing new features for Nautilus
- Rgw instances have their own key and thus are properly reflected in the Ceph status
- The Rook Agent pods are now started when the CephCluster is created rather than immediately when the operator is started.
- Ceph CRUSH tunable are not enforced to "firefly" anymore, Ceph picks the right tunable for its own version, to read more about tunable [see the Ceph documentation](http://docs.ceph.com/docs/master/rados/operations/crush-map/#tunables)
- `NodeAffinity` can be applied to `rook-ceph-agent DaemonSet` with `AGENT_NODE_AFFINITY` environment variable.
- `NodeAffinity` can be applied to `rook-discover DaemonSet` with `DISCOVER_AGENT_NODE_AFFINITY` environment variable.
- Rook does not create an initial CRUSH map anymore and let Ceph do it normally
- Ceph monitor placement will now take failure zones into account [see the
  documentation](Documentation/ceph-advanced-configuration.md#monitor-placement)
  for more information.
- The cluster CRD option to allow multiple monitors to be scheduled on the same
  node---`spec.Mon.AllowMultiplePerNode`---is now active when a cluster is first
  created. Previously, it was ignored when a cluster was first installed.
- The Cluster CRD now provides option to enable prometheus based monitoring, provided that prometheus is pre-installed.
- Upgrades have drastically improved, Rook intelligently checks for each daemon state before and after upgrading. To learn more about the upgrade workflow see [Ceph Upgrades](Documentation/ceph-upgrade.md)
- Rook Operator now supports 2 new environmental variables: `AGENT_TOLERATIONS` and `DISCOVER_TOLERATIONS`. Each accept list of tolerations for agent and discover pods accordingly.
- Ceph daemons now run under 'ceph' user and not 'root' anymore (monitor or osd store already owned by 'root' will keep running under 'root')
- Ceph monitors have initial support for running on PVC storage. See docs on
  [monitor settings for more detail](Documentation/ceph-cluster-crd.md#mon-settings).
- Ceph OSDs can be created by using StorageClassDeviceSet. See docs on [Storage Class Device Sets](Documentation/ceph-cluster-crd.md#storage-class-device-sets).
- Rook can now connect to an external cluster, for more info about external cluster [read the design](https://github.com/rook/rook/blob/master/design/ceph-external-cluster.md) as well as the documentation [Ceph External cluster](Documentation/ceph-cluster-crd.md#external-cluster)
- Added a new property in `storageClassDeviceSets` named `portable`:
   - If `true`, the OSDs will be allowed to move between nodes during failover. This requires a storage class that supports portability (e.g. `aws-ebs`, but not the local storage provisioner).
   - If `false`, the OSDs will be assigned to a node permanently. Rook will configure Ceph's CRUSH map to support the portability.
- Rook can now manage MachineDisruptionBudgets for the OSDs (only available on OpenShift). MachineDisruptionBudgets for OSDs are dynamically managed as documented in the `disruptionManagement` section of the [CephCluster CR](Documentation/ceph-cluster-crd.md##luster-settings). This can be enabled with the `manageMachineDisruptionBudgets` flag in the cluster CR.
- Rook can now manage PodDisruptionBudgets for the following Daemons: OSD, Mon, RGW, MDS. OSD budgets are dynamically managed as documented in the [design](https://github.com/rook/rook/blob/master/design/ceph-managed-disruptionbudgets.md). This can be enabled with the `managePodBudgets` flag in the cluster CR. When this is enabled, drains on OSDs will be blocked by default and dynamically unblocked in a safe manner one failureDomain at a time. When a failure domain is draining, it will be marked as no out for a longer time than the default DOWN/OUT interval.
- Rook now has a new config CRD `mgr` to enable ceph manager modules
- Flexvolume plugin now supports dynamic PVC expansion.
- The Rook-enforced minimum memory for OSD pods has been reduced from 4096M to 2048M

### EdgeFS

- The minimum version supported by Rook is now EdgeFS v1.2.64.
- Graduate CRDs to stable v1 [#3702](https://github.com/rook/rook/issues/3702)
- Added support for useHostLocalTime option to synchronize time in service pods to host [#3627](https://github.com/rook/rook/issues/3627)
- Added support for Multi-homing networking to provide better storage backend security isolation [#3576](https://github.com/rook/rook/issues/3576)
- Allow users to define Kubernetes users to define ServiceType and NodePort via the service CRD spec [#3516](https://github.com/rook/rook/pull/3516)
- Added mgr pod liveness probes [#3492](https://github.com/rook/rook/issues/3492)
- Ability to add/remove nodes via EdgeFS cluster CRD [#3462](https://github.com/rook/rook/issues/3462)
- Support for device full name path spec i.e. /dev/disk/by-id/NAME [#3374](https://github.com/rook/rook/issues/3374)
- Rolling Upgrade support [#2990](https://github.com/rook/rook/issues/2990)
- Prevents multiple targets deployment on the same node  [#3181](https://github.com/rook/rook/issues/3181)
- Enhance S3 compatibility support for S3X pods [#3169](https://github.com/rook/rook/issues/3169)
- Add K8S_NAMESPACE env to EdgeFS containers [#3097](https://github.com/rook/rook/issues/3097)
- Improved support for ISGW dynamicFetch configuring [#3070](https://github.com/rook/rook/issues/3070)
- OLM integration [#3017](https://github.com/rook/rook/issues/3017)
- Flexible Metadata Offload page size setting support [#3776](https://github.com/rook/rook/issues/3776)

### YugabyteDB

- Rook now supports YugabyteDB as storage provider. YugaByteDB is a high-performance, cloud-native distributed SQL database which can tolerate disk, node, zone and region failures automatically. You can find more information about YugabyteDB [here](https://docs.yugabyte.com/latest/introduction/)
- Newly added Rook operator for YugabyteDB lets you easily create a YugabyteDB cluster.
- Please follow Rook YugabyteDB operator [quickstart guide](Documentation/yugabytedb.md) to create a simple YugabyteDB cluster.

## Breaking Changes

### Ceph

- The minimum version supported by Rook is Ceph Mimic v13.2.4. Before upgrading to v1.1 it is required to update the version of Ceph to at least this version.
- The CSI driver is enabled by default. Documentation has been changed significantly for block and filesystem to use the CSI driver instead of flex.
While the flex driver is still supported, it is anticipated to be deprecated soon.
- The `Mon.PreferredCount` setting has been removed.
- imagePullSecrets option added to helm-chart

### EdgeFS

- With Rook EdgeFS operator CRDs graduated to v1, please follow [upgrade procedure](Documentation/edgefs-upgrade.md) on how to get CRDs and running setups converted.
- EdgeFS versions greater than v1.2.62 require full cluster restart.

## Known Issues

### <Storage Provider>

## Deprecations

### Ceph

- For rgw, when deploying an object store with `object.yaml`, using `allNodes` is not supported anymore, a transition path has been implemented in the code though.
So if you were using `allNodes: true`, Rook will replace each daemonset with a deployment (one for one replacement) gradually.
This operation will be triggered on an update or when a new version of the operator is deployed.
Once complete, it is expected that you edit your object CR with `kubectl -n rook-ceph edit cephobjectstore.ceph.rook.io/my-store` and set `allNodes: false` and `instances` with the current number of rgw instances.

### <Storage Provider>
