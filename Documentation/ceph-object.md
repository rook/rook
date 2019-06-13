---
title: Object Storage
weight: 2200
indent: true
---

# Object Storage

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [Kubernetes guide](ceph-quickstart.md)

## Create an Object Store

**NOTE** This example requires you to have **at least 3 bluestore OSDs each on a different node**.
This is because the below `erasureCoded` chunk settings require at least 3 bluestore OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) to `host` (default), each OSD needs to be on a different nodes.

Now we will create the object store, which starts the RGW service in the cluster with the S3 API.
Specify your desired settings for the object store in the `object.yaml`. For more details on the settings see the [Object Store CRD](ceph-object-store-crd.md).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: host
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  gateway:
    type: s3
    sslCertificateRef:
    port: 80
    securePort:
    instances: 1
```

When the object store is created the Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.
```bash
# Create the object store
kubectl create -f object.yaml

# To confirm the object store is configured, wait for the rgw pod to start
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

## Create a User

Next we will create the object store user, which calls the RGW service in the cluster with the S3 API.
Specify your desired settings for the object store user in the `object-user.yaml`. For more details on the settings see the [Object Store User CRD](ceph-object-store-user-crd.md).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: "my display name"
```

When the object store user is created the Rook operator will create the RGW user on the object store specified, and store the Access Key and Secret Key in a kubernetes secret in the same namespace as the object store user.

```bash
# Create the object store user
kubectl create -f object-user.yaml

# To confirm the object store user is configured, describe the secret
kubectl -n rook-ceph describe secret rook-ceph-object-user-my-store-my-user

Name:		rook-ceph-object-user-my-store-my-user
Namespace:	rook-ceph
Labels:			app=rook-ceph-rgw
			      rook_cluster=rook-ceph
			      rook_object_store=my-store
Annotations:	<none>

Type:	kubernetes.io/rook

Data
====
AccessKey:	20 bytes
SecretKey:	40 bytes
```

The AccessKey and SecretKey data fields can be mounted in a pod as an environment variable. More information on consuming
kubernetes secrets can be found in the [K8s secret documentation](https://kubernetes.io/docs/concepts/configuration/secret/)

To directly retrieve the secrets:
```bash
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o yaml | grep AccessKey | awk '{print $2}' | base64 --decode
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o yaml | grep SecretKey | awk '{print $2}' | base64 --decode
```

## Consume the Object Storage

Use an S3 compatible client to create a bucket in the object store.

This section will allow you to test connecting to the object store and uploading and downloading from it. Run the following commands after you have connected to the [Rook toolbox](ceph-toolbox.md).

### Install s3cmd

To test the object store we will install the `s3cmd` tool into the toobox pod.
```bash
yum --assumeyes install s3cmd
```

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

The access key and secret key can be retrieved as described in the section above on [creating a user](#create-a-user).

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
   echo "Hello Rook" > /tmp/rookObj
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
