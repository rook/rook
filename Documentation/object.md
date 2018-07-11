---
title: Object Storage
weight: 24
indent: true
---

# Object Storage

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](quickstart.md)

## Create an Object Store

Now we will create the object store, which starts the RGW service in the cluster with the S3 API.
Specify your desired settings for the object store in the `object.yaml`. For more details on the settings see the [Object Store CRD](ceph-object-store-crd.md).

```yaml
apiVersion: ceph.rook.io/v1beta1
kind: ObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
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

When the object store is created the Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.
```bash
# Create the object store
kubectl create -f object.yaml

# To confirm the object store is configured, wait for the rgw pod to start
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

## Create a User

Creating an object storage user requires running a `radosgw-admin` command with the [Rook toolbox](quickstart.md#tools) pod. This will be simplified in the future with a CRD for the object store users.

```bash
radosgw-admin user create --uid rook-user --display-name "A rook rgw User" --rgw-realm=my-store --rgw-zonegroup=my-store
```

The object store is now available by using the creds of `rook-user`. Take note of the `access_key` and `secret_key` printed by the user creation. For example:
```json
{
    "user": "rook-user",
    "access_key": "XEZDB3UJ6X7HVBE7X7MA",
    "secret_key": "7yGIZON7EhFORz0I40BFniML36D2rl8CQQ5kXU6l"
}
```

## Consume the Object Storage

Use an S3 compatible client to create a bucket in the object store.

This section will allow you to test connecting to the object store and uploading and downloading from it. The `s3cmd` tool is included in the [Rook toolbox](toolbox.md) pod to simplify your testing. Run the following commands after you have connected to the toolbox.

### Connection Environment Variables

To simplify the s3 client commands, you will want to set the four environment variables for use by your client (ie. inside the toolbox):
```bash
export AWS_HOST=<host>
export AWS_ENDPOINT=<endpoint>
export AWS_ACCESS_KEY_ID=<accessKey>
export AWS_SECRET_ACCESS_KEY=<secretKey>
```

- `Host`: The DNS host name where the rgw service is found in the cluster. Assuming you are using the default `rook-ceph` cluster, it will be `rook-ceph-rgw-my-store.rook-ceph`.
- `Endpoint`: The endpoint where the rgw service is listening. Run `kubectl -n rook-ceph get svc rook-ceph-rgw-my-store`, then combine the clusterIP and the port.
- `Access key`: The user's `access_key` as printed above
- `Secret key`: The user's `secret_key` as printed above

The variables for the user generated in this example would be:
```bash
export AWS_HOST=rook-ceph-rgw-my-store.rook-ceph
export AWS_ENDPOINT=10.104.35.31:80
export AWS_ACCESS_KEY_ID=XEZDB3UJ6X7HVBE7X7MA
export AWS_SECRET_ACCESS_KEY=7yGIZON7EhFORz0I40BFniML36D2rl8CQQ5kXU6l
```

### Create a bucket

Now that the user connection variables were set above, we can proceed to perform operations such as creating buckets.

Create a bucket in the object store

   ```bash
   s3cmd mb --no-ssl --host=${AWS_HOST} --host-bucket=  s3://rookbucket
   ```

List buckets in the object store

   ```bash
   s3cmd ls --no-ssl --host=${AWS_HOST}
   ```

### PUT or GET an object

Upload a file to the newly created bucket

   ```bash
   echo "Hello Rook!" > /tmp/rookObj
   s3cmd put /tmp/rookObj --no-ssl --host=${AWS_HOST} --host-bucket=  s3://rookbucket
   ```

Download and verify the file from the bucket

   ```bash
   s3cmd get s3://rookbucket/rookObj /tmp/rookObj-download --no-ssl --host=${AWS_HOST} --host-bucket=
   cat /tmp/rookObj-download
   ```

## Access External to the Cluster

Rook sets up the object storage so pods will have access internal to the cluster. If your applications are running outside the cluster,
you will need to setup an external service through a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact and create a new service for external access.
```bash
$ kubectl -n rook-ceph get service rook-ceph-rgw-my-store
NAME                     CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
rook-ceph-rgw-my-store   10.3.0.177   <none>        80/TCP      2m
```

Save the external service as `rgw-external.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: rook-ceph-rgw-my-store-external
  namespace: rook-ceph
  labels:
    app: rook-ceph-rgw
    rook_cluster: rook-ceph
    rook_object_store: my-store
spec:
  ports:
  - name: rgw
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: rook-ceph-rgw
    rook_cluster: rook-ceph
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
$ kubectl -n rook-ceph get service rook-ceph-rgw-my-store rook-ceph-rgw-my-store-external
NAME                              TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)        AGE
rook-ceph-rgw-my-store            ClusterIP   10.104.82.228    <none>        80/TCP         4m
rook-ceph-rgw-my-store-external   NodePort    10.111.113.237   <none>        80:31536/TCP   39s
```

Internally the rgw service is running on port `80`. The external port in this case is `31536`. Now you can access the object store from anywhere! All you need is the hostname for any machine in the cluster, the external port, and the user credentials.
