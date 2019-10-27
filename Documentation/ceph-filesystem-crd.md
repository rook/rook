# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [CephFilesystem](#cephfilesystem)
* [CephFilesystemList](#cephfilesystemlist)
* [FilesystemSpec](#filesystemspec)
* [MetadataServerSpec](#metadataserverspec)

## CephFilesystem



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true |
| spec |  | [FilesystemSpec](#filesystemspec) | true |

[Back to TOC](#table-of-contents)

## CephFilesystemList



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true |
| items |  | [][CephFilesystem](#cephfilesystem) | true |

[Back to TOC](#table-of-contents)

## FilesystemSpec

FilesystemSpec represents the spec of a file system

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadataPool | The metadata pool settings | PoolSpec | false |
| dataPools | The data pool settings | []PoolSpec | false |
| preservePoolsOnDelete | Preserve pools on filesystem deletion | bool | true |
| metadataServer | The mds pod info | [MetadataServerSpec](#metadataserverspec) | true |

[Back to TOC](#table-of-contents)

## MetadataServerSpec



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| activeCount | The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode. | int32 | true |
| activeStandby | Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover. If false, standbys will still be available, but will not have a warm metadata cache. | bool | true |
| placement | The affinity to place the mds pods (default is to place on all available node) with a daemonset | rook.Placement | true |
| annotations | The annotations-related configuration to add/set on each Pod related object. | rook.Annotations | false |
| resources | The resource requirements for the rgw pods | [v1.ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#resourcerequirements-v1-core) | true |

[Back to TOC](#table-of-contents)
