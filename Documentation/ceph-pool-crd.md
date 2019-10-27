# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [CephBlockPool](#cephblockpool)
* [CephBlockPoolList](#cephblockpoollist)
* [ErasureCodedSpec](#erasurecodedspec)
* [PoolSpec](#poolspec)
* [ReplicatedSpec](#replicatedspec)

## CephBlockPool



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true |
| spec |  | [PoolSpec](#poolspec) | true |

[Back to TOC](#table-of-contents)

## CephBlockPoolList



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true |
| items |  | [][CephBlockPool](#cephblockpool) | true |

[Back to TOC](#table-of-contents)

## ErasureCodedSpec

ErasureCodeSpec represents the spec for erasure code in a pool

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| codingChunks | Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type) | uint | true |
| dataChunks | Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type) | uint | true |
| algorithm | The algorithm for erasure coding | string | true |

[Back to TOC](#table-of-contents)

## PoolSpec

PoolSpec represents the spec of ceph pool

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| failureDomain | The failure domain: osd/host/(region or zone if topologyAware) - technically also any type in the crush map | string | true |
| crushRoot | The root of the crush hierarchy utilized by the pool | string | true |
| deviceClass | The device class the OSD should set to (options are: hdd, ssd, or nvme) | string | true |
| replicated | The replication settings | [ReplicatedSpec](#replicatedspec) | true |
| erasureCoded | The erasure code settings | [ErasureCodedSpec](#erasurecodedspec) | true |

[Back to TOC](#table-of-contents)

## ReplicatedSpec

ReplicationSpec represents the spec for replication in a pool

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| size | Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type) | uint | true |

[Back to TOC](#table-of-contents)
