# Major Themes

## Action Required

## Notable Features

### Minio Object Stores

    - Now have an additional label named `objectstore` with the name of the Object Store, to allow better selection for Services.
    - Use `Readiness` and `Liveness` probes.
    - Updated automatically on Object Store CRD changes.

### Ceph

- Ceph Nautilus (`v14`) is now supported by Rook.
- Ceph status is surfaced in the CephCluster CR and periodically updated by the operator (default is 60s). The interval can be configured with the `ROOK_CEPH_STATUS_CHECK_INTERVAL` env var.
- A `CephNFS` CRD will start NFS daemon(s) for exporting CephFS volumes or RGW buckets. See the [NFS documentation](Documentation/ceph-nfs-crd.md).
- Selinux labeling for mounts can now be toggled with the [ROOK_ENABLE_SELINUX_RELABELING](https://github.com/rook/rook/issues/2417) environment variable.
- Recursive chown for mounts can now be toggled with the [ROOK_ENABLE_FSGROUP](https://github.com/rook/rook/issues/2254) environment variable.
- Added the dashboard `port` configuration setting.
- Added the dashboard `ssl` configuration setting.
- Added Ceph CSI driver deployments on Kubernetes 1.13 and above.
- The number of mons can be increased automatically when new nodes come online. See the [preferredCount](https://rook.io/docs/rook/master/ceph-cluster-crd.html#mon-settings) setting in the cluster CRD documentation.
- New Kubernetes nodes or nodes which are not tainted `NoSchedule` anymore get added automatically to the existing rook cluster if `useAllNodes` is set. [Issue #2208](https://github.com/rook/rook/issues/2208)
- Pod's logs can be written on the filesystem as of Ceph Nautilus 14.2.1 on demand (see [common issues](https://rook.io/docs/rook/master/common-issues.html#activate-ceph-log-on-file))
- Orchestration is automatically triggered when addition or removal of storage
  devices is detected. This should remove the requirement of restarting the
  operator to detect new devices.
- Rook will now set `noout` on the CephClusters that have osd Nodes tainted `NoSchedule`
- `rook-version` and `ceph-version` labels are now applied to Ceph daemon Deployments, DaemonSets,
  Jobs, and StatefulSets. These identify the Rook version which last modified the resource and the
  Ceph version which Rook has detected in the pod(s) being run by the resource.

## Breaking Changes

- Rook no longer supports Kubernetes `1.8` and `1.9`.
- Rook no longer supports running more than one monitor on the same node when `hostNetwork` and `allowMultiplePerNode` are `true`.
- Rook Operator switches from Extensions v1beta1 to use Apps v1 API for DaemonSet and Deployment.

### Ceph

- The example operator and CRD yaml files have been refactored to simplify configuration. See the [examples help topic](Documentation/ceph-examples.md) for more details.
   - The common resources are now factored into `common.yaml` from `operator.yaml` and `cluster.yaml`.
   - `common.yaml`: Creates the namespace, RBAC, CRD definitions, and other common operator and cluster resources
   - `operator.yaml`: Only contains the operator deployment
   - `cluster.yaml`: Only contains the cluster CRD
   - Multiple examples of the operator and CRDs are provided for common usage of the operator and CRDs.
   - By default, a single namespace (`rook-ceph`) is configured instead of two namespaces (`rook-ceph-system` and `rook-ceph`). New and upgraded clusters can still be configured with the operator and cluster in two separate namespaces.
- Rook will no longer create a directory-based osd in the `dataDirHostPath` if no directories or
  devices are specified or if there are no disks on the host.
- Containers in `mon`, `mgr`, `mds`, `rgw`, and `rbd-mirror` pods have been removed and/or changed names.
- Config paths in `mon`, `mgr`, `mds` and `rgw` containers are now always under
  `/etc/ceph` or `/var/lib/ceph` and as close to Ceph's default path as possible regardless of the
  `dataDirHostPath` setting.
- The `rbd-mirror` pod labels now read `rbd-mirror` instead of `rbdmirror` for consistency.
- The operator will no longer remove osds from specified nodes when the node is tainted with
  [automatic Kubernetes taints](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/#taint-based-evictions)
  Osds can still be removed by more explicit methods. See the "Node Settings" section of the
  [Ceph Cluster CRD documentation](Documentation/ceph-cluster-crd.md#node-settings) for full details.

## Known Issues

## Deprecations
