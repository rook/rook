♜ [Rook NooBaa Design](README.md) /
# NooBaaBucketClass

NooBaaBucketClass is a CRD representing a class for buckets that defines policies for data placement and more.

Data placement capabilities are built as a multi-layer structure, here are the layers bottom-up:
- Spread Layer - list of backing-stores, aggregates the storage of multiple stores.
- Mirroring Layer - list of spread-layers, async-mirroring to all mirrors, with locality optimization.
- Tiering Layer - list of mirroring-layers, push cold data to next tier.

For more information on using bucket-classes from S3 see [S3 Account](noobaa-s3-account.md).

# Definitions

- CRD: [noobaa-bucket-class-crd.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-bucket-class-crd.yaml)
- Examples:
    - [noobaa-bucket-class-example-single.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-bucket-class-example-single.yaml)
    - [noobaa-bucket-class-example-mirror.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-bucket-class-example-mirror.yaml)


# Reconcile

- The operator will verify that bucket-class is valid - i.e. that the backing-stores exist and can be used.
- Changes to a bucket-class spec will be propagated to buckets that were instantiated from it.
- Other than that the bucket-class is passive, just waiting there for new buckets to use it.

# Read Status

Here is an example of healthy status:

```yaml
apiVersion: noobaa.rook.io/v1alpha1
kind: NooBaaBucketClass
metadata:
  name: noobaa-default-class
  namespace: rook-noobaa
spec:
  ...
status:
  health: OK
  buckets: 31
  issues: []
```

# Delete

Deleting a bucket-class should not be possible as long as there are buckets referring to it.
Since CRD's do not offer this level of control, the operator will use the `finalizer` pattern as explained in the link below, and set a finalizer on every bucket-class to mark that external cleanup is needed before it can be delete:

https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#finalizers

The status of the bucket-class will show the remaining buckets that prevent if from being deleted:

```yaml
apiVersion: noobaa.rook.io/v1alpha1
kind: NooBaaBucketClass
metadata:
  name: noobaa-default-class
  namespace: rook-noobaa
  finalizers:
    - finalizer.noobaa.rook.io
spec:
  ...
status:
  health: WARNING
  buckets: 3
  issues:
    - title: Bucket-class "noobaa-default-class" - Cannot remove `finalizer.noobaa.rook.io` to complete deletion while it has buckets
      buckets:
        - bucket1
        - bucket2
        - bucket3
      createTime: "2019-06-04T13:05:35.473Z"
      lastTime: "2019-06-04T13:05:35.473Z"
      troubleshooting: "https://github.com/noobaa/noobaa-core/wiki/Bucket-class-finalizer-troubleshooting"
```
