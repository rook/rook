# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [CephObjectStoreUser](#cephobjectstoreuser)
* [CephObjectStoreUserList](#cephobjectstoreuserlist)
* [ObjectStoreUserSpec](#objectstoreuserspec)

## CephObjectStoreUser



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true |
| spec |  | [ObjectStoreUserSpec](#objectstoreuserspec) | true |

[Back to TOC](#table-of-contents)

## CephObjectStoreUserList



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true |
| items |  | [][CephObjectStoreUser](#cephobjectstoreuser) | true |

[Back to TOC](#table-of-contents)

## ObjectStoreUserSpec

ObjectStoreUserSpec represent the spec of an Objectstoreuser

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| store | The store the user will be created in | string | false |
| displayName | The display name for the ceph users | string | false |

[Back to TOC](#table-of-contents)
