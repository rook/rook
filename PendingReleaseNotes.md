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

## Breaking Changes

- Rook no longer supports Kubernetes `1.8` and `1.9`.
- Rook no longer supports running more than one monitor on the same node when `hostNetwork` and `allowMultiplePerNode` are `true`.

### Ceph

- Rook will no longer create a directory-based osd in the `dataDirHostPath` if no directories or
  devices are specified or if there are no disks on the host.
- Containers in `mon` and `mgr` pods have been removed and/or changed names.
- Config paths in `mon` and `mgr` containers are now always the Ceph default paths
  (`/etc/ceph`, `/var/lib/ceph/...`) regardless of the `dataDirHostPath` setting.

## Known Issues

## Deprecations
