---
title: Object Storage Overview
---

Object storage exposes an S3 API to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](../../Getting-Started/quickstart.md).

## Configure an Object Store

Rook has the ability to either deploy an object store in Kubernetes or to connect to an external RGW service.
Most commonly, the object store will be configured locally by Rook.
Alternatively, if you have an existing Ceph cluster with Rados Gateways, see the
[external section](#connect-to-an-external-object-store) to consume it from Rook.

### Create a Local Object Store

The below sample will create a `CephObjectStore` that starts the RGW service in the cluster with an S3 API.

!!! note
    This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

See the [Object Store CRD](../../CRDs/Object-Storage/ceph-object-store-crd.md#object-store-settings), for more detail on the settings available for a `CephObjectStore`.

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
```

After the `CephObjectStore` is created, the Rook operator will then create all the pools and other resources necessary to start the service. This may take a minute to complete.

Create an object store:

```console
kubectl create -f object.yaml
```

To confirm the object store is configured, wait for the RGW pod(s) to start:

```console
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

### Connect to an External Object Store

Rook can connect to existing RGW gateways to work in conjunction with the external mode of the `CephCluster` CRD. First, create a `rgw-admin-ops-user` user in the Ceph cluster with the necessary caps:

```console
radosgw-admin user create --uid=rgw-admin-ops-user --display-name="RGW Admin Ops User" --caps="buckets=*;users=*;usage=read;metadata=read;zone=read" --rgw-realm=<realm-name> --rgw-zonegroup=<zonegroup-name> --rgw-zone=<zone-name>
```

The `rgw-admin-ops-user` user is required by the Rook operator to manage buckets and users via the admin ops and s3 api. The multisite configuration needs to be specified only if the admin sets up multisite for RGW.

Then create a secret with the user credentials:

```console
kubectl -n rook-ceph create secret generic --type="kubernetes.io/rook" rgw-admin-ops-user --from-literal=accessKey=<access key of the user> --from-literal=secretKey=<secret key of the user>
```

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
        # hostname: example.com
```

Use the existing `object-external.yaml` file. Even though multiple endpoints can be specified, it is recommend to use only one endpoint. This endpoint is randomly added to `configmap` of OBC and secret of the `cephobjectstoreuser`. Rook never guarantees the randomly picked endpoint is a working one or not.
If there are multiple endpoints, please add load balancer in front of them and use the load balancer endpoint in the `externalRgwEndpoints` list.

When ready, the message in the `cephobjectstore` status similar to this one:

```console
kubectl -n rook-ceph get cephobjectstore external-store
NAME                                 PHASE
external-store                       Ready

```

Any pod from your cluster can now access this endpoint:

```console
$ curl 10.100.28.138:8080
<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>anonymous</ID><DisplayName></DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>
```

## Create a Bucket

!!! info
    This document is a guide for creating bucket with an Object Bucket Claim (OBC). To create a bucket with the experimental COSI Driver, see the [COSI documentation](cosi.md).

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
When the OBC is created, the Rook bucket provisioner will create a new bucket. Notice that the OBC
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

```console
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
Run the following commands after you have connected to the [Rook toolbox](../../Troubleshooting/ceph-toolbox.md).

### Connection Environment Variables

To simplify the s3 client commands, you will want to set the four environment variables for use by your client (ie. inside the toolbox).
See above for retrieving the variables for a bucket created by an `ObjectBucketClaim`.

```console
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

```console
export AWS_HOST=rook-ceph-rgw-my-store.rook-ceph.svc
export PORT=80
export AWS_ACCESS_KEY_ID=XEZDB3UJ6X7HVBE7X7MA
export AWS_SECRET_ACCESS_KEY=7yGIZON7EhFORz0I40BFniML36D2rl8CQQ5kXU6l
```

The access key and secret key can be retrieved as described in the section above on [client connections](#client-connections) or
below in the section [creating a user](#create-a-user) if you are not creating the buckets with an `ObjectBucketClaim`.

### Configure s5cmd

To test the `CephObjectStore`, set the object store credentials in the toolbox pod that contains the `s5cmd` tool.

!!! important
    The default toolbox.yaml does not contain the s5cmd. The toolbox must be started with the rook operator image (toolbox-operator-image), which does contain s5cmd.

```console
kubectl create -f deploy/examples/toolbox-operator-image.yaml
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

## Monitoring health

Rook configures health probes on the deployment created for CephObjectStore gateways. Refer to
[the CRD document](../../CRDs/Object-Storage/ceph-object-store-crd.md#health-settings) for
information about configuring the probes and monitoring the deployment status.

## Access External to the Cluster

Rook sets up the object storage so pods will have access internal to the cluster. If your applications are running outside the cluster,
you will need to setup an external service through a `NodePort`.

First, note the service that exposes RGW internal to the cluster. We will leave this service intact and create a new service for external access.

```console
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

```console
kubectl create -f rgw-external.yaml
```

See both rgw services running and notice what port the external service is running on:

```console
$ kubectl -n rook-ceph get service rook-ceph-rgw-my-store rook-ceph-rgw-my-store-external
NAME                              TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)        AGE
rook-ceph-rgw-my-store            ClusterIP   10.104.82.228    <none>        80/TCP         4m
rook-ceph-rgw-my-store-external   NodePort    10.111.113.237   <none>        80:31536/TCP   39s
```

Internally the rgw service is running on port `80`. The external port in this case is `31536`. Now you can access the `CephObjectStore` from anywhere! All you need is the hostname for any machine in the cluster, the external port, and the user credentials.

## Create a User

If you need to create an independent set of user credentials to access the S3 endpoint,
create a `CephObjectStoreUser`. The user will be used to connect to the RGW service in the cluster using the S3 API.
The user will be independent of any object bucket claims that you might have created in the earlier
instructions in this document.

See the [Object Store User CRD](../../CRDs/Object-Storage/ceph-object-store-user-crd.md) for more detail on the settings available for a `CephObjectStoreUser`.

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
$ kubectl -n rook-ceph describe secret rook-ceph-object-user-my-store-my-user
Name:    rook-ceph-object-user-my-store-my-user
Namespace:  rook-ceph
Labels:     app=rook-ceph-rgw
            rook_cluster=rook-ceph
            rook_object_store=my-store
Annotations:  <none>

Type: kubernetes.io/rook

Data
====
AccessKey:  20 bytes
SecretKey:  40 bytes
```

The AccessKey and SecretKey data fields can be mounted in a pod as an environment variable. More information on consuming
kubernetes secrets can be found in the [K8s secret documentation](https://kubernetes.io/docs/concepts/configuration/secret/)

To directly retrieve the secrets:

```console
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o jsonpath='{.data.AccessKey}' | base64 --decode
kubectl -n rook-ceph get secret rook-ceph-object-user-my-store-my-user -o jsonpath='{.data.SecretKey}' | base64 --decode
```

## Object Multisite

Multisite is a feature of Ceph that allows object stores to replicate its data over multiple Ceph clusters.

Multisite also allows object stores to be independent and isolated from other object stores in a cluster.

For more information on multisite please read the [ceph multisite overview](ceph-object-multisite.md) for how to run it.
