# Rook-Ceph Bucket Provisioning

## Proposed Feature

Add an CephObjectBucketClaim API endpoint and control loop to the Rook-Ceph operator for the dynamic provisioning of S3 buckets by Kubernetes users.

### Overview

  Rook-Ceph has developed Custom Resource Definitions to support the dynamic provisioning of Ceph Object Stores and Ceph Object Store Users.  This proposal introduces the next logical step of dynamic bucket provisioning with the addition of a CephObjectBucketClaim CRD.  The intent is to round out the Rook-Ceph experience for Kubernetes users by providing a single control plane for the managing of Ceph Object components.      

## Goals

- Provide Rook-Ceph users the ability to dynamically provision object buckets via the Kubernetes API with a CephObjectBucketClaim Custom Resource Definition (CRD).
- Utilize a familiar and deterministic pattern when injecting bucket connection information into workload environments. 

## Non-Goals

- This proposal does not implement a generalized method for bucket provisioning; it will be specific to Rook-Ceph
- This design does not attempt to implement a 1:1 model of Kubernetes PersistentVolumes and PersistentVolumeClaims.  Some shallow similarities will exist. 
- This design does not include the Swift interface implementation by Ceph Object.
- This design does not provide users a means of deleting buckets via the Kubernetes API.  This creates a dangerous situation which could lead to accidental loss of data.  Only the Kubernetes API objects are deleted.

## Users

- Admin: A Kubernetes cluster administrator with permissions to instantiate and control access to cluster storage. The admin will use existing Rook-Ceph CRDs to create object stores and StorageClasses to make them accessible to users.  
- User: A Kubernetes cluster user with limited permissions, typically confined CRUD operations on Kubernetes objects within a single namespace.  The user will create request ceph object buckets by instantiating CephObjectBucketClaims. 

## Use Cases

### Assumptions

1. A Rook-Ceph operator deployed on a Kubernetes Cluster.
1. A CephObjectStore object exists along with the accompanying Service in a protected namespace.
1. An existing CephObjectStoreUser exists along with the accompanying Secret in the user's namespace.

##### Use Case: Expose an Object Store Endpoint for Bucket Provisioning

As an admin, I want to expose an existing object store to cluster users so they can begin bucket provisioning via the Kubernetes API._

![admin-actions](./cobc-admin.png)

1. The admin creates a StorageClass with `parameters[serviceName]=objectstore-service` and `parameters[serviceNamespace]=some-namespace`
  - _Note:_ the `provisioner` field is ignored.  Defining the provisioner is unnecessary as Rook-Ceph is the only operator watching for CephObjectBucketClaims.

**Use Case: Provision a Bucket** 

_As a Kubernetes user, I want to leverage the Kubernetes API to create Ceph Object Buckets. I expect to get back the bucket connection information in a Pod-attachable object._
 
![user-actions](./cobc-user.png)

0. The user creates a CephObjectBucketClaim in the user's namespace.
1. The Rook Operator detects a new ObjectBucketClaim instance.  
    1. The operator gets the user's credentials via `cephObjectBucketClaim.spec.secretName`.  
    1. The operator gets the object store URL via `storageClass.parameters.serviceNamespace` + `storageClass.parameters.serviceName`.
1. The operator uses the object store URL and credentials to make an S3 "make bucket" call.
1. The operator creates a ConfigMap in the namespace of the CephObjectBucketClaim with relevant connection data for that bucket.
1. An app Pod may then mount the Secret and the ConfigMap to begin accessing the bucket. 

#####Use Case: Delete an Object Bucket

_As a Kubernetes user, I want to delete ObjectBucketClaim instances and cleanup generated API resources._

1. The user deletes the CephObjectBucketClaim via `kubectl delete ...`.
1. The CephObjectBucketClaim is marked for deletion and left in the foreground.
1. The respective ConfigMap is deleted.
1. The CephObjectBucketClaim is garbage collected.

---

## API Specifications

### Custom Resource Definition

```yaml
apiVersion: apiextensions.k8s.io/v1beta1 
kind: CustomResourceDefinition
metadata:
  name: cephobjectbucketclaims.rook.io
spec:
  group: ceph.rook.io
  version: v1
  scope: namespaced
  names:
      kind: CephObjectBucketClaim
      listKind: CephObjectBucketClaimList
      singular: cephobjectbucketclaim
      plural: cephobjectbucketclaims
```

### Custom Resource

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectBucketClaim
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

1. Added by the Rook-Ceph operator.
1. The operator will append a hyphen followed by random characters to the string given here.

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-ceph-object-bucket-my-bucket-1
  namespace: dev-user
  labels:
    ceph.rook.io/claims:
  ownerReferences:  [1]
  - name: my-bucket-1
    uid: 1234-qwer-4321-rewq
    apiVersion: ceph.rook.io/v1
    kind: CephObjectBucketClaim
    blockOwnerDeletion: true 
data:
  CEPH_BUCKET_HOST: http://my-store-url-or-ip
  CEPH_BUCKET_NAME: my-bucket-1
  CEPH_BUCKET_PORT: 80
  CEPH_BUCKET_SSL: no
```

1. Treat the configMap as a child of the ObjectBucketClaim for garbage collection.

### StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: some-object-store
provisioner: "" [1]
parameters:
  serviceName: my-store
  serviceNamespace: some-namespace
```

1. Since CephObjectBucketClaim instances are only watched by Rook-Ceph operator, defining the provisioner is unnecessary; the field value is ignored.
