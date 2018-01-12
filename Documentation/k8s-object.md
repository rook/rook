---
title: Object Storage
weight: 13
indent: true
---

# Object Storage Quickstart

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](kubernetes.md)

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

Creating an object storage user requires running `rookctl` commands with the [Rook toolbox](kubernetes.md#tools) pod. This will be simplified in the future with a CRD for the object store users.

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
you will need another level of setup. In this example we will create an ingress controller. You could also create a service based on a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact.
```bash
$ kubectl -n rook get service rook-ceph-rgw-my-store
NAME                     CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
rook-ceph-rgw-my-store   10.3.0.177   <none>        80/TCP      2m
```

Save the ingress resource as `rgw-external.yaml`:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: rgw-my-store-external
  namespace: rook
  annotations:
    kubernetes.io/ingress.class: "nginx"
spec:
  rules:
  - host: my-host
    http:
      paths:
      - path: /
        backend:
          serviceName: rook-ceph-rgw-my-store
          servicePort: 80
```

Create the ingress resource.

```bash
kubectl create -f rgw-external.yaml
```

See the ingress controller:
```bash
$ kubectl -n rook get ingress
NAME                    HOSTS         ADDRESS   PORTS     AGE
rgw-my-store-external   my-host                 80        2s
```

If you are running in GCE, that is all you need. If running in other environments, you now need to start an ingress controller. The two popular ones are nginx and traefik, which run as pods on "ingress nodes". They listen on port 80/443 and are essentially reverse HTTP proxies which match domain names to deployment endpoints.

A simple way to deploy the nginx ingress is through the helm chart. First create the settings file and save as `nginx-ingress-values.yml`: 
```
rbac:
  create: true
controller:
  replicaCount: 1
  hostNetwork: true
```

Now install the helm chart.
```
$ helm install --values nginx-ingress-values.yml stable/nginx-ingress --namespace nginx-ingress
```

After the controller is running you can now create DNS records to point at the IP addresses of the nodes where the nginx pods are running. Now you have object store access for clients external to your cluster.
