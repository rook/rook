---
title: Bucket Claim
---

Rook supports the creation of new buckets and access to existing buckets via two custom resources:

* an `Object Bucket Claim (OBC)` is custom resource which requests a bucket (new or existing) and is described by a Custom Resource Definition (CRD) shown below.
* an `Object Bucket (OB)` is a custom resource automatically generated when a bucket is provisioned. It is a global resource, typically not visible to non-admin users, and contains information specific to the bucket. It is described by an OB CRD, also shown below.

An OBC references a storage class which is created by an administrator. The storage class defines whether the bucket requested is a new bucket or an existing bucket. It also defines the bucket retention policy.
Users request a new or existing bucket by creating an OBC which is shown below. The ceph provisioner detects the OBC and creates a new bucket or grants access to an existing bucket, depending the storage class referenced in the OBC. It also generates a Secret which provides credentials to access the bucket, and a ConfigMap which contains the bucket's endpoint. Application pods consume the information in the Secret and ConfigMap to access the bucket. Please note that to make provisioner watch the cluster namespace only you need to set `ROOK_OBC_WATCH_OPERATOR_NAMESPACE` to `true` in the operator manifest, otherwise it watches all namespaces.

## Example

### OBC Custom Resource

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-bucket [1]
  namespace: rook-ceph [2]
spec:
  bucketName: [3]
  generateBucketName: photo-booth [4]
  storageClassName: rook-ceph-bucket [5]
  additionalConfig: [6]
    maxObjects: "1000"
    maxSize: "2G"
```

1. `name` of the `ObjectBucketClaim`. This name becomes the name of the Secret and ConfigMap.
1. `namespace`(optional) of the `ObjectBucketClaim`, which is also the namespace of the ConfigMap and Secret.
1. `bucketName` name of the `bucket`.
**Not** recommended for new buckets since names must be unique within
an entire object store.
1. `generateBucketName` value becomes the prefix for a randomly generated name, if supplied then `bucketName` must be empty.
If both `bucketName` and `generateBucketName` are supplied then `BucketName` has precedence and `GenerateBucketName` is ignored.
If both `bucketName` and `generateBucketName` are blank or omitted then the storage class is expected to contain the name of an _existing_ bucket. It's an error if all three bucket related names are blank or omitted.
1. `storageClassName` which defines the StorageClass which contains the names of the bucket provisioner, the object-store and specifies the bucket retention policy.
1. `additionalConfig` is an optional list of key-value pairs used to define attributes specific to the bucket being provisioned by this OBC. This information is typically tuned to a particular bucket provisioner and may limit application portability. Options supported:

    * `maxObjects`: The maximum number of objects in the bucket
    * `maxSize`: The maximum size of the bucket, please note minimum recommended value is 4K.

### OBC Custom Resource after Bucket Provisioning

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  creationTimestamp: "2019-10-18T09:54:01Z"
  generation: 2
  name: ceph-bucket
  namespace: default [1]
  resourceVersion: "559491"
spec:
  ObjectBucketName: obc-default-ceph-bucket [2]
  additionalConfig: null
  bucketName: photo-booth-c1178d61-1517-431f-8408-ec4c9fa50bee [3]
  storageClassName: rook-ceph-bucket [4]
status:
  phase: Bound [5]
```

1. `namespace` where OBC got created.
1. `ObjectBucketName` generated OB name created using name space and OBC name.
1. the generated (in this case), unique `bucket name` for the new bucket.
1. name of the storage class from OBC got created.
1. phases of bucket creation:
    * _Pending_: the operator is processing the request.
    * _Bound_: the operator finished processing the request and linked the OBC and OB
    * _Released_: the OB has been deleted, leaving the OBC unclaimed but unavailable.
    * _Failed_: not currently set.

### App Pod

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
    envFrom: [1]
    - configMapRef:
        name: ceph-bucket [2]
    - secretRef:
        name: ceph-bucket [3]
```

1. use `env:` if mapping of the defined key names to the env var names used by the app is needed.
1. makes available to the pod as env variables: `BUCKET_HOST`, `BUCKET_PORT`, `BUCKET_NAME`
1. makes available to the pod as env variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`

### StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-bucket
  labels:
    aws-s3/object [1]
provisioner: rook-ceph.ceph.rook.io/bucket [2]
parameters: [3]
  objectStoreName: my-store
  objectStoreNamespace: rook-ceph
  bucketName: ceph-bucket [4]
reclaimPolicy: Delete [5]
```

1. `label`(optional) here associates this `StorageClass` to a specific provisioner.
1. `provisioner` responsible for handling `OBCs` referencing this `StorageClass`.
1. **all** `parameter` required.
1. `bucketName` is required for access to existing buckets but is omitted when provisioning new buckets.
Unlike greenfield provisioning, the brownfield bucket name appears in the `StorageClass`, not the `OBC`.
1. rook-ceph provisioner decides how to treat the `reclaimPolicy` when an `OBC` is deleted for the bucket. See explanation as [specified in Kubernetes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#retain)

    * _Delete_ = physically delete the bucket.
    * _Retain_ = do not physically delete the bucket.
