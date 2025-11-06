---
title: Object Storage Overview
---

Object storage exposes an S3 API and or a [Swift API](https://developer.openstack.org/api-ref/object-store/index.html) to the storage cluster for applications to put and get data.

## Prerequisites

This guide assumes a Rook cluster as explained in the [Quickstart](../../Getting-Started/quickstart.md).

## Configure an Object Store

Rook can configure the Ceph Object Store for several different scenarios. See each linked section for the configuration details.

1. Create a [local object store](#create-a-local-object-store-with-s3) with dedicated Ceph pools. This option is recommended if a single object store is required, and is the simplest to get started.
2. Create [one or more object stores with shared Ceph pools](#create-local-object-stores-with-shared-pools). This option is recommended when multiple object stores are required.
3. Create [one or more object stores with pool placement targets and storage classes](#create-local-object-stores-with-pool-placements). This configuration allows Rook to provide different object placement options to object store clients.
4. Connect to an [RGW service in an external Ceph cluster](#connect-to-an-external-object-store), rather than create a local object store.
5. Configure [RGW Multisite](#object-multisite) to synchronize buckets between object stores in different clusters.
6. Create a [multi-instance RGW setup](#object-multi-instance). This option allows to have multiple `CephObjectStore` with different configurations backed by the same storage pools. For example, serving S3, Swift, or Admin-ops API by separate RGW instances.

!!! note
    Updating the configuration of an object store between these types is not supported.

Rook has the ability to either deploy an object store in Kubernetes or to connect to an external RGW service.
Most commonly, the object store will be configured in Kubernetes by Rook.
Alternatively see the [external section](#connect-to-an-external-object-store) to consume an existing Ceph cluster with [Rados Gateways](https://docs.ceph.com/en/latest/radosgw/index.html) from Rook.

### Create a Local Object Store with S3

The below sample will create a `CephObjectStore` that starts the RGW service in the cluster with an S3 API.

!!! note
    This sample requires *at least 3 OSDs*, with each OSD located on a *different node*.

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
    # For production it is recommended to use more chunks, such as 4+2 or 8+4
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  preservePoolsOnDelete: true
  gateway:
    # sslCertificateRef:
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

To consume the object store, continue below in the section to [Create a bucket](#create-a-bucket).

### Create Local Object Store(s) with Shared Pools

The below sample will create one or more object stores. Shared Ceph pools will be created, which reduces the overhead of additional Ceph pools for each additional object store.

Data isolation is enforced between the object stores with the use of Ceph RADOS namespaces. The separate RADOS namespaces do not allow access of the data across object stores.

!!! note
    This sample requires *at least 3 OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

#### Shared Pools

Create the shared pools that will be used by each of the object stores.

!!! note
    If object stores have been previously created, the first pool below (`.rgw.root`)
    does not need to be defined again as it would have already been created
    with an existing object store. There is only one `.rgw.root` pool existing
    to store metadata for all object stores.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-root
  namespace: rook-ceph # namespace:cluster
spec:
  name: .rgw.root
  failureDomain: host
  replicated:
    size: 3
    requireSafeReplicaSize: false
  parameters:
    pg_num: "8"
  application: rgw
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-meta-pool
  namespace: rook-ceph # namespace:cluster
spec:
  failureDomain: host
  replicated:
    size: 3
    requireSafeReplicaSize: false
  parameters:
    pg_num: "8"
  application: rgw
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-data-pool
  namespace: rook-ceph # namespace:cluster
spec:
  failureDomain: osd
  erasureCoded:
    # For production it is recommended to use more chunks, such as 4+2 or 8+4
    dataChunks: 2
    codingChunks: 1
  parameters:
    bulk: "true"
  application: rgw
```

Create the shared pools:

```console
kubectl create -f object-shared-pools.yaml
```

#### Create Each Object Store

After the pools have been created above, create each object store to consume the shared pools.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: store-a
  namespace: rook-ceph # namespace:cluster
spec:
  sharedPools:
    metadataPoolName: rgw-meta-pool
    dataPoolName: rgw-data-pool
    preserveRadosNamespaceDataOnDelete: true
  gateway:
    # sslCertificateRef:
    port: 80
    instances: 1
```

Create the object store:

```console
kubectl create -f object-a.yaml
```

To confirm the object store is configured, wait for the RGW pod(s) to start:

```console
kubectl -n rook-ceph get pod -l rgw=store-a
```

Additional object stores can be created based on the same shared pools by simply changing the
`name` of the CephObjectStore. In the example manifests folder, two object store examples are
provided: `object-a.yaml` and `object-b.yaml`.

To consume the object store, continue below in the section to [Create a bucket](#create-a-bucket).
Modify the default example object store name from `my-store` to the alternate name of the object store
such as `store-a` in this example.

### Create Local Object Store(s) with pool placements

!!! attention
    This feature is experimental.

This section contains a guide on how to configure [RGW's pool placement and storage classes](https://docs.ceph.com/en/reef/radosgw/placement/) with Rook.

Object Storage API allows users to override where bucket data will be stored during bucket creation. With `<LocationConstraint>` parameter in S3 API and `X-Storage-Policy` header in SWIFT. Similarly, users can override where object data will be stored by setting `X-Amz-Storage-Class` and `X-Object-Storage-Class` during object creation.

To enable this feature, configure `poolPlacements` representing a list of possible bucket data locations.
Each `poolPlacement` must have:

* a **unique** `name` to refer to it in `<LocationConstraint>` or `X-Storage-Policy`. Name `default-placement` is reserved and can be used **only** if placement also marked as `default`.
* **optional** `default` flag to use given placement by default, meaning that it will be used if no location constraint is provided. Only one placement in the list can be marked as default.
* `dataPoolName` and `metadataPoolName` representing object data and metadata locations. In Rook, these data locations are backed by `CephBlockPool`. `poolPlacements` and `storageClasses` specs refer pools by name. This means that all pools should be defined in advance. Similarly to [sharedPools](#create-local-object-stores-with-shared-pools), the same pool can be reused across multiple ObjectStores and/or poolPlacements/storageClasses because of RADOS namespaces. Here, each pool will be namespaced with `<object store name>.<placement name>.<pool type>` key.
* **optional** `dataNonECPoolName` - extra pool for data that cannot use erasure coding (ex: multi-part uploads). If not set, `metadataPoolName` will be used.
* **optional** list of placement `storageClasses`. Classes defined per placement, which means that even classes of `default` placement will be available only within this placement and not others. Each placement will automatically have default storage class named `STANDARD`. `STANDARD` class always points to placement `dataPoolName` and cannot be removed or redefined. Each storage class must have:
    * `name` (unique within placement). RGW allows arbitrary name for StorageClasses, however some clients/libs insist on AWS names so it is recommended to use one of the valid `x-amz-storage-class` values for better compatibility: `STANDARD | REDUCED_REDUNDANCY | STANDARD_IA | ONEZONE_IA | INTELLIGENT_TIERING | GLACIER | DEEP_ARCHIVE | OUTPOSTS | GLACIER_IR | SNOW | EXPRESS_ONEZONE`. See [AWS docs](https://aws.amazon.com/s3/storage-classes/).
    * `dataPoolName` - overrides placement data pool when this class is selected by user.

Example: Configure `CephObjectStore` with `default` placement `us` pools and placement `europe` pointing to pools in corresponding geographies. These geographical locations are only an example. Placement name can be arbitrary and could reflect the backing pool's replication factor, device class, or failure domain. This example also  defines storage class `REDUCED_REDUNDANCY` for each placement.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  gateway:
    port: 80
    instances: 1
  sharedPools:
    poolPlacements:
    - name: us
      default: true
      metadataPoolName: "us-data-pool"
      dataPoolName: "us-meta-pool"
      storageClasses:
      - name: REDUCED_REDUNDANCY
        dataPoolName: "us-reduced-pool"
    - name: europe
      metadataPoolName: "eu-meta-pool"
      dataPoolName: "eu-data-pool"
      storageClasses:
      - name: REDUCED_REDUNDANCY
        dataPoolName: "eu-reduced-pool"
```

S3 clients can direct objects into the pools defined in the above. The example below uses the [s5cmd](https://github.com/peak/s5cmd) CLI tool which is pre-installed in the toolbox pod:

```shell
# make bucket without location constraint -> will use "us"
s5cmd mb s3://bucket1

# put object to bucket1 without storage class -> end up in "us-data-pool"
s5cmd put obj s3://bucket1/obj

# put object to bucket1 with  "STANDARD" storage class -> end up in "us-data-pool"
s5cmd put obj s3://bucket1/obj --storage-class=STANDARD

# put object to bucket1 with "REDUCED_REDUNDANCY" storage class -> end up in "us-reduced-pool"
s5cmd put obj s3://bucket1/obj --storage-class=REDUCED_REDUNDANCY


# make bucket with location constraint europe
s5cmd mb s3://bucket2 --region=my-store:europe

# put object to bucket2 without storage class -> end up in "eu-data-pool"
s5cmd put obj s3://bucket2/obj

# put object to bucket2 with  "STANDARD" storage class -> end up in "eu-data-pool"
s5cmd put obj s3://bucket2/obj --storage-class=STANDARD

# put object to bucket2 with "REDUCED_REDUNDANCY" storage class -> end up in "eu-reduced-pool"
s5cmd put obj s3://bucket2/obj --storage-class=REDUCED_REDUNDANCY

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

For an external CephCluster, configure Rook to consume external RGW servers with the following:

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

See `object-external.yaml` for a more detailed example.

Even though multiple `externalRgwEndpoints` can be specified, it is best to use a single endpoint.
Only the first endpoint in the list will be advertised to any consuming resources like
CephObjectStoreUsers, ObjectBucketClaims, or COSI resources. If there are multiple external RGW
endpoints, add load balancer in front of them, then use the single load balancer endpoint in the
`externalRgwEndpoints` list.

## Object store endpoint

The CephObjectStore resource `status.info` contains `endpoint` (and `secureEndpoint`) fields, which
report the endpoint that can be used to access the object store as a client. This endpoint is also
advertised as the default endpoint for CephObjectStoreUsers, ObjectBucketClaims, and
Container Object Store Interface (COSI) resources.

Each object store also creates a Kubernetes service that can be used as a client endpoint from
within the Kubernetes cluster. The DNS name of the service is
`rook-ceph-rgw-<objectStoreName>.<objectStoreNamespace>.svc`. This service DNS name is the default
`endpoint` (and `secureEndpoint`).

For [external clusters](#connect-to-an-external-object-store), the default endpoint is the first
`spec.gateway.externalRgwEndpoint` instead of the service DNS name.

The advertised endpoint can be overridden using `advertiseEndpoint` in the
[`spec.hosting` config](../../CRDs/Object-Storage/ceph-object-store-crd.md#hosting-settings).

Rook always uses the advertised endpoint to perform management operations against the object store.
When [TLS is enabled](#enable-tls), the TLS certificate must always specify the endpoint DNS name to
allow secure management operations.

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

If any `hosting.dnsNames` are set in the `CephObjectStore` CRD, S3 clients can access buckets in [virtual-host-style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html).
Otherwise, S3 clients must be configured to use path-style access.

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

### Managing User S3 Credentials

The default behavior of `CephObjectStoreUser` is to place the S3 credentials generated when an RGW user is created into a Kubernetes Secret.
The s3 credential(s) may also be explicitly set via `.spec.keys`.
This field requires at least one keypair must and there is no maximum limit.
When `.spec.keys` is present, no `Secret` is created for the user.
If an existing `CephObjectStoreUser` is being transited to using explicit keys, the existing `Secret` will be deleted.
Conversely, if a `.spec.keys` is patched out of an existing `CephObjectStoreUser` all except one of keypairs set on the user will be purged and a `Secret` will be created holding the only remaining keypair.

!!! important
    When `.spec.keys` is not set, all but **one** keypair will be removed from the rgw user.
    When `.spec.keys` is set, any keypair not explicitly specified will be removed from the rgw user.

Example of explicitly managing the rgw user's keypairs:

```yaml
---
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: foo
  namespace: rook-ceph
spec:
  store: my-store
  clusterNamespace: rook-ceph
  keys:
    - accessKeyRef:
        name: foo-s3
        key: AWS_ACCESS_KEY_ID
      secretKeyRef:
        name: foo-s3
        key: AWS_SECRET_ACCESS_KEY
    - accessKeyRef:
        name: bar-s3
        key: AWS_ACCESS_KEY_ID
      secretKeyRef:
        name: bar-s3
        key: AWS_SECRET_ACCESS_KEY
```

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: foo-s3
  namespace: rook-ceph
data:
  AWS_ACCESS_KEY_ID: baz
  AWS_SECRET_ACCESS_KEY: baz
```

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: bar-s3
  namespace: rook-ceph
data:
  AWS_ACCESS_KEY_ID: qux
  AWS_SECRET_ACCESS_KEY: quxbar
```

## Enable TLS

TLS is critical for securing object storage data access, and it is assumed as a default by many S3
clients. TLS is enabled for CephObjectStores by configuring
[`gateway` options](../../CRDs/Object-Storage/ceph-object-store-crd.md#gateway-settings).
Set `securePort`, and give Rook access to a TLS certificate using `sslCertificateRef`.
`caBundleRef` may be necessary as well to give the deployed gateway (RGW) access to the TLS
certificate's CA signing bundle.

Ceph RGW only supports a **single** TLS certificate. If the given TLS certificate is a concatenation
of multiple certificates, only the first certificate will be used by the RGW as the server
certificate. Therefore, the TLS certificate given must include all endpoints that clients will use
for access as subject alternate names (SANs).

The [CephObjectStore service endpoint](#object-store-endpoint) must be added as a SAN on the TLS
certificate. If it is not possible to add the service DNS name as a SAN on the TLS certificate,
set `hosting.advertiseEndpoint` to a TLS-approved endpoint to help ensure Rook and clients use
secure data access.

!!! note
    OpenShift users can use add `service.beta.openshift.io/serving-cert-secret-name` as a service
    annotation instead of using `sslCertificateRef`.

## Virtual host-style Bucket Access

The Ceph Object Gateway supports accessing buckets using
[virtual host-style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html)
addressing, which allows addressing buckets using the bucket name as a subdomain in the endpoint.

AWS has deprecated the the alternative path-style addressing mode which is Rook and Ceph's default.
As a result, many end-user applications have begun to remove path-style support entirely. Many
production clusters will have to enable virtual host-style address.

Virtual host-style addressing requires 2 things:

1. An endpoint that supports [wildcard addressing](https://en.wikipedia.org/wiki/Wildcard_DNS_record)
2. CephObjectStore [hosting](../../CRDs/Object-Storage/ceph-object-store-crd.md#hosting-settings) configuration.

Wildcard addressing can be configured in myriad ways. Some options:

- Kubernetes [ingress loadbalancer](https://kubernetes.io/docs/concepts/services-networking/ingress/#hostname-wildcards)
- Openshift [DNS operator](https://docs.openshift.com/container-platform/latest/networking/dns-operator.html)

The minimum recommended `hosting` configuration is exemplified below. It is important to ensure that
Rook advertises the wildcard-addressable endpoint as a priority over the default. TLS is also
recommended for security, and the configured TLS certificate should specify the advertise endpoint.

```yaml
spec:
  ...
  hosting:
    advertiseEndpoint:
      dnsName: my.wildcard.addressable.endpoint.com
      port: 443
      useTls: true
```

A more complex `hosting` configuration is exemplified below. In this example, two
wildcard-addressable endpoints are available. One is a wildcard-addressable ingress service that is
accessible to clients outside of the Kubernetes cluster (`s3.ingress.domain.com`). The other is a
wildcard-addressable Kubernetes cluster service (`s3.rook-ceph.svc`). The cluster service is the
preferred advertise endpoint because the internal service avoids the possibility of the ingress
service's router being a bottleneck for S3 client operations.

```yaml
spec:
  ...
  hosting:
    advertiseEndpoint:
      dnsName: s3.rook-ceph.svc
      port: 443
      useTls: true
  dnsNames:
    - s3.ingress.domain.com
```

## Object Multisite

Multisite is a feature of Ceph that allows object stores to replicate its data over multiple Ceph clusters.

Multisite also allows object stores to be independent and isolated from other object stores in a cluster.

For more information on multisite please read the [ceph multisite overview](ceph-object-multisite.md) for how to run it.

## Object Multi-instance

This section describes how to configure multiple `CephObjectStore` backed by the same storage pools.
The setup allows using different configuration parameters for each `CephObjectStore`, like:

- `hosting` and `gateway` configs to host object store APIs on different ports and domains. For example, having a independently-scalable deployments for internal and external traffic.
- `protocols` to host `S3`, `Swift`, and `Admin-ops` APIs on a separate `CephObjectStores`.
- having different resource limits, affinity or other configurations per `CephObjectStore` instance for other possible use-cases.

Multi-instance setup can be described in two steps. The first step is to create `CephObjectRealm`, `CephObjectZoneGroup`, and `CephObjectZone`, where
`CephObjectZone` contains storage pools configuration. This configuration will be shared across all `CephObjectStore` instances:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectRealm
metadata:
  name: multi-instance-store
  namespace: rook-ceph # namespace:cluster
spec:
  defaultRealm: false
---
apiVersion: ceph.rook.io/v1
kind: CephObjectZoneGroup
metadata:
  name: multi-instance-store
  namespace: rook-ceph # namespace:cluster
spec:
  realm: multi-instance-store
---
apiVersion: ceph.rook.io/v1
kind: CephObjectZone
metadata:
  name: multi-instance-store
  namespace: rook-ceph # namespace:cluster
spec:
  zoneGroup: multi-instance-store
  metadataPool:
    failureDomain: host
    replicated:
      size: 1
      requireSafeReplicaSize: false
  dataPool:
    failureDomain: host
    replicated:
      size: 1
      requireSafeReplicaSize: false
```

The second step defines multiple `CephObjectStore` with different configurations. All of the stores should refer to the same `zone`.

```yaml
# RGW instance to host admin-ops API only
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: store-admin
  namespace: rook-ceph # namespace:cluster
spec:
  gateway:
    port: 80
    instances: 1
  zone:
    name: multi-instance-store
  protocols:
    enableAPIs: ["admin"]
---
# RGW instance to host S3 API only
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: store-s3
  namespace: rook-ceph # namespace:cluster
spec:
  gateway:
    port: 80
    instances: 1
  zone:
    name: multi-instance-store
  protocols:
    enableAPIs:
      - s3
      - s3website
      - sts
      - iam
      - notifications
---
# RGW instance to host SWIFT API only
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: store-swift
  namespace: rook-ceph # namespace:cluster
spec:
  gateway:
    port: 80
    instances: 1
  zone:
    name: multi-instance-store
  protocols:
    enableAPIs:
      - swift
      - swift_auth
    swift:
      # if S3 API is disabled, then SWIFT can be hosted on root path without prefix
      urlPrefix: "/"
```

!!! note
    Child resources should refer to the appropriate RGW instance.
    For example, a `CephObjectStoreUser` requires the Admin Ops API,
    so it should refer to an instance where this API is enabled.
    After the user is created, it can be used for all instances.

See the [example configuration](https://github.com/rook/rook/blob/master/deploy/examples/object-multi-instance-test.yaml) for more details.

## Using Swift and Keystone

It is possible to access an object store using the [Swift API](https://developer.openstack.org/api-ref/object-store/index.html).
Using Swift requires the use of [OpenStack Keystone](https://docs.openstack.org/keystone/latest/) as an authentication provider.

More information on the use of Swift and Keystone can be found in the document on [Object Store with Keystone and Swift](ceph-object-swift.md).
