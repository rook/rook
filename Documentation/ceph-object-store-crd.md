# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [CephObjectStore](#cephobjectstore)
* [CephObjectStoreList](#cephobjectstorelist)
* [GatewaySpec](#gatewayspec)
* [ObjectStoreSpec](#objectstorespec)

## CephObjectStore



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true |
| spec |  | [ObjectStoreSpec](#objectstorespec) | true |

[Back to TOC](#table-of-contents)

## CephObjectStoreList



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true |
| items |  | [][CephObjectStore](#cephobjectstore) | true |

[Back to TOC](#table-of-contents)

## GatewaySpec



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| port | The port the rgw service will be listening on (http) | int32 | true |
| securePort | The port the rgw service will be listening on (https) | int32 | true |
| instances | The number of pods in the rgw replicaset. If \"allNodes\" is specified, a daemonset is created. | int32 | true |
| allNodes | Whether the rgw pods should be started as a daemonset on all nodes | bool | true |
| sslCertificateRef | The name of the secret that stores the ssl certificate for secure rgw connections | string | true |
| placement | The affinity to place the rgw pods (default is to place on any available node) | rook.Placement | true |
| annotations | The annotations-related configuration to add/set on each Pod related object. | rook.Annotations | false |
| resources | The resource requirements for the rgw pods | [v1.ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#resourcerequirements-v1-core) | true |

[Back to TOC](#table-of-contents)

## ObjectStoreSpec

ObjectStoreSpec represent the spec of a pool

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadataPool | The metadata pool settings | PoolSpec | true |
| dataPool | The data pool settings | PoolSpec | true |
| preservePoolsOnDelete | Preserve pools on object store deletion | bool | true |
| gateway | The rgw pod info | [GatewaySpec](#gatewayspec) | true |

[Back to TOC](#table-of-contents)
