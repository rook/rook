---
title: adding bucket policy for ceph object store
target-version: release-1.4
---

# Feature Name
Adding bucket policy support for ceph object store

## Summary
The bucket policy is the feature in which permissions for specific user can be set on s3 bucket. Read more about it from [ceph documentation](https://docs.ceph.com/docs/master/radosgw/bucketpolicy/)

Currently [ceph object store](/Documentation/Storage-Configuration/Object-Storage-RGW/object-storage.md) can be consumed either via [OBC](/Documentation/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md) and [ceph object user](/Documentation/CRDs/Object-Storage/ceph-object-store-user-crd.md). As of now there is no direct way for ceph object user to access the OBC. The idea behind this feature to allow that functionality via Rook. Refer bucket policy examples from [here](https://docs.aws.amazon.com/AmazonS3/latest/dev/example-bucket-policies.html).  Please note it is different from [IAM policies](https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies.html).

## Proposal details

The following settings are needed to add for defining policies:
  - _bucketPolicy_ in the `Spec` section of `CephObjectStoreUser` CR
  - _bucketPolicy_ in the `parameters` section of `StorageClass` for `ObjectBucketClaim`

Policies need to be provided in generic `json` format. A policy can have multiple `statements`.

Rook must perform the following checks to verify whether the `bucketpolicy` applicable to OBC.
- for ceph object user, `Principal` value should have username and `Resource` should have specific bucket names. It can be defined for buckets which are not part of the OBC as well, the `bucketname` of an OBC can be fetched from its `configmap`.
- In `StorageClass`, `Principal` value should be `*`(applicable to all users) and `Resource` should have the bucket name can be empty since it can be generated name from Rook as well and will be attached before setting the policy.

Examples:

```yaml
# storageclass-bucket.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-ceph-delete-bucket
provisioner: rook-ceph.ceph.rook.io/bucket
reclaimPolicy: Delete
parameters:
  objectStoreName: my-store
  objectStoreNamespace: rook-ceph
  region: us-east-1
  bucketName: ceph-bkt
  bucketPolicy: "Version": "2012-10-17",
                "Statement": [
                {
                "Sid": "listobjs",
                "Effect": "Allow",
                "Principal": {"AWS": ["arn:aws:iam:::*"]},
                "Action": "s3:ListObject",
                "Resource": "arn:aws:s3:::/*"
                }
                ]

# object-user.yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: "my display name"
  bucketPolicy: "Version": "2012-10-17",
                "Statement": [
                {
                "Sid": "putobjs"
                "Effect": "Allow",
                "Principal": {"AWS": ["arn:aws:iam:::my-user"]},
                "Action": "s3:PutObject",
                "Resource": "arn:aws:s3:::ceph-bkt-1/*"
                },
                {
                "Sid": "getobjs"
                "Effect": "Allow",
                "Principal": {"AWS": ["arn:aws:iam:::my-user"]},
                "Action": "s3:GettObject",
                "Resource": "arn:aws:s3:::ceph-bkt-2/*"
                }
                ]
```
In the above examples, the `bucket policy` mentioned in the `storage class` will be inherited to all the OBCs created from it. And this policy needs to be for the anonymous users(all users in the ceph object store), it will be attached to the bucket during the OBC creation.
In the case of `ceph object store user` the policy can have multiple statements and each represents a policy for the existing buckets in the `ceph object store` for the user `my-user`. During the creation of the user, the `bucketPolicy` CRD will convert into and divide into different bucket policy statement, then fetch each bucket info, and using the credentials of bucket owner this policy will be set via s3 API.
The `bucketPolicy` defined on CRD won't override any existing policies on that bucket, will just append. But this can be easily overwritten with help of S3 client since does not have much control over there.

## APIs and structural changes

The following field will be added to `ObjectStoreUserSpec` and this need to reflected on the existing API's for `CephObjectStoreUser`

```
type ObjectStoreUserSpec struct {
        Store string `json:"store,omitempty"`
        //The display name for the ceph users
        DisplayName string `json:"displayName,omitempty"`
+       //The list of bucket policy for this user
+       BucketPolicy string `json:"bucketPolicy,omitempty"`
 }
```

The `bucket policy` feature is consumed by the brownfield use case of `OBC`, so supporting apis and structures already exists in [policy.go](/pkg/operator/ceph/object/policy.go). Still few more api's are need to read the policy from CRD, validate it and then convert it into `bucketpolicy`, so that can be consumed by existing api's
