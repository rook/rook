# Major Themes

v1.7...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

### Ceph

## Features

### Core

### Ceph

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
