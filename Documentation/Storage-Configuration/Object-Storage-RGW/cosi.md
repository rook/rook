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
```

```console
cd deploy/examples/cosi
kubectl create -f cephcosidriver.yaml
```

The driver is created in the same namespace as Rook operator.

## Admin Operations

### Create a Ceph Object Store User

Create a CephObjectStoreUser to be used by the COSI driver for provisioning buckets.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: cosi
  namespace: rook-ceph
spec:
  displayName: "cosi user"
  store: my-store
  capabilities:
    bucket: "*"
    user: "*"
```

```console
kubectl create -f cosi-user.yaml
```

Above step will be automated in future by the Rook operator.

### Create a BucketClass and BucketAccessClass

The BucketClass and BucketAccessClass are CRDs defined by COSI. The BucketClass defines the storage class for the bucket. The BucketAccessClass defines the access class for the bucket. The BucketClass and BucketAccessClass are defined as below:

```yaml
kind: BucketClass
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-bcc
driverName: ceph.objectstorage.k8s.io
deletionPolicy: Delete
parameters:
  objectStoreUserSecretName: rook-ceph-object-user-my-store-cosi
  objectStoreUserSecretNamespace: rook-ceph
---
kind: BucketAccessClass
apiVersion: objectstorage.k8s.io/v1alpha1
metadata:
  name: sample-bac
driverName: ceph.objectstorage.k8s.io
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
kubectl get secret sample-secret-name -o yaml
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

Another approach is the json data can be parsed by the application to access the bucket via init container. Following is a sample init container which parses the json data and creates a file with the access details:

``` bash
set -e

jsonfile=%s

if [ -d "$jsonfile" ]; then
    export ENDPOINT=$(jq -r '.spec.secretS3.endpoint' $jsonfile)
    export BUCKET=$(jq -r '.spec.bucketName' $jsonfile)
    export AWS_ACCESS_KEY_ID=$(jq -r '.spec.secretS3.accessKeyID' $jsonfile)
    export AWS_SECRET_ACCESS_KEY=$(jq -r '.spec.secretS3.accessSecretKey' $jsonfile)
fi
else
    echo "Error: $jsonfile does not exist"
    exit 1
fi

```

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: sample-app
  namespace: rook-ceph
spec:
  containers:
  - name: sample-app
    image: busybox
    command: ["/bin/sh", "-c", "sleep 3600"]
    volumeMounts:
    - name: cosi-secrets
      mountPath: /data/cosi
  initContainers:
  - name: init-cosi
    image: busybox
    command: ["/bin/sh", "-c", "setup-aws-credentials /data/cosi/BucketInfo/credentials"]
    volumeMounts:
    - name: cosi-secrets
      mountPath: /data/cosi
  volumes:
  - name: cosi-secrets
    secret:
      #  Set the name of the secret from the BucketAccess
      secretName: sample-secret-name
```
