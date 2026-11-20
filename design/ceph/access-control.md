---
title: Access Control for Rook Resources
target-version: release-1.22
---

# Access Control for Rook Resources

## Summary

Rook's custom resources configure the Ceph cluster and the privileged daemons that run it, so
permission to create or edit them is an administrative grant, and Rook currently supports granting
it only to trusted administrators. ObjectBucketClaims in application namespaces are the one
exception. The
[Admin Access](../../Documentation/Getting-Started/Prerequisites/prerequisites.md#admin-access)
section of the prerequisites states this for users. This document is the reference behind it: it
lists each Rook custom resource and each related permission in the Rook namespaces, and, where it
follows from Rook's design, what a grant is equivalent to. It also lists the access that is not
administrative.

### Goals

* Describe the access model that Rook's custom resources, and the related permissions in the Rook
    namespaces, are designed for.
* Give users and reviewers, human or automated, one reference for which inputs to Rook are
    administrative.

### Non-Goals

* Supporting grants of the administrative resources and permissions listed here to principals
    other than trusted administrators.
* Changing Rook's RBAC, or the validation of any field.

## Access model

Every resource and permission in the tables below is an administrative grant unless its row says
otherwise. Custom resources that Rook installs and that this document does not list, including ones
added later, are administrative until this document says otherwise. That a setting of an
administrative resource can configure the cluster or its nodes in a damaging way is expected: the
principal that can change it is already trusted with the cluster. Where a row names what a grant is
equivalent to, it is at least that.

The tables use these terms:

* **Root on the node**: the grant can run code in, or choose the image or host paths of, a container
    that runs privileged or with access to the host.
* **Ceph admin**: the grant can obtain the `client.admin` key, or a key with the same capabilities.
* **Secrets in the namespace**: the grant can read any Secret in the resource's namespace.
* **Operator identity**: the grant can obtain the permissions of the operator's ServiceAccount,
    `rook-ceph-system`, which can read Secrets in every namespace.
* **Administrative**: grant only to trusted administrators.
* **User**: a user-level grant; see [What is not administrative](#what-is-not-administrative).

The cluster namespace is the namespace of the CephCluster. In the default install it is also the
operator namespace.

### Custom resources

| Resource | Equivalent to | Details |
| -------- | ------------- | ------- |
| [CephCluster](../../Documentation/CRDs/Cluster/ceph-cluster-crd.md) | Root on the node, Ceph admin, Operator identity | `cephVersion.image` is the image of the privileged OSD containers, which hold the admin keyring. `dataDirHostPath` and `storage` choose the host paths and devices that privileged pods use, and `cleanupPolicy` lets privileged jobs wipe them. The operator can connect to the key management service that `security.kms` configures; see [Key Management System](../../Documentation/Storage-Configuration/Advanced/key-management-system.md) |
| [CephNFS](../../Documentation/CRDs/ceph-nfs-crd.md) | Root on the node, Ceph admin, Secrets in the namespace | Every `volumeSource` under `security` accepts a writable `hostPath` and any Secret in its namespace, including `rook-ceph-admin-keyring`. `server.image` and `security.sssd.sidecar.image` set the images of the containers that mount those volumes |
| [CephNVMeOFGateway](../../Documentation/Storage-Configuration/Block-Storage-RBD/nvme-of.md) | Root on the node, Ceph admin | `image` runs in a privileged container that holds the admin keyring |
| [CephObjectStore](../../Documentation/CRDs/Object-Storage/ceph-object-store-crd.md) | Root on the node, Ceph admin, Secrets in the namespace, Operator identity | `gateway.additionalVolumeMounts` accepts a writable `hostPath` and any Secret in its namespace. `gateway.rgwConfig`, `gateway.rgwConfigFromSecret` and `gateway.rgwCommandFlags` set the options of the RGW daemon. The operator can connect to the key management service that `security.kms` configures; see [Key Management System](../../Documentation/Storage-Configuration/Advanced/key-management-system.md) |
| [CephFilesystem](../../Documentation/CRDs/Shared-Filesystem/ceph-filesystem-crd.md) | Administrative | Grant only to trusted administrators |
| [CephClient](../../Documentation/CRDs/ceph-client-crd.md) | Ceph admin | `caps` define the key's access |
| [CephCOSIDriver](../../Documentation/Storage-Configuration/Object-Storage-RGW/cosi.md) | Administrative | Grant only to trusted administrators |
| [CephObjectStoreUser](../../Documentation/CRDs/Object-Storage/ceph-object-store-user-crd.md) | Administrative | Grant only to trusted administrators |
| [ObjectBucketClaim](../../Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md) | User | Creating an ObjectBucketClaim in an application namespace, outside the operator and cluster namespaces, that sets only the `additionalConfig` fields that the operator allows by default is a user-level grant, like a PersistentVolumeClaim. Creating one in the operator or cluster namespace is administrative. The bucket StorageClass, and widening `ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS`, are the administrative grants |
| ObjectBucket | Administrative | Grant only to trusted administrators |
| [CephBucketTopic](../../Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications.md) | Administrative | Grant only to trusted administrators |
| [CephBlockPool](../../Documentation/CRDs/Block-Storage/ceph-block-pool-crd.md) | Administrative | Grant only to trusted administrators |
| [CephBlockPoolRadosNamespace](../../Documentation/CRDs/Block-Storage/ceph-block-pool-rados-namespace-crd.md), [CephFilesystemSubVolumeGroup](../../Documentation/CRDs/Shared-Filesystem/ceph-fs-subvolumegroup-crd.md) | Administrative | Grant only to trusted administrators |
| [CephObjectRealm](../../Documentation/CRDs/Object-Storage/ceph-object-realm-crd.md), [CephObjectZoneGroup](../../Documentation/CRDs/Object-Storage/ceph-object-zonegroup-crd.md), [CephObjectZone](../../Documentation/CRDs/Object-Storage/ceph-object-zone-crd.md) | Administrative | Grant only to trusted administrators |
| [CephFilesystemMirror](../../Documentation/CRDs/Shared-Filesystem/ceph-fs-mirror-crd.md), [CephRBDMirror](../../Documentation/CRDs/Block-Storage/ceph-rbd-mirror-crd.md), [CephObjectStoreAccount](../../Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-accounts.md), [CephBucketNotification](../../Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications.md) | Administrative | Grant only to trusted administrators |

The COSI BucketClass and BucketAccessClass, which Rook does not install, are also administrative:
they name the Secret of the privileged CephObjectStoreUser that the COSI driver uses. The Ceph-CSI
operator's custom resources in the `csi.ceph.io` group, which Rook installs, are also
administrative.

### Namespace permissions

| Permission | Equivalent to | Details |
| ---------- | ------------- | ------- |
| Read Secrets in the cluster namespace | Ceph admin | `rook-ceph-admin-keyring` and `rook-ceph-mon` hold the admin key |
| Read Secrets in the operator namespace, or create or update Secrets in the operator or cluster namespace | Administrative | Grant only to trusted administrators |
| Write ConfigMaps in the operator or cluster namespace | Root on the node, Ceph admin | In the cluster namespace, the `rook-ceph-osd-env-override` ConfigMap sets environment variables in privileged OSD containers that hold the admin keyring |
| Write the `rook-ceph-operator-config` ConfigMap in the operator namespace | Administrative | Sets the operator's settings, which can change what the other rows grant |
| Create pods, or workloads that create pods, or use `pods/exec`, in the operator or cluster namespace | Root on the node, Ceph admin, Operator identity | The cluster namespace must admit privileged pods for the OSDs. A pod in the operator namespace can use the operator's ServiceAccount. The OSD pods hold the admin keyring. See also [OpenShift](../../Documentation/Getting-Started/ceph-openshift.md) |

### What is not administrative

Data-plane access is a user-level grant. This covers PersistentVolumeClaims, ObjectBucketClaims as
described above, COSI BucketClaims, and the S3, NFS, RBD and CephFS clients of the storage that Rook
provides. The limits that Ceph enforces on these users, as Rook configures them, remain security
boundaries.

### Granting access

Rook's custom resources are not aggregated into the built-in `admin`, `edit` and `view`
ClusterRoles, so binding those roles in an application namespace does not grant them. The exception
is the `rook-ceph` chart setting `rbacAggregate.enableOBCs`, `false` by default, which aggregates
permission to create and edit ObjectBucketClaims into `edit`, and through it `admin`, and read
access into `view`. It thereby grants ObjectBucketClaims, a user-level resource, to every principal
bound to `admin` or `edit`. Binding `admin` or `edit` in the operator or cluster namespace grants
the namespace permissions above, which are administrative.
