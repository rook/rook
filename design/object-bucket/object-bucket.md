# ObjectBucketClaim CRD and Controller

## Proposed Feature

**A generalized method of S3 bucket provisioning through the implementation of an ObjectBucketClaim CRD and controller.** 

Users will request buckets via the Kubernetes API and have returned ConfigMaps with connection information for that bucket.  

One ConfigMap will be created per ObjectBucketClaim instance.  ConfigMap names will be deterministic and derived from the ObjectBucketClaim name.  This gives users the opportunity to define their Pod spec at the same time as the ObjectBucketClaim.

Additionally, the ObjectBucketClaim and Pod can be deployed at the same time thanks to in-built synchronization in Kubernetes. Pods mounting the configMap as either `envFrom:` or `env[0].valueFrom.ConfigMapKeyRef` will have a status of `CreateContainerConfigError` until the configMap is created. Pods mounting as a volume will have a status of `ContainerCreating` until the configMap is created. In both cases, the Pod will start once the configMap exists.

Bucket deletion in the early stages of this controller design will not be addressed beyond cleaning up API objects.  Deletion of the bucket within the object store is left as an administrator responsibility.  This is to prevent the accidental destruction of data by users.

## Design

### Work Flow

#### Assumptions

- An object store has been provisioned with a reachable endpoint.  ([e.g. the Rook-Ceph Object provisioner](https://rook.github.io/docs/rook/master/ceph-object.html)) This may also be an external endpoint such as AWS S3.
- A Service and/or Endpoint API object exists to route connections to the object store.  Typically, object store operators generate these. In cases where the is external to the cluster, they must be configured manually.

#### Use Cases

**Use Case: Expose an Object Store Endpoint**

_As a cluster admin, I want to expose an object store to cluster users so that they may begin bucket provisioning via the Kubernetes API._

![admin-actions](./obc-admin.png)

These steps may be performed in any order.

The admin ...

- creates the ObjectBucketClaim Operator (a Deployment API object)
- creates a StorageClass with `parameters[serviceName]=objectstore-service` and `parameters[serviceNamespace]=some-namespace`

Once the operator is running, it will be begin watching ObjectBucketClaims.
- An object store has been provisioned with a reachable endpoint.  ([e.g. Rook-Ceph Object](https://rook.github.io/docs/rook/master/ceph-object.html))

#### MVP Use Cases

**Use Case: Expose an Object Store Endpoint**

_As a Kubernetes admin, I want to expose an object store to cluster users for bucket provisioning via the Kubernetes API._

![admin-actions](./obc-admin.png)

These next steps may be performed in any order.

_The administrator ..._
- creates the Rook ObjectBucketClaim Operator
- creates a StorageClass with `parameters[objectStoreService]=in-cluster-service` and `parameters[objectStoreNamespace]=admin-namespace`



**Use Case: Provision a Bucket** 

_As a Kubernetes user, I want to leverage the Kubernetes API to create S3 buckets. I expect to get back the bucket connection information in a ConfigMap._
 
![user-actions](./obc-user.png)

1. The Rook Operator detects a new ObjectBucketClaim instance.  
    1. The operator uses `objectBucketClaim.spec.secretName` to get the S3 access keys secret.  
    1. The operator uses `objectBucketClaim.spec.storageClassName` to get the `Service` endpoint of the object store.
1. The operator uses the object store endpoint and access keys for an S3 "make bucket" call.
1. The operator creates a ConfigMap in the namespace of the ObjectBucketClaim with relevant connection data of the bucket.
1. An app Pod may then mount the Secret and the ConfigMap to begin accessing the bucket. 

**Use Case:** Delete an Object Bucket

_As a Kubernetes user, I want to delete ObjectBucketClaim instances and cleanup generated API resources._

1. The user deletes the ObjectBucketClaim via `kubectl delete ...`.
1. The ObjectBucketClaim is marked for deletion and left in the foreground.
1. The respective ConfigMap is deleted.
1. The ObjectBucketClaim is garbage collected.

---

## API Specifications

#### ObjectBucketClaim CRD

```yaml
apiVersion: apiextensions.k8s.io/v1beta1 
kind: CustomResourceDefinition
metadata:
  name: objectbucketclaims.rook.io
spec:
  group: rook.io
  version: v1alpha2
  scope: namespaced
  names:
      kind: ObjectBucketClaim
      listKind: ObjectBucketClaimList
      singular: objectbucketclaim
      plural: objectbucketclaims
```

#### ObjectBucketClaim

```yaml
apiVersion: rook.io/v1alpha2
kind: ObjectBucketClaim
metadata:
  name: my-bucket-1
  namespace: dev-user
  labels:
    rook.io/bucket-provisioner:
    rook.io/object-bucket-claim: my-bucket-1 [1]
spec:
  storageClassName: some-object-store
  secretName: my-s3-key-pair
  bucketNamePrefix: prefix [2]
```

1. Added by the rook operator.
1. The operator will append a hyphen followed by random characters to the string given here.

#### Access Keys Secret  
  
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-s3-key-pair
  namespace: dev-user
data:
  accessKeyId: <base64 encoded string>
  secretAccessKey: <base64 encoded string>
```

#### ObjectBucketClaim ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-object-bucket-my-bucket-1
  namespace: dev-user
  labels:
    rook.io/object-bucket-claim-controller:
    rook.io/object-bucket-claim: my-bucket-1
  ownerReferences:  [1]
  - name: my-bucket-1
    uid: 1234-qwer-4321-rewq
    apiVersion: rook.io/v1alpha2
    kind: ObjectBucketClaim
    blockOwnerDeletion: true 
data:
  ROOK_BUCKET_HOST: http://my-store-url-or-ip
  ROOK_BUCKET_NAME: my-bucket-1
  ROOK_BUCKET_PORT: 80
  ROOK_BUCKET_SSL: no
```

1. Treat the configMap as a child of the ObjectBucketClaim for garbage collection.

#### ObjectStore StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: some-object-store
provisioner: rook.io/object-bucket-claim-controller
parameters:
  serviceName: my-store
  serviceNamespace: some-namespace
```
