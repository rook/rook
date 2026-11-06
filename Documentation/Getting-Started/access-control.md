---
title: Access Control
---

# Access Control

Rook's custom resources configure the Ceph cluster and the privileged daemons that run it, so
permission to create or edit them is an administrative grant. Rook currently supports granting them
only to trusted administrators. This page lists each Rook custom resource and each related
permission in the Rook namespaces, and, where it applies, what the permission is equivalent to.

The tables use these terms:

* **Root on the node**: the grant can run code in, or choose the image or host paths of, a container
    that runs privileged or with access to the host.
* **Ceph admin**: the grant can obtain the `client.admin` key, or a key with the same capabilities.
* **Secrets in the namespace**: the grant can read any Secret in the resource's namespace.
* **Operator identity**: the grant can obtain the permissions of the operator's ServiceAccount,
    `rook-ceph-system`, which can read Secrets in every namespace.
* **Administrative**: grant only to trusted administrators.

The cluster namespace is the namespace of the CephCluster. In the default install it is also the
operator namespace.

## Custom resources

| Resource | Equivalent to | Details |
| -------- | ------------- | ------- |
| [CephCluster](../CRDs/Cluster/ceph-cluster-crd.md) | Root on the node, Ceph admin, Operator identity | `cephVersion.image` is the image of the privileged OSD containers, which hold the admin keyring. `dataDirHostPath`, `storage` and `cleanupPolicy` choose the host paths and devices that privileged pods use. The operator itself connects to the key management service that `security.kms` configures; see [Key Management System](../Storage-Configuration/Advanced/key-management-system.md) |
| [CephNFS](../CRDs/ceph-nfs-crd.md) | Root on the node, Ceph admin, Secrets in the namespace | Every `volumeSource` under `security` accepts a writable `hostPath` and any Secret in its namespace, including `rook-ceph-admin-keyring`. `server.image` and `security.sssd.sidecar.image` set the images of the containers that mount those volumes |
| [CephNVMeOFGateway](../Storage-Configuration/Block-Storage-RBD/nvme-of.md) | Root on the node, Ceph admin | `image` runs in a privileged container that holds the admin keyring |
| [CephObjectStore](../CRDs/Object-Storage/ceph-object-store-crd.md) | Root on the node, Ceph admin, Secrets in the namespace, Operator identity | `gateway.additionalVolumeMounts` accepts a writable `hostPath` and any Secret in its namespace. `gateway.rgwConfig`, `gateway.rgwConfigFromSecret` and `gateway.rgwCommandFlags` set the options of the RGW daemon. The operator itself connects to the key management service that `security.kms` configures; see [Key Management System](../Storage-Configuration/Advanced/key-management-system.md) |
| [CephFilesystem](../CRDs/Shared-Filesystem/ceph-filesystem-crd.md) | Administrative | Grant only to trusted administrators |
| [CephClient](../CRDs/ceph-client-crd.md) | Ceph admin | `caps` define the key's access |
| [CephCOSIDriver](../Storage-Configuration/Object-Storage-RGW/cosi.md) | Administrative | Grant only to trusted administrators |
| [CephObjectStoreUser](../CRDs/Object-Storage/ceph-object-store-user-crd.md) | Administrative | Grant only to trusted administrators |
| [ObjectBucketClaim](../Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md) | Administrative | Grant only to trusted administrators |
| ObjectBucket | Administrative | Grant only to trusted administrators |
| [CephBucketTopic](../Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications.md) | Administrative | Grant only to trusted administrators |
| [CephBlockPool](../CRDs/Block-Storage/ceph-block-pool-crd.md) | Administrative | Grant only to trusted administrators |
| [CephBlockPoolRadosNamespace](../CRDs/Block-Storage/ceph-block-pool-rados-namespace-crd.md), [CephFilesystemSubVolumeGroup](../CRDs/Shared-Filesystem/ceph-fs-subvolumegroup-crd.md) | Administrative | Grant only to trusted administrators |
| [CephObjectRealm](../CRDs/Object-Storage/ceph-object-realm-crd.md), [CephObjectZoneGroup](../CRDs/Object-Storage/ceph-object-zonegroup-crd.md), [CephObjectZone](../CRDs/Object-Storage/ceph-object-zone-crd.md) | Administrative | Grant only to trusted administrators |
| [CephFilesystemMirror](../CRDs/Shared-Filesystem/ceph-fs-mirror-crd.md), [CephRBDMirror](../CRDs/Block-Storage/ceph-rbd-mirror-crd.md), [CephObjectStoreAccount](../Storage-Configuration/Object-Storage-RGW/ceph-object-accounts.md), [CephBucketNotification](../Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications.md) | Administrative | Grant only to trusted administrators |

The COSI BucketClass and BucketAccessClass, which Rook does not install, are also administrative:
they name the Secret of the privileged CephObjectStoreUser that the COSI driver uses. The Ceph-CSI
operator's custom resources in the `csi.ceph.io` group, which Rook installs, are also
administrative.

## Namespace permissions

| Permission | Equivalent to | Details |
| ---------- | ------------- | ------- |
| Read Secrets in the cluster namespace | Ceph admin | `rook-ceph-admin-keyring` and `rook-ceph-mon` hold the admin key |
| Write ConfigMaps in the cluster namespace | Root on the node, Ceph admin | The `rook-ceph-osd-env-override` ConfigMap sets environment variables in privileged OSD containers that hold the admin keyring |
| Write the `rook-ceph-operator-config` ConfigMap in the operator namespace | Administrative | Sets the operator's settings, which can change what the other rows grant |
| Create pods, or workloads that create pods, or use `pods/exec`, in the operator or cluster namespace | Root on the node, Ceph admin, Operator identity | The cluster namespace must admit privileged pods for the OSDs. A pod in the operator namespace can use the operator's ServiceAccount. The OSD pods hold the admin keyring. See also [OpenShift](ceph-openshift.md) |

## Granting access

Every resource and permission on this page is an administrative grant, and Rook currently supports
granting them only to trusted administrators. Where a row names what a grant is equivalent to, it is
at least that. Rook's custom resources are not aggregated into the built-in `admin`, `edit` and
`view` ClusterRoles, so binding those roles in an application namespace does not grant them. The
exception is the `rook-ceph` chart setting `rbacAggregate.enableOBCs`, `false` by default, which
aggregates permission to create and edit ObjectBucketClaims into `edit`, and through it `admin`,
and read access into `view`, so it grants ObjectBucketClaims to every principal bound to `admin` or
`edit`. Binding `admin` or `edit` in the operator or cluster namespace grants the namespace
permissions above.
