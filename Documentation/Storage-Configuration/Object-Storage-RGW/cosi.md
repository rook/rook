---
title: Container Object Storage Interface (COSI)
---

The Ceph COSI driver provisions buckets for object storage. This document instructs on enabling the driver and consuming a bucket from a sample application.

!!! note
    The Ceph COSI driver is currently in experimental mode.

## Prerequisites

COSI requires:

1. A running Rook [object store](object-storage.md)
2. [COSI controller](https://github.com/kubernetes-sigs/container-object-storage-interface-controller#readme)

Deploy the COSI controller with these commands:

```bash
kubectl apply -k github.com/kubernetes-sigs/container-object-storage-interface-api
kubectl apply -k github.com/kubernetes-sigs/container-object-storage-interface-controller
```

## Ceph COSI Driver

The Ceph COSI driver will be started when the CephCOSIDriver CR is created and when the first CephObjectStore is created.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCOSIDriver
metadata:
  name: ceph-cosi-driver
  namespace: rook-ceph
spec:
  deploymentStrategy: "Auto"
---
# The Ceph-COSI driver needs a privileged user for each CephObjectStore
# in order to provision buckets and users
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: cosi
  namespace: rook-ceph # rook operator namespace
spec:
  displayName: "cosi user"
  store: my-store # name of the CephObjectStore
  capabilities:
    bucket: "*"
    user: "*"
```

```console
cd deploy/examples/cosi
kubectl create -f cephcosidriver.yaml
```

## Admin Operations

### Create a BucketClass and BucketAccessClass

The BucketClass and BucketAccessClass are CRDs defined by COSI. The BucketClass defines the storage class for the bucket. The BucketAccessClass defines the access class for the bucket. The BucketClass and BucketAccessClass are defined as below:

```yaml
kind: BucketClass
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-bcc
driverName: rook-ceph.ceph.objectstorage.k8s.io
deletionPolicy: Delete
parameters:
  objectStoreUserSecretName: rook-ceph-object-user-my-store-cosi
  objectStoreUserSecretNamespace: rook-ceph
```

```yaml
kind: BucketAccessClass
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-bac
driverName: rook-ceph.ceph.objectstorage.k8s.io
authenticationType: KEY
parameters:
  objectStoreUserSecretName: rook-ceph-object-user-my-store-cosi
  objectStoreUserSecretNamespace: rook-ceph
```

```console
kubectl create -f bucketclass.yaml -f bucketaccessclass.yaml
```

The `objectStoreUserSecretName` and `objectStoreUserSecretNamespace` are the name and namespace of the CephObjectStoreUser created in the previous step.

## User Operations

### Create a Bucket

To create a bucket, use the BucketClass to pointing the required object store and then define BucketClaim request as below:

```yaml
kind: BucketClaim
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-bc
  namespace: default # any namespace can be used
spec:
  bucketClassName: sample-bcc
  protocols:
    - s3
```

```console
kubectl create -f bucketclaim.yaml
```

### Bucket Access

Define access to the bucket by creating the BucketAccess resource:

```yaml
kind: BucketAccess
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-access
  namespace: default # any namespace can be used
spec:
  bucketAccessClassName: sample-bac
  bucketClaimName: sample-bc
  protocol: s3
  # Change to the name of the secret where access details are stored
  credentialsSecretName: sample-secret-name
```

```console
kubectl create -f bucketaccess.yaml
```

The secret will be created which contains the access details for the bucket in JSON format in the namespace of BucketAccess:

``` console
kubectl get secret sample-secret-name -o jsonpath='{.data.BucketInfo}' | base64 -d
```

```json
{
  "metadata": {
    "name": "bc-81733d1a-ac7a-4759-96f3-fbcc07c0cee9",
    "creationTimestamp": null
  },
  "spec": {
    "bucketName": "sample-bcc1fc94b04-6011-45e0-a3d8-b6a093055783",
    "authenticationType": "KEY",
    "secretS3": {
      "endpoint": "http://rook-ceph-rgw-my-store.rook-ceph.svc:80",
      "region": "us-east",
      "accessKeyID": "LI2LES8QMR9GB5SZLB02",
      "accessSecretKey": "s0WAmcn8N1eIBgNV0mjCwZWQmJiCF4B0SAzbhYCL"
    },
    "secretAzure": null,
    "protocols": [
      "s3"
    ]
  }
}
```

### Consuming the Bucket via secret

To access the bucket from an application pod, mount the secret for accessing the bucket:

```yaml
  volumes:
  - name: cosi-secrets
    secret:
      #  Set the name of the secret from the BucketAccess
      secretName: sample-secret-name
  spec:
    containers:
    - name: sample-app
      volumeMounts:
      - name: cosi-secrets
        mountPath: /data/cosi
```

The Secret will be mounted in the pod in the path: `/data/cosi/BucketInfo`. The app must parse the JSON object to load the bucket connection details.
