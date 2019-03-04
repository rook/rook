---
title: Ceph Storage FAQ
weight: 12000
indent: true
---

# Ceph Storage Frequently Asked Questions.

High-quality community support is hard, really hard and very time-consuming. While answering questions is very important, in meantime no features can be developed and bugs fixed. The same questions asked over again and again. This section is to rescue. Contribute to this section, if you want more features and fewer bugs in rook! Be a hero!

## Is there any way to use Rook Ceph with LVM?

It is currently not possible to use LVM devices for OSDs (except using them as `directories`, but that is not recommended due to performance hits then). This will hopefully come with 1.0. See also the related issue [#2047](https://github.com/rook/rook/issues/2047).

## How to use `CephBlockPool` for replicated databases like MongoDB with ReplicaSet and avoid storage replication overhead?

Ceph distributes your data. Ceph does not like `replicated.size: 1` as for Ceph there is then data loss. Ceph's first priority is to protected data and will block IO to such a `size: 1` Pool then when one OSD is down. It is recommended to use Kubernetes local storage for that.

## Is there any way to use one device for everything instead of configure `dataDirHostPath`, `databaseSizeMB`, `walSizeMB`, `journalSizeMB`, `metadataDevice` and `devices`?

`dataDirHostPath` is separate as it contains config data for, e.g., MON and OSDs, and Mon data as of right now. The `*SizeMB` are for configuring the sizes of the partitions on each disk you are providing. `metadataDevice` is optional. See [Ceph Cluster CRD](https://rook.io/docs/rook/v0.9/ceph-cluster-crd.html).

## How to decide the the right size for `databaseSizeMB`, `walSizeMB`, `journalSizeMB`?

For `databaseSizeMB` and `journalSizeMB`, see [cluster.yaml](https://github.com/rook/rook/blob/release-0.9/cluster/examples/kubernetes/ceph/cluster.yaml#L236-L237), they are then auto-calculated by disk size. For more info see Ceph documentation about OSD bluestore and filestore partitioning.
