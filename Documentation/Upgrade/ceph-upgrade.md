---
title: Ceph Upgrades
---

This guide will walk through the steps to upgrade the version of Ceph in a Rook cluster.
Rook and Ceph upgrades are designed to ensure data remains available even while
the upgrade is proceeding. Rook will perform the upgrades in a rolling fashion
such that application pods are not disrupted.

Rook is cautious when performing upgrades. When an upgrade is requested (the Ceph image has been
updated in the CR), Rook will go through all the daemons one by one and will individually perform
checks on them. It will make sure a particular daemon can be stopped before performing the upgrade.
Once the deployment has been updated, it checks if this is ok to continue. After each daemon is
updated we wait for things to settle (monitors to be in a quorum, PGs to be clean for OSDs, up for
MDSes, etc.), then only when the condition is met we move to the next daemon. We repeat this process
until all the daemons have been updated.

## Considerations

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of the storage cluster, including data loss.
* The Rook cluster's storage may be unavailable for short periods during the upgrade process.
* Read this document in full before undertaking a Rook cluster upgrade.

## Supported Versions

Rook v1.13 supports the following Ceph versions:

* Ceph Reef v18.2.0 or newer
* Ceph Quincy v17.2.0 or newer

Support for Ceph Pacific (16.2.x) is removed in Rook v1.13. Upgrade to Quincy or Reef before upgrading
to Rook v1.13.

!!! important
    When an update is requested, the operator will check Ceph's status,
    **if it is in `HEALTH_ERR` the operator will refuse to proceed with the upgrade.**

!!! warning
    Ceph v17.2.2 has a blocking issue when running with Rook. Use v17.2.3 or newer when possible.

### Quincy Consideration

In Ceph Quincy (v17), the `device_health_metrics` pool was renamed to `.mgr`. Ceph will perform this
migration automatically. The pool rename will be automatically handled by Rook if the configuration
of the `device_health_metrics` pool is not customized via CephBlockPool.

If the configuration of the `device_health_metrics` pool is customized via CephBlockPool, two extra
steps are required after the Ceph upgrade is complete. Once upgrade is complete:

1. Create a new CephBlockPool to configure the `.mgr` built-in pool. For an example, see
   [builtin mgr pool](https://github.com/rook/rook/blob/master/deploy/examples/pool-builtin-mgr.yaml).
2. Delete the old CephBlockPool that represents the `device_health_metrics` pool.

### CephNFS User Consideration

Ceph Quincy v17.2.1 has a potentially breaking regression with CephNFS. See the NFS documentation's
[known issue](../CRDs/ceph-nfs-crd.md#ceph-v1721) for more detail.

### Ceph Images

Official Ceph container images can be found on [Quay](https://quay.io/repository/ceph/ceph?tab=tags).

These images are tagged in a few ways:

* The most explicit form of tags are full-ceph-version-and-build tags (e.g., `v17.2.6-20230410`).
  These tags are recommended for production clusters, as there is no possibility for the cluster to
  be heterogeneous with respect to the version of Ceph running in containers.
* Ceph major version tags (e.g., `v17`) are useful for development and test clusters so that the
  latest version of Ceph is always available.

**Ceph containers other than the official images from the registry above will not be supported.**

### Example Upgrade to Ceph Quincy

#### **1. Update the Ceph daemons**

The upgrade will be automated by the Rook operator after the desired Ceph image is changed in the
CephCluster CRD (`spec.cephVersion.image`).

```console
ROOK_CLUSTER_NAMESPACE=rook-ceph
NEW_CEPH_IMAGE='quay.io/ceph/ceph:v17.2.6-20230410'
kubectl -n $ROOK_CLUSTER_NAMESPACE patch CephCluster $ROOK_CLUSTER_NAMESPACE --type=merge -p "{\"spec\": {\"cephVersion\": {\"image\": \"$NEW_CEPH_IMAGE\"}}}"
```

#### **2. Update the toolbox image**

Since the [Rook toolbox](https://rook.io/docs/rook/latest/Troubleshooting/ceph-toolbox/) is not controlled by
the Rook operator, users must perform a manual upgrade by modifying the `image` to match the ceph version
employed by the new Rook operator release. Employing an outdated Ceph version within the toolbox may result
in unexpected behaviour.

```console
kubectl -n rook-ceph set image deploy/rook-ceph-tools rook-ceph-tools=quay.io/ceph/ceph:v17.2.6-20230410
```

#### **3. Wait for the pod updates**

As with upgrading Rook, now wait for the upgrade to complete. Status can be determined in a similar
way to the Rook upgrade as well.

```console
watch --exec kubectl -n $ROOK_CLUSTER_NAMESPACE get deployments -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"  \treq/upd/avl: "}{.spec.replicas}{"/"}{.status.updatedReplicas}{"/"}{.status.readyReplicas}{"  \tceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}'
```

Confirm the upgrade is completed when the versions are all on the desired Ceph version.

```console
kubectl -n $ROOK_CLUSTER_NAMESPACE get deployment -l rook_cluster=$ROOK_CLUSTER_NAMESPACE -o jsonpath='{range .items[*]}{"ceph-version="}{.metadata.labels.ceph-version}{"\n"}{end}' | sort | uniq
This cluster is not yet finished:
    ceph-version=15.2.13-0
    ceph-version=v17.2.6-0
This cluster is finished:
    ceph-version=v17.2.6-0
```

#### **4. Verify cluster health**

Verify the Ceph cluster's health using the [health verification](health-verification.md).
