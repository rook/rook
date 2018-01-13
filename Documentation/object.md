---
title: Object Storage
weight: 24
indent: true
---

# Object Storage

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](quickstart.md)

## Object Store

Now we will create the object store, which starts the RGW service in the cluster with the S3 API.
Specify your desired settings for the object store in the `rook-object.yaml`. For more details on the settings see the [Object Store CRD](object-store-crd.md).

```yaml
apiVersion: rook.io/v1alpha1
kind: ObjectStore
metadata:
  name: my-store
  namespace: rook
spec:
  metadataPool:
    replicated:
      size: 3
  dataPool:
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  gateway:
    type: s3
    sslCertificateRef: 
    port: 80
    securePort:
    instances: 1
    allNodes: false
```

### Kubernetes 1.6 or earlier

If you are using a version of Kubernetes earlier than 1.7, you will need to slightly modify one setting to be compatible with TPRs (deprecated in 1.7). Notice the different casing.
```yaml
kind: Objectstore
```

### Create the Object Store

Now let's create the object store. The Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.
```bash
# Create the object store
kubectl create -f rook-object.yaml

# To confirm the object store is configured, wait for the rgw pod to start
kubectl -n rook get pod -l app=rook-ceph-rgw
```

## Create a User

Creating an object storage user requires running `rookctl` commands with the [Rook toolbox](quickstart.md#tools) pod. This will be simplified in the future with a CRD for the object store users.

```bash
rookctl object user create my-store rook-user "A rook rgw User"
```

The object store is now available by using the creds of `rook-user`.

## Environment Variables

If your s3 client uses environment variables, the client can print them for you
```bash
rookctl object connection my-store rook-user --format env-var
```

See the [Object Storage](client.md#object-storage) documentation for more steps on consuming the object storage.

## Access External to the Cluster

Rook sets up the object storage so pods will have access internal to the cluster. If your applications are running outside the cluster,
you will need to setup an external service through a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact and create a new service for external access.
```bash
$ kubectl -n rook get service rook-ceph-rgw-my-store
NAME                     CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
rook-ceph-rgw-my-store   10.3.0.177   <none>        80/TCP      2m
```

Save the external service as `rgw-external.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: rook-ceph-rgw-my-store-external
  namespace: rook
  labels:
    app: rook-ceph-rgw
    rook_cluster: rook
    rook_object_store: my-store
spec:
  ports:
  - name: rgw
    port: 53390
    protocol: TCP
    targetPort: 53390
  selector:
    app: rook-ceph-rgw
    rook_cluster: rook
    rook_object_store: my-store
  sessionAffinity: None
  type: NodePort
```

Now create the external service.

```bash
kubectl create -f rgw-external.yaml
```

See both rgw services running and notice what port the external service is running on:
```bash
$ kubectl -n rook get service rook-ceph-rgw-my-store rook-ceph-rgw-my-store-external
NAME                              CLUSTER-IP   EXTERNAL-IP   PORT(S)           AGE
rook-ceph-rgw-my-store            10.0.0.83    <none>        80/TCP            21m
rook-ceph-rgw-my-store-external   10.0.0.26    <nodes>       53390:30041/TCP   1m
```

Internally the rgw service is running on port `53390`. The external port in this case is `30041`. Now you can access the object store from anywhere! All you need is the hostname for any machine in the cluster, the external port, and the user credentials.
