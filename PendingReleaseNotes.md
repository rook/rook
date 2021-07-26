# Major Themes

v1.7...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph

- The Operator configuration option `ROOK_ALLOW_MULTIPLE_FILESYSTEMS` has been removed in favor of simply verifying the Ceph version is at least Pacific.
Multiple filesystems are stable since Ceph Pacific.
So users who had `ROOK_ALLOW_MULTIPLE_FILESYSTEMS` enabled will need to update their Ceph version to Pacific.

## Features

### Core

### Ceph

- Official Ceph images have moved from docker.io to quay.io. Users running tags like `v14.2`, `v15.2`, `v16.2` must change the registry URL.
So the CephCLuster spec field `image` must be updated to point to quay, like `image: quay.io/ceph/ceph:v16.2`.
- Add user data protection when deleting Rook-Ceph Custom Resources
  - A CephCluster will not be deleted if there are any other Rook-Ceph Custom resources referencing
    it with the assumption that they are using the underlying Ceph cluster.
  - A CephObjectStore will not be deleted if there is a bucket present. In addition to protection
    from deletion when users have data in the store, this implicitly protects these resources from
    being deleted when there is a referencing ObjectBucketClaim present.
  - See [the design](https://github.com/rook/rook/blob/master/design/ceph/resource-dependencies.md)
    for detailed information.
- Add support for creating Hybrid Storage Pools
  - Hybrid storage pool helps to create hybrid crush rule for choosing primary OSD for high performance
    devices (ssd, nvme, etc) and remaining OSD for low performance devices (hdd).
  - See [the design](Documentation/ceph-pool-crd.md#hybrid-storage-pools) for more details.
  - Checkout the [ceph docs](https://docs.ceph.com/en/latest/rados/operations/crush-map/#custom-crush-rules)
    for detailed information.
- Add support cephfs mirroring peer configuration, refer to the [configuration](Documentation/ceph-filesystem-crd.md#mirroring) for more details
- Add support for Kubernetes TLS secret for referring TLS certs needed for ceph RGW server.
- Stretch clusters are considered stable
  - Ceph v16.2.5 or greater is required for stretch clusters
- The use of peer secret names in CephRBDMirror is deprecated. Please use CephBlockPool CR to configure peer secret names and import peers. Checkout the `mirroring` section in the CephBlockPool [spec](Documentation/ceph-pool-crd.md#spec) for more details.

### Cassandra

- CRDs converted from v1beta1 to v1
  - Schema is generated from the internal types for more complete validation
  - Minimum K8s version for the v1 CRDs is K8s 1.16

### NFS

- CRDs converted from v1beta1 to v1
  - Schema is generated from the internal types for more complete validation
  - Minimum K8s version for the v1 CRDs is K8s 1.16
