# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [CephNFS](#cephnfs)
* [CephNFSList](#cephnfslist)
* [GaneshaRADOSSpec](#ganesharadosspec)
* [GaneshaServerSpec](#ganeshaserverspec)
* [NFSGaneshaSpec](#nfsganeshaspec)

## CephNFS



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true |
| spec |  | [NFSGaneshaSpec](#nfsganeshaspec) | true |

[Back to TOC](#table-of-contents)

## CephNFSList



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true |
| items |  | [][CephNFS](#cephnfs) | true |

[Back to TOC](#table-of-contents)

## GaneshaRADOSSpec



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| pool | Pool is the RADOS pool where NFS client recovery data is stored. | string | true |
| namespace | Namespace is the RADOS namespace where NFS client recovery data is stored. | string | true |

[Back to TOC](#table-of-contents)

## GaneshaServerSpec



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| active | The number of active Ganesha servers | int | true |
| placement | The affinity to place the ganesha pods | rook.Placement | true |
| annotations | The annotations-related configuration to add/set on each Pod related object. | rook.Annotations | false |
| resources | Resources set resource requests and limits | [v1.ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#resourcerequirements-v1-core) | false |

[Back to TOC](#table-of-contents)

## NFSGaneshaSpec

NFSGaneshaSpec represents the spec of an nfs ganesha server

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| rados |  | [GaneshaRADOSSpec](#ganesharadosspec) | true |
| server |  | [GaneshaServerSpec](#ganeshaserverspec) | true |

[Back to TOC](#table-of-contents)
