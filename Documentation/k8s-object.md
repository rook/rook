# Object Storage Quickstart

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

### Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](kubernetes.md)

## Rook Client
Setting up the object storage requires running `rookctl` commands with the [rook-client](kubernetes.md#rook-client) pod. This will be simplified in the future with a TPR for the object stores.

## Create the Object Store and User
Now we will create the object store, which starts the RGW service in the cluster with the S3 API. 
From within the rook-client container, run the following:

```bash
# Create an object storage instance in the cluster
rookctl object create

# Create an object storage user. The first user may take a minute to create. 
# If it times out, run the same command again to confirm that it finished.
rookctl object user create rook-user "A rook rgw User"
```

The object store is now available for pods to connect by using the creds of `rook-user`. 

### Environment Variables
If your s3 client uses environment variables, the client can print them for you
```bash
rookctl object connection rook-user --format env-var
```

See the [Object Storage](client.md#object-storage) documentation for more steps on consuming the object storage.

## Access External to the Cluster

Rook sets up the object storage so pods will have access internal to the cluster. If your applications are running outside the cluster,
you will need to setup an external service through a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact and create a new service for external access.
```bash
$ kubectl -n rook get service rgw
NAME      CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
rgw       10.3.0.248   <none>        53390/TCP   45s
```

Now create the external service:
```bash
cd demo/kubernetes
kubectl create -f rgw-external.yaml
```

See both rgw services running and notice what port the external service is running on:
```bash
$ kubectl -n rook get service rgw rgw-external
NAME           CLUSTER-IP   EXTERNAL-IP   PORT(S)           AGE
rgw            10.3.0.248   <none>        53390/TCP         1m
rgw-external   10.3.0.146   <nodes>       53390:30711/TCP   1m
```

Internally the rgw service is running on port `53390`. The external port in this case is `30711`. Now you can access the object store from anywhere! All you need is the hostname for any machine in the cluster, the external port, and the user credentials.

If you're testing on the [coreos-kubernetes vagrant environment](k8s-pre-reqs.md#new-local-kubernetes-cluster), you can verify it is working from your host:
- If running in the single-node cluster:
  - `curl 172.17.4.99:30711`
- If running in the multi-node cluster:
  - `curl 172.17.4.101:30711`
