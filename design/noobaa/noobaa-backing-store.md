♜ [Rook NooBaa Design](README.md) /
# NooBaaBackingStore

NooBaaBackingStore is a CRD representing a storage target to be used as underlying storage for the data in NooBaa buckets.
These storage targets are used to store deduped+compressed+encrypted chunks of data (encryption keys are stored separatly).
Backing-stores are referred to by name when defining [NooBaaBucketClass](noobaa-bucket-class.md).

Multiple types of backing-stores are supported: aws-s3, s3-compatible, google-cloud-storage, azure-blob, obc, pvc.
Adding support for a new type of backing-store is rather easy as it requires just GET/PUT key-value store, see [Backing-stores supported by NooBaa](https://github.com/noobaa/noobaa-core/tree/master/src/agent/block_store_services).


# Definitions

- CRD: [noobaa-backing-store-crd.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-crd.yaml)
- Examples:
  1. [noobaa-backing-store-example-aws-s3.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-aws-s3.yaml)
  1. [noobaa-backing-store-example-s3-compatible.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-s3-compatible.yaml)
  1. [noobaa-backing-store-example-google-cloud-storage.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-google-cloud-storage.yaml)
  1. [noobaa-backing-store-example-azure-blob.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-azure-blob.yaml)
  1. [noobaa-backing-store-example-obc.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-obc.yaml)
  1. [noobaa-backing-store-example-pvc.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-backing-store-example-pvc.yaml)


# Reconcile

#### OBC type

The operator will create a claim and the appropriate provisioner will create a new bucket or connect to existing one depending on the obc options. Once the claim is ready its details will be used to configure a cloud resource in NooBaa.

#### PVC type

Create a NooBaa storage agent StatefulSet with PVC mounted in each pod. Each agent will connect to the NooBaa brain and provide the PV filesystem storage to be used for storing encrypted chunks of data.

#### Credentials change

In case the credentials of a backing-store need to be updated due to a periodic security policy or concern, the appropriate secret should be updated by the user, and the operator will be responsible for watching changes in those secrets and propagating the new credential update to the NooBaa system server.


# Read Status

Here is an example healthy status (see below example of non-healthy status):

```yaml
apiVersion: noobaa.rook.io/v1alpha1
kind: NooBaaBackingStore
metadata:
  name: aws-s3
  namespace: rook-noobaa
spec:
  ...
status:
  health: OK
  issues: []
```


# Delete

Backing-stores are used for data persistency, therefore there is a cleanup process before they can be deleted.
The operator will use the `finalizer` pattern as explained in the link below, and set a finalizer on every backing-store to mark that external cleanup is needed before it can be delete:

https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#finalizers

After marking a backing-store for deletion, the operator will notify the NooBaa server on the deletion which will enter a *decommissioning* state, in which NooBaa will attempt to rebuild the data to a new backing-store location. Once the decomissioning process completes the operator will remove the finalizer and allow the CR to be deleted.

There are cases where the decommissioning cannot complete due to inability to read the data from the backing-store that is already not serving - for example if the target bucket was already deleted or the credentials were invalidated or there is no network from the system to the backing-store service. In such cases the system status will be used to report these issues and suggest manual resolution for example:

```yaml
apiVersion: noobaa.rook.io/v1alpha1
kind: NooBaaBackingStore
metadata:
  name: aws-s3
  namespace: rook-noobaa
  finalizers:
    - finalizer.noobaa.rook.io
spec:
  ...
status:
  health: WARNING
  issues:
    - title: Backing-Store "aws" - Target bucket is missing / access denied
      createTime: "2019-06-04T13:05:35.473Z"
      lastTime: "2019-06-04T13:05:35.473Z"
    - title: Backing-Store "aws" - Cannot remove `finalizer.noobaa.rook.io` to complete deletion until the data rebuild process completes
      createTime: "2019-06-04T13:05:35.473Z"
      lastTime: "2019-06-04T13:05:35.473Z"
      troubleshooting: "https://github.com/noobaa/noobaa-core/wiki/Backing-store-finalizer-troubleshooting"
```
