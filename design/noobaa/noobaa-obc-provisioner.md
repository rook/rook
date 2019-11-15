♜ [Rook NooBaa Design](README.md) /
# OBC Provisioner

Kubernetes natively supports dynamic provisioning for many types of file and block storage, but lacks support for object bucket provisioning.
In order to provide a native provisioning of object storage buckets, the concept of Object Bucket Claim (OBC/OB) was introduced in a similar manner to Persistent Volume Claim (PVC/PV)

The `lib-bucket-provisioner` repo provides a library implementation and design to unify the implementations:

[OBC Design Document](https://github.com/yard-turkey/lib-bucket-provisioner/blob/master/doc/design/object-bucket-lib.md)


# StorageClass

The administrator will create StorageClasses and control its visibility to app-owners using RBAC rules.

See https://github.com/yard-turkey/lib-bucket-provisioner/blob/master/deploy/storageClass.yaml

Example:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: noobaa-default-class
provisioner: noobaa.rook.io/bucket
parameters:
  reclaimPolicy: Delete
  backingStore: noobaa-aws-resource
```

# OBC

Applications that require a bucket will create an OBC and refer to a storage class name.

See https://github.com/yard-turkey/lib-bucket-provisioner/blob/master/deploy/example-claim.yaml

The operator will watch for OBC's and fulfill the claims by create/find existing bucket in NooBaa, and will share a config map and a secret with the application in order to give it all the needed details to work with the bucket.

Example:

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: my-bucket-claim
spec:
  generateBucketName: "my-bucket-"
  storageClassName: noobaa-default-class
  SSL: false
```

# Bucket Permissions and Sharing

The scope of bucket permissions will be at the namespace scope - this means that all the OBC's from the same namespace will receive S3 credentials that has permission to use any other bucket provisioned by that namespace. Notice that also listing buckets with these S3 credentials will return only the subset of buckets claimed by that namespace.

While there are cases that this namespace scope is not enough, it provides a simple model for sharing and privacy for the initial release.
