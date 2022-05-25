---
title: Ceph Upgrades
---

This guide will walk you through the steps to upgrade the version of Ceph in a Rook cluster.
Rook and Ceph upgrades are designed to ensure data remains available even while
the upgrade is proceeding. Rook will perform the upgrades in a rolling fashion
such that application pods are are not disrupted.

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been
updated in the CR), Rook will go through all the daemons one by one and will individually perform
checks on them. It will make sure a particular daemon can be stopped before performing the upgrade.
Once the deployment has been updated, it checks if this is ok to continue. After each daemon is
updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for
MDSes, etc.), then only when the condition is met we move to the next daemon. We repeat this process
until all the daemons have been updated.

## Considerations

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process.
* We recommend that you read this document in full before you undertake a Ceph upgrade.

## Supported Versions

Rook v1.9 supports the following Ceph versions:

* Ceph Quincy v17.2.0 or newer
* Ceph Pacific v16.2.0 or newer
* Ceph Octopus v15.2.0 or newer

Rook v1.10 is planning to drop support for Ceph Octopus (15.2.x),
so please consider upgrading your Ceph cluster.

!!! important
    When an update is requested, the operator will check Ceph's status,
    **if it is in `HEALTH_ERR` the operator will refuse to proceed with the upgrade.**

We recommend updating to v16.2.7 or newer. If you require updating **to v16.2.0-v16.2.6**,
please see the [v1.8 upgrade guide for a special upgrade consideration](https://rook.github.io/docs/rook/v1.8/ceph-upgrade.html#disable-bluestore_fsck_quick_fix_on_mount).

### Quincy Consideration

In Ceph Quincy (v17), the `device_health_metrics` pool was renamed to `.mgr`. Ceph will perform this
migration automatically. If you do not use CephBlockPool to customize the configuration of the
`device_health_metrics` pool, the pool rename will be automatic.

If you do use CephBlockPool to customize the configuration of the `device_health_metrics` pool, you
will need two extra steps after the Ceph upgrade is complete. Once upgrade is complete:

1. Create a new CephBlockPool to configure the `.mgr` built-in pool. You can reference the example
[builtin mgr pool](https://github.com/rook/rook/blob/master/deploy/examples/pool-builtin-mgr.yaml).
2. Delete the old CephBlockPool that represents the `device_health_metrics` pool.

### CephNFS User Consideration

Ceph Quincy v17.2.0 has a potentially breaking regression with CephNFS. See the NFS documentation's
[known issue](../CRDs/ceph-nfs-crd.md#ceph-v1720) for more detail.

### Ceph Images

Official Ceph container images can be found on [Quay](https://quay.io/repository/ceph/ceph?tab=tags).

These images are tagged in a few ways:

* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v16.2.9-20220519`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v16`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

### Example Upgrade to Ceph Pacific

#### **1. Update the Ceph daemons**

The upgrade will be automated by the Rook operator after you update the desired Ceph image
in the cluster CRD (`spec.cephVersion.image`).

```console
ROOK_CLUSTER_NAMESPACE=rook-ceph
NEW_CEPH_IMAGE='quay.io/ceph/ceph:v16.2.9-20220519'
kubectl -n $ROOK_CLUSTER_NAMESPACE patch CephCluster $ROOK_CLUSTER_NAMESPACE --type=merge -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

#### **2. Wait for the pod updates**

As with upgrading Rook, you must now wait for the upgrade to complete. Status can be determined in a
similar way to the Rook upgrade as well.

```console
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \tceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}'
```

Confirm the upgrade is completed when the versions are all on the desired Ceph version.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"ceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
    ceph-version=15.2.13-0
    ceph-version=16.2.9-0
This cluster is finished:
    ceph-version=16.2.9-0
```

#### **3. Verify cluster health**

Verify the Ceph cluster's health using the [health verification](health-verification.md).
