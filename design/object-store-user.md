### Background
This proposal is a necessary part of another enhancement to [dynamically provision S3 buckets](https://github.com/rook/rook/issues/2480). 

With the merge of [2076](https://github.com/rook/rook/pull/2076) Rook now supports an object user CRD and its generated secret containing the AWS key pairs. To create a new object store user, a developer creates a simple _CephObjectStoreUser_ yaml file defining the object store and the user’s display name. After applying this file a CephObjectStoreUser CRD is created in the object store’s namespace, and the S3 object user is created.

Some issues with this approach are:
* the developer needs to know the name and namespace of the object store service, both to create the object store user yaml file, and to know, in advance, the name of the generated secret[1]. 
* the object store user needs to reside in the same namespace as the target object store.

<sup>[1]  It is desirable that the app developer be able to produce the pod yaml before creation of the object store user and its generated secret, and, thus, the secret’s name must be predictable since it is consumed by the pod. Today, the secret name consists of “rook-ceph-object-user-” + _objectStore’s metadata.Name_ + _CephObjectStoreUser’s metadata.Name_, and is assumed to live in the object store’s namespace. This enhancement proposes to remove the object store name from the secret’s name. Since object store user names must be unique within a namespace, a secret name derived from the CephObjectStoreUser’s `metadata.Name` is also unique, thus the object store name is not required. This supports an advantage of using a storage class (see below), namely, that the developer does not need to know the name (or namespace) of the object store.</sup>

## Proposed Enhancement
1. Allow a storage class, which references an object store service, to be defined in the object user API, but continue to support the existing `store` property.
1. Remove the restriction that a CephObjectStoreUser be located in the same namespace as the ObjectStore.
1. Change the ownerReference of the generated key-pair Secret to reference the CephObjectStoreUser rather than the ceph-cluster.
1. Change the name of the generated secret per [1] above. Note: this change is **not** backwards compatible.

#### Storage Class
A [storage class](https://kubernetes.io/docs/concepts/storage/storage-classes/) is a global Kubernetes resource with the ability to pass parameters to a provisioner. Storage classes further abstract the underlying storage based on admin policies. Storage class visibility can be controlled by RBAC rules. A simple storage class for a rook-ceph object store might look like this:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-object-store
provisioner: kubernetes.io/rook-ceph-operator
parameters:
  objectStore: rook-ceph/objectStoreName  # format: “namespace/objectStoreName”
```
There should be a 1:1 mapping of storage class and rook object store. For example, if there are 5 object stores, each in their own namespace, then 5 global storage classes are needed. Likewise, if there are 5 object stores in a single namespace, then, again, 5 storage classes are required. Note that the creation of an object store does not require an associated storage class. Using storage class more closely adheres to the Kubernetes storage model and offers greater flexibility to the developer and storage admin.

Adding storage class to the rook-ceph object user API more closely adheres to the Kubernetes storage model and offers greater flexibility to the developer and the storage admin. However, the existing `store` stanza will be supported for backwards compatibility. The API will verify that `store` and `storageClass` are mutually exclusive and return an error if both (or neither) are specified.

#### Namespace
When a storage class is specified the object store’s namespace is found in the storage class parameter. A typical object store user definition might look like this:
```yaml
apiVersion: storage.k8s.io/v1
kind: cephObjectStoreUser
metadata:
  name: my-user-1
    namespace: dev-user
spec:
  storageClass: rook-ceph-object-store  # name of storage class
  displayName: aUserDisplayName
```
Since a storage class is specified we don’t need to know the namespace nor name of the object store. If the storage class is omitted then the `store` stanza must be provided and non-empty, and the user and secret will reside in the same namespace as the object store (as is true today). When `storageClass` is defined the `store` property may be completely omitted, or specified but its value must be empty.

#### Secret OwnerReference
Today, the generated key-pair secret’s owner reference is the id of the ceph cluster, which is also the same owner set for the CephObjectStoreUser. Thus, using the Kubernete’s [cascading deletes feature](https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/#foreground-cascading-deletion), the secret is not actually deleted until the ceph cluster is deleted. This enhancement proposes to set the Secret’s OwnerReference to the id of the CephObjectStoreUser that created the secret. So, when the CephObjectStoreUser is deleted, its secret is automatically deleted.

#### Secret Finalizer
A finalizer will be added so that the the generated secret cannot be prematurely deleted until its owner CephObjectStoreUser is deleted. The purpose of this is to prevent the stand-alone deletion of the generated secret and thus to ensure that the user and secret resources are coupled. It is possible to allow deletion of the secret without deleting the user depending on how rook-ceph behaves when a bucket's credentials are missing.

#### Secret Name
See note [1] above. Essentially, if the object store name is embedded in the secret’s name then the app developer needs to know the name (and namespace) of the object store. This defeats the main benefit of adding a storage class property to the API.

### Use Cases
_As an app developer, I want to use `kubectl` to request the creation of an object user associated to an existing object store represented by a storage class_
##### Steps
  1. The Rook-Ceph operator, watching all namespaces, sees a new `CephObjectStoreUser` in the developer's namespace. The associated object store service is obtained via the referenced Storage Class’s  `spec.parameters.objectstore` field.
  1. As done today, the operator creates a new S3 user in the object store. The credentials for this user are stored in a Secret now created in *same* namespace as the originating CephObjectStoreUser. Per the standard controller pattern, if an error occurs the operator will retry the create.
  1. Per standard kubelet behavior, the app pod is blocked from running until it can access the Secret data.

_As an app developer, I want to use `kubectl` to request the deletion of an object store user and expect the secret to also be deleted._
##### Steps
The kubectl API call to delete the object user triggers an automated cleanup sequence:
  1. The CephObjectStoreUser is marked for deletion first and left in the foreground. Deletion is blocked (by Kubernetes) by an automated foreground finalizer which is removed once the secret is deleted.
  1. Since the Secret’s ownerReference points to the CephObjectStoreUser, it is automatically deleted, and then Kubernetes removes the automated foreground finalizer on the CephObjectStoreUser.
  1.  The CephObjectStoreUser is is garbage collected once the foreground finalizer is removed.

**Note:**
The actual S3 user and key-pair credentials are **not** deleted. Only the associated Kubernetes resources are deleted. Thus, creating _user-1_, deleting _user-1_ and re-creating _user-1_ will fail due to _user-1_ already existing. It would be possible, in a later release, to add a retention property to the `CephObjectStoreUser` CRD to control whether or not the underlying S3 resources are physically deleted.

### Resource Specs
_rookObjectUser.yaml_ (user defined):
```yaml
apiVersion: ceph.rook.io/v1beta1
kind: CephObjectStoreUser
metadata:
  name: my-user-1
  namespace: dev-user
spec:
  displayName: my-goodness-user-1
  storageClass: rook-ceph-object-store
```
_rookObjectUser.yaml_ (after creation):
```yaml
apiVersion: ceph.rook.io/v1beta1
kind: CephObjectStoreUser
metadata:
  name: my-user-1
  namespace: dev-user
  objectReference: # bookkeeping
    kind: Secret
    apiVersion: v1
    name: rook-ceph-object-user-my-user-1
    namespace: dev-user  # can't yet get a secret via uid so name+ns are used here
spec:
  displayName: my-goodness-user-1
  storageClass: rook-ceph-object-store
status:
  ...
```

_secret.yaml_ (operator generated):
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rook-ceph-object-user-my-user-1 # enhancement
  namespace: dev-user
  ownerReferences:   # enhancement...
  - name: my-user-1  # user resource name, not ceph-cluster name
    uid: 1234-qwer-4321-rewq
    blockOwnerDeletion: true
  finalizers:   # new
  - rook-ceph.io/provisioner/user/my-user-1
type: Opaque
data:
  aws-access-key-id: yzzxx
  aws-secret-access-key: xyzzy
```

_objectStorageClass.yaml_ (admin defined):
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-object-store
provisioner: kubernetes.io/rook-ceph-operator
parameters:
  objectStore: rook-ceph.io/objectStoreName  # format: namespace/objectStoreService
```

_appPod.yaml_ (user defined):
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-pod
  namespace: dev-user
spec:
  containers:
  - name: mycontainer
    image: redis
    env:
      - name: BUCKET_AWS_ACCESS_KEY_ID # user defined
        valueFrom:
          secretKeyRef:
            name: rook-ceph-object-user-my-user-1 # name of secret
            key: aws-access-key-id  # generated key
      - name: BUCKET_AWS_SECRET_ACCESS_KEY  # user defined
        valueFrom:
          secretKeyRef:
            name: rook-ceph-object-user-my-user-1
            key: aws-secret-access-key
...
```

