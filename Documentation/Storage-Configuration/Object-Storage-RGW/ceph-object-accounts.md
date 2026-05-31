---
title: Object Store Accounts
---

!!! attention
    This feature is experimental and currently only supported with the Ceph main branch image (`quay.ceph.io/ceph-ci/ceph:main`).

## Overview

[RGW accounts](https://docs.ceph.com/en/latest/radosgw/account/) provide a way to group and manage object store users and their resources under a single entity.
When a user is associated with an account, resources created by that user are owned by the account.
This enables centralized resource management and ownership across multiple users.

Rook can manage RGW accounts through the `CephObjectStoreAccount` custom resource and associate users with
accounts via the `accountRef` field in `CephObjectStoreUser`.

## Prerequisites

This guide assumes the following:

1. A Rook cluster as explained in the [Quickstart](../../Getting-Started/quickstart.md).
2. A running [Object Store](object-storage.md).

## Create an Object Store Account

The following example creates an RGW account associated with an object store:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  name: my-account
  namespace: rook-ceph
spec:
  store: my-store
```

Create the account:

```console
kubectl create -f object-account.yaml
```

Verify the account was created successfully:

```console
kubectl -n rook-ceph get cephobjectstoreaccount my-account
```

The account is ready when the `Phase` is `Ready`.

### Account Settings

* `store`: The name of the `CephObjectStore` in which the account will be created. This field is **immutable** once set.
* `name`: An optional display name for the RGW account if different from the CR name.
* `accountID`: An optional unique account identifier. Format must be `RGW` followed by 17 digits (e.g., `RGW00889737169837717`). If not specified, Ceph will auto-generate the account ID. This field is **immutable** once set.
* `rootUser`: Optional configuration for the account root user.
    * `skipCreate`: When set to `true`, the root user will not be created for this account. This can be useful if the user wants to manually manage the root user outside of Rook.
    * `displayName`: Display name for the root user.

### Account Status

Once the account is created, the status will contain:

* `phase`: The current phase of the account (e.g., `Ready`).
* `accountID`: The account ID assigned to the RGW account.
* `rootAccountSecretName`: The name of the Kubernetes secret containing the root user's access credentials.

### Root User Credentials

By default, a root user is created for each account. The root user credentials are stored in a Kubernetes secret
referenced by `status.rootAccountSecretName`. The secret contains the `AccessKey` and `SecretKey` for the root user.

```console
kubectl -n rook-ceph get secret rook-ceph-object-root-user-my-account -o jsonpath='{.data.AccessKey}' | base64 --decode
kubectl -n rook-ceph get secret rook-ceph-object-root-user-my-account -o jsonpath='{.data.SecretKey}' | base64 --decode
```

## Create a User with an Account Reference

To associate a `CephObjectStoreUser` with an account, set the `accountRef` field to reference the account CR.
When set, the user is created as an account user with no default permissions, and resources created by this user
are owned by the account.

!!! note
    The `accountRef` field is **immutable** once set. The referenced account must be in the same namespace as the user.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: my-display-name
  accountRef:
    name: my-account
```

Create the user:

```console
kubectl create -f object-user.yaml
```

Verify the user was created and associated with the account:

```console
kubectl -n rook-ceph get cephobjectstoreuser my-user -o yaml
```

## Deleting an Account

Deleting a `CephObjectStoreUser` that references an account will not affect the account itself.
The account and its other users will continue to function normally.

!!! note
    Before deleting an account, ensure all `CephObjectStoreUser` resources and buckets associated with the account are removed first. Ceph will block account deletion if the account still has associated users or buckets.

To delete the account itself:

```console
kubectl -n rook-ceph delete cephobjectstoreaccount my-account
```
