# Major Themes

## Action Required

## Notable Features

### Minio Object Stores

    - Now have an additional label named `objectstore` with the name of the Object Store, to allow better selection for Services.
    - Use `Readiness` and `Liveness` probes.
    - Updated automatically on Object Store CRD changes.

### Ceph

- A `CephNFS` CRD will start NFS daemon(s) for exporting CephFS volumes or RGW buckets. See the [NFS documentation](Documentation/ceph-nfs-crd.md).
- Selinux labeling for mounts can now be toggled with the [ROOK_ENABLE_SELINUX_RELABELING](https://github.com/rook/rook/issues/2417) environment variable.
- Recursive chown for mounts can now be toggled with the [ROOK_ENABLE_FSGROUP](https://github.com/rook/rook/issues/2254) environment variable.
- Added the dashboard `port` configuration setting.
- Added the dashboard `ssl` configuration setting.
- Added Ceph CSI driver deployments on Kubernetes 1.13 and above.
- The number of mons can be increased automatically when new nodes come online. See the [preferredCount](https://rook.io/docs/rook/master/ceph-cluster-crd.html#mon-settings) setting in the cluster CRD documentation.
- New Kubernetes nodes or nodes which are not tainted `NoSchedule` anymore get added automatically to the existing rook cluster if `useAllNodes` is set. [Issue #2208](https://github.com/rook/rook/issues/2208)
- Specific devices for OSDs can now be specified using the full udev path (e.g. /dev/disk/by-id/ata-ST4000DM004-XXXX) instead of the device name


## Breaking Changes

- Rook no longer supports Kubernetes `1.8` and `1.9`.
- Rook no longer supports running more than one monitor on the same node when `hostNetwork` and `allowMultiplePerNode` are `true`.
- Rook Operator switches from Extensions v1beta1 to use Apps v1 API for DaemonSet and Deployment.

### Ceph

- Rook will no longer create a directory-based osd in the `dataDirHostPath` if no directories or
  devices are specified or if there are no disks on the host.
- Containers in `mon`, `mgr`, `mds`, `rgw`, and `rbd-mirror` pods have been removed and/or changed names.
- Config paths in `mon`, `mgr`, `mds` and `rgw` containers are now always under
  `/etc/ceph` or `/var/lib/ceph` and as close to Ceph's default path as possible regardless of the
  `dataDirHostPath` setting.
- The `rbd-mirror` pod labels now read `rbd-mirror` instead of `rbdmirror` for consistency.

## Known Issues

## Deprecations
