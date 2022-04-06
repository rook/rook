---
title: Object Storage
weight: 2200
indent: true
---

# Object Storage

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](quickstart.md).

## Configure an Object Store

Rook has the ability to either deploy an object store in Kubernetes or to connect to an external RGW service.
Most commonly, the object store will be configured locally by Rook.
Alternatively, if you have an existing Ceph cluster with Rados Gateways, see the
[external section](#connect-to-an-external-object-store) to consume it from Rook.

### Create a Local Object Store

The below sample will create a `CephObjectStore` that starts the RGW service in the cluster with an S3 API.

> **NOTE**: This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

See the [Object Store CRD](ceph-object-store-crd.md#object-store-settings), for more detail on the settings available for a `CephObjectStore`.

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
  preservePoolsOnDelete: true
  gateway:
    sslCertificateRef:
    port: 80
    # securePort: 443
    instances: 1
  healthCheck:
    bucket:
      disabled: false
      interval: 60s
```

After the `CephObjectStore` is created, the Rook operator will then create all the pools and other resources necessary to start the service. This may take a minute to complete.

```console
# Create the object store
kubectl create -f object.yaml

# To confirm the object store is configured, wait for the rgw pod to start
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

### Connect to an External Object Store

Rook can connect to existing RGW gateways to work in conjunction with the external mode of the `CephCluster` CRD.
If you have an external `CephCluster` CR, you can instruct Rook to consume external gateways with the following:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: external-store
  namespace: rook-ceph
spec:
  gateway:
    port: 8080
    externalRgwEndpoints:
      - ip: 192.168.39.182
  healthCheck:
    bucket:
      enabled: true
      interval: 60s
```

You can use the existing `object-external.yaml` file.
When ready the ceph-object-controller will output a message in the Operator log similar to this one:

>```
>ceph-object-controller: ceph object store gateway service >running at 10.100.28.138:8080
>```

You can now get and access the store via:

```console
kubectl -n rook-ceph get svc -l app=rook-ceph-rgw
```

>```
>NAME                     TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
>rook-ceph-rgw-my-store   ClusterIP   10.100.28.138   <none>        8080/TCP   6h59m
>```

Any pod from your cluster can now access this endpoint:

```console
$ curl 10.100.28.138:8080
```

>```
><?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
>```

It is also possible to use the internally registered DNS name:

```console
curl rook-ceph-rgw-my-store.rook-ceph:8080
```

```console
<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
```

The DNS name is created  with the following schema `rook-ceph-rgw-$STORE_NAME.$NAMESPACE`.

## Create a Bucket

Now that the object store is configured, next we need to create a bucket where a client can read and write objects. A bucket can be created by defining a storage class, similar to the pattern used by block and file storage.
First, define the storage class that will allow object clients to create a bucket.
The storage class defines the object storage system, the bucket retention policy, and other properties required by the administrator. Save the following as `storageclass-bucket-delete.yaml` (the example is named as such due to the `Delete` reclaim policy).

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-ceph-bucket
# Change "rook-ceph" provisioner prefix to match the operator namespace if needed
provisioner: rook-ceph.ceph.rook.io/bucket
reclaimPolicy: Delete
parameters:
  objectStoreName: my-store
  objectStoreNamespace: rook-ceph
```
If you’ve deployed the Rook operator in a namespace other than `rook-ceph`, change the prefix in the provisioner to match the namespace you used. For example, if the Rook operator is running in the namespace `my-namespace` the provisioner value should be `my-namespace.ceph.rook.io/bucket`.
```console
kubectl create -f storageclass-bucket-delete.yaml
```

Based on this storage class, an object client can now request a bucket by creating an Object Bucket Claim (OBC).
When the OBC is created, the Rook-Ceph bucket provisioner will create a new bucket. Notice that the OBC
references the storage class that was created above.
Save the following as `object-bucket-claim-delete.yaml` (the example is named as such due to the `Delete` reclaim policy):

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-bucket
spec:
  generateBucketName: ceph-bkt
  storageClassName: rook-ceph-bucket
```

```console
kubectl create -f object-bucket-claim-delete.yaml
```

Now that the claim is created, the operator will create the bucket as well as generate other artifacts to enable access to the bucket. A secret and ConfigMap are created with the same name as the OBC and in the same namespace.
The secret contains credentials used by the application pod to access the bucket.
The ConfigMap contains bucket endpoint information and is also consumed by the pod.
See the [Object Bucket Claim Documentation](ceph-object-bucket-claim.md) for more details on the `CephObjectBucketClaims`.

### Client Connections

The following commands extract key pieces of information from the secret and configmap:"

```bash
#config-map, secret, OBC will part of default if no specific name space mentioned
export AWS_HOST=$(kubectl -n default get cm ceph-bucket -o jsonpath='{.data.BUCKET_HOST}')
export PORT=$(kubectl -n default get cm ceph-bucket -o jsonpath='{.data.BUCKET_PORT}')
export BUCKET_NAME=$(kubectl -n default get cm ceph-bucket -o jsonpath='{.data.BUCKET_NAME}')
export AWS_ACCESS_KEY_ID=$(kubectl -n default get secret ceph-bucket -o jsonpath='{.data.AWS_ACCESS_KEY_ID}' | base64 --decode)
export AWS_SECRET_ACCESS_KEY=$(kubectl -n default get secret ceph-bucket -o jsonpath='{.data.AWS_SECRET_ACCESS_KEY}' | base64 --decode)
```

## Consume the Object Storage

Now that you have the object store configured and a bucket created, you can consume the
object storage from an S3 client.

This section will guide you through testing the connection to the `CephObjectStore` and uploading and downloading from it.
Run the following commands after you have connected to the [Rook toolbox](ceph-toolbox.md).

### Connection Environment Variables

To simplify the s3 client commands, you will want to set the four environment variables for use by your client (ie. inside the toolbox).
See above for retrieving the variables for a bucket created by an `ObjectBucketClaim`.

```bash
export AWS_HOST=<host>
export PORT=<port>
export AWS_ACCESS_KEY_ID=<accessKey>
export AWS_SECRET_ACCESS_KEY=<secretKey>
```

* `Host`: The DNS host name where the rgw service is found in the cluster. Assuming you are using the default `rook-ceph` cluster, it will be `rook-ceph-rgw-my-store.rook-ceph.svc`.
* `Port`: The endpoint where the rgw service is listening. Run `kubectl -n rook-ceph get svc rook-ceph-rgw-my-store`, to get the port.
* `Access key`: The user's `access_key` as printed above
* `Secret key`: The user's `secret_key` as printed above

The variables for the user generated in this example might be:

```bash
export AWS_HOST=rook-ceph-rgw-my-store.rook-ceph.svc
export PORT=80
export AWS_ACCESS_KEY_ID=XEZDB3UJ6X7HVBE7X7MA
export AWS_SECRET_ACCESS_KEY=7yGIZON7EhFORz0I40BFniML36D2rl8CQQ5kXU6l
```

The access key and secret key can be retrieved as described in the section above on [client connections](#client-connections) or
below in the section [creating a user](#create-a-user) if you are not creating the buckets with an `ObjectBucketClaim`.

### Configure s5cmd

To test the `CephObjectStore`, set the object store credentials in the toolbox pod for the `s5cmd` tool.

```console
mkdir ~/.aws
cat > ~/.aws/credentials << EOF
[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}
EOF
```

### PUT or GET an object

Upload a file to the newly created bucket

```console
echo "Hello Rook" > /tmp/rookObj
s5cmd --endpoint-url http://$AWS_HOST:$PORT cp /tmp/rookObj s3://$BUCKET_NAME
```

Download and verify the file from the bucket

```console
s5cmd --endpoint-url http://$AWS_HOST:$PORT cp s3://$BUCKET_NAME/rookObj /tmp/rookObj-download
cat /tmp/rookObj-download
```

## Access External to the Cluster

Rook sets up the object storage so pods will have access internal to the cluster. If your applications are running outside the cluster,
you will need to setup an external service through a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact and create a new service for external access.

```console
kubectl -n rook-ceph get service rook-ceph-rgw-my-store
```

>```
>NAME                     CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
>rook-ceph-rgw-my-store   10.3.0.177   <none>        80/TCP      2m
>```

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

```console
kubectl create -f rgw-external.yaml
```

See both rgw services running and notice what port the external service is running on:

```console
kubectl -n rook-ceph get service rook-ceph-rgw-my-store rook-ceph-rgw-my-store-external
```

>```
>NAME                              TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)        AGE
>rook-ceph-rgw-my-store            ClusterIP   10.104.82.228    <none>        80/TCP         4m
>rook-ceph-rgw-my-store-external   NodePort    10.111.113.237   <none>        80:31536/TCP   39s
>```

Internally the rgw service is running on port `80`. The external port in this case is `31536`. Now you can access the `CephObjectStore` from anywhere! All you need is the hostname for any machine in the cluster, the external port, and the user credentials.

## Create a User

If you need to create an independent set of user credentials to access the S3 endpoint,
create a `CephObjectStoreUser`. The user will be used to connect to the RGW service in the cluster using the S3 API.
The user will be independent of any object bucket claims that you might have created in the earlier
instructions in this document.

See the [Object Store User CRD](ceph-object-store-user-crd.md) for more detail on the settings available for a `CephObjectStoreUser`.

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

When the `CephObjectStoreUser` is created, the Rook operator will then create the RGW user on the specified `CephObjectStore` and store the Access Key and Secret Key in a kubernetes secret in the same namespace as the `CephObjectStoreUser`.

```console
# Create the object store user
kubectl create -f object-user.yaml
```

```console
# To confirm the object store user is configured, describe the secret
kubectl -n rook-ceph describe secret rook-ceph-object-user-my-store-my-user
```

>```
>Name:		rook-ceph-object-user-my-store-my-user
>Namespace:	rook-ceph
>Labels:			app=rook-ceph-rgw
>			      rook_cluster=rook-ceph
>			      rook_object_store=my-store
>Annotations:	<none>
>
>Type:	kubernetes.io/rook
>
>Data
>====
>AccessKey:	20 bytes
>SecretKey:	40 bytes
>```

The AccessKey and SecretKey data fields can be mounted in a pod as an environment variable. More information on consuming
kubernetes secrets can be found in the [K8s secret documentation](https://kubernetes.io/docs/concepts/configuration/secret/)

To directly retrieve the secrets:

```console
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o jsonpath='{.data.AccessKey}' | base64 --decode
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o jsonpath='{.data.SecretKey}' | base64 --decode
```

## Object Multisite

Multisite is a feature of Ceph that allows object stores to replicate its data over multiple Ceph clusters.

Multisite also allows object stores to be independent and isloated from other object stores in a cluster.

For more information on multisite please read the [ceph multisite overview](ceph-object-multisite.md) for how to run it.
