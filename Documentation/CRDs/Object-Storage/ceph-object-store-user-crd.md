---
title: CephObjectStoreUser CRD
---

Rook allows creation and customization of object store users through the custom resource definitions (CRDs). The following settings are available
for Ceph object store users.

## Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: my-display-name
  quotas:
    maxBuckets: 100
    maxSize: 10G
    maxObjects: 10000
  capabilities:
    user: "*"
    bucket: "*"
```

## Object Store User Settings

### Metadata

* `name`: The name of the object store user to create, which will be reflected in the secret and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store user is created.

### Spec

* `store`: The object store in which the user will be created. This matches the name of the objectstore CRD.
* `displayName`: The display name which will be passed to the `radosgw-admin user create` command.
* `clusterNamespace`: The namespace where the parent CephCluster and CephObjectStore are found. If not specified,
    the user must be in the same namespace as the cluster and object store.
    To enable this feature, the CephObjectStore allowUsersInNamespaces must include the namespace of this user.
* `quotas`: This represents quota limitation can be set on the user. Please refer [here](https://docs.ceph.com/en/latest/radosgw/admin/#quota-management) for details.
    * `maxBuckets`: The maximum bucket limit for the user.
    * `maxSize`: Maximum size limit of all objects across all the user's buckets.
    * `maxObjects`: Maximum number of objects across all the user's buckets.
* `capabilities`: Ceph allows users to be given additional permissions. Due to missing APIs in go-ceph for updating the user capabilities, this setting can currently only be used during the creation of the object store user. If a user's capabilities need modified, the user must be deleted and re-created.
    See the [Ceph docs](https://docs.ceph.com/en/latest/radosgw/admin/#add-remove-admin-capabilities) for more info.
    Rook supports adding `read`, `write`, `read, write`, or `*` permissions for the following resources:
    * `user`
    * `buckets`
    * `usage`
    * `metadata`
    * `zone`
    * `roles`
    * `info`
    * `amz-cache`
    * `bilog`
    * `mdlog`
    * `datalog`
    * `user-policy`
    * `odic-provider`
    * `ratelimit`

### CephObjectStoreUser Reference Secret

A Kubernetes secret is created by Rook with the name, `rook-ceph-object-user-<store-name>-<user-name>` in the same namespace of cephobjectUser, where:

* `store-name`: The object store name in which the user will be created. This matches the name of the objectstore CRD.
* `user-name`: The metadata name of the cephObjectStoreUser

Application pods can use the secret to configure access to the object store.

```yaml
apiVersion: v1
data:
  AccessKey: ***       # [1]
  Credentials: ***     # [2]
  Endpoint: ***        # [3]
  SecretKey: ***       # [4]
kind: Secret
...
...
type: kubernetes.io/rook

```

1. `AccessKey`: User access key for ceph S3.
2. `SecretKey`: User secret key for ceph S3.
3. `Credentials`: Comma separated values of all access and secret keys.
4. `Endpoint`: Endpoint for ceph S3.

#### User-Specified Reference Secret

AccessKey, SecretKey, and Credentials can be specified before CephObjectStore creation or modified afterwards to support user-specified keys and key rotation. Endpoint cannot be overridden by users and will always be updated by Rook to match the latest CephObjectStore information.

Create or update the Kubernetes secret with name, `rook-ceph-object-user-<store-name>-<user-name>` in the same namespace of cephobjectUser,

Steps:

i) Specify the annotations as `rook.io/source-of-truth: secret` and type as `type: "kubernetes.io/rook"`.

ii) Specify the user defined `AccessKey` and `SecretKey`.

iii) (optional) The array of `Credentials` which contains all the trusted access and secret keys. Make sure to also add the user defined `AccessKey` and `SecretKey` of step ii).

`Credentials`
```console
[{"access_key":"IE58RNT71Y2F1EQE80RA","secret_key":"cULyMz5dCpX18dPsJhpIKay7vcDNRNJWJPu8VqUA"}, {"access_key":"IE58RNT71Y2F1EQE80RC","secret_key":"cULyMz5dCpX18dPsJhpIKay7vcDNRNJWJPu8VqUA"}]
```

If any key is present in the ceph user and not present in the `Credentials`, it will be removed from the ceph user too.
If `Credentials` is left empty, it will be auto updated by the latest AccessKey and SecretKey and will remove all the other keys from the ceph user.

!!! note
    Secret data usually needs to be converted to base64 format.

Example Secret:

```console
kubectl create -f
apiVersion: v1
kind: Secret
metadata:
  name: rook-ceph-object-user-my-store-my-user
  namespace: rook-ceph
  annotations:
    rook.io/source-of-truth: secret
data:
  AccessKey: ***
  SecretKey: ***
  Endpoint: ***
  Credentials: ***
type: "kubernetes.io/rook"
```

!!! note
    Be careful when updating Kubernetes secret values. Mistakes entering info can cause errors reconciling the CephObjectStore.
