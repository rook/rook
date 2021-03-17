---
title: Object Store CRD
weight: 2800
indent: true
---

# Ceph Object Store CRD

Rook allows creation and customization of object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object stores.

## Sample

### Erasure Coded

Erasure coded pools can only be used with `dataPools`. The `metadataPool` must use a replicated pool.

> **NOTE**: This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](ceph-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

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
    type: s3
    sslCertificateRef:
    port: 80
    # securePort: 443
    instances: 1
    # A key/value list of annotations
    annotations:
    #  key: value
    placement:
    #  nodeAffinity:
    #    requiredDuringSchedulingIgnoredDuringExecution:
    #      nodeSelectorTerms:
    #      - matchExpressions:
    #        - key: role
    #          operator: In
    #          values:
    #          - rgw-node
    #  tolerations:
    #  - key: rgw-node
    #    operator: Exists
    #  podAffinity:
    #  podAntiAffinity:
    #  topologySpreadConstraints:
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
  #zone:
    #name: zone-a
```

## Object Store Settings

### Metadata

* `name`: The name of the object store to create, which will be reflected in the pool and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

When the `zone` section is set pools with the object stores name will not be created since the object-store will the using the pools created by the ceph-object-zone.

* `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
* `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.
* `preservePoolsOnDelete`: If it is set to 'true' the pools used to support the object store will remain when the object store will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified is also deemed as 'false'.

## Gateway Settings

The gateway settings correspond to the RGW daemon settings.

* `type`: `S3` is supported
* `sslCertificateRef`: If the certificate is not specified, SSL will not be configured. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections to the object store. Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be in the format expected by the [RGW service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb): "The server key, server certificate, and any other CA or intermediate certificates be supplied in one file. Each of these items must be in pem form."
* `port`: The port on which the Object service will be reachable. If host networking is enabled, the RGW daemons will also listen on that port. If running on SDN, the RGW daemon listening port will be 8080 internally.
* `securePort`: The secure port on which RGW pods will be listening. An SSL certificate must be specified.
* `instances`: The number of pods that will be started to load balance this object store.
* `externalRgwEndpoints`: A list of IP addresses to connect to external existing Rados Gateways (works with external mode). This setting will be ignored if the `CephCluster` does not have `external` spec enabled. Refer to the [external cluster section](ceph-cluster-crd.md#external-cluster) for more details.
* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `placement`: The Kubernetes placement settings to determine where the RGW pods should be started in the cluster.
* `resources`: Set resource requests/limits for the Gateway Pod(s), see [Resource Requirements/Limits](ceph-cluster-crd.md#resource-requirementslimits).
* `priorityClassName`: Set priority class name for the Gateway Pod(s)

Example of external rgw endpoints to connect to:

```yaml
gateway:
  port: 80
  externalRgwEndpoints:
    - ip: 192.168.39.182
```

This will create a service with the endpoint `192.168.39.182` on port `80`, pointing to the Ceph object external gateway.
All the other settings from the gateway section will be ignored, except for `securePort`.

## Zone Settings

The [zone](ceph-object-multisite.md) settings allow the object store to join custom created [ceph-object-zone](ceph-object-multisite-crd.md).

* `name`: the name of the ceph-object-zone the object store will be in.

## Runtime settings

### MIME types

Rook provides a default `mime.types` file for each Ceph object store. This file is stored in a
Kubernetes ConfigMap with the name `rook-ceph-rgw-<STORE-NAME>-mime-types`. For most users, the
default file should suffice, however, the option is available to users to edit the `mime.types`
file in the ConfigMap as they desire. Users may have their own special file types, and particularly
security conscious users may wish to pare down the file to reduce the possibility of a file type
execution attack.

Rook will not overwrite an existing `mime.types` ConfigMap so that user modifications will not be
destroyed. If the object store is destroyed and recreated, the ConfigMap will also be destroyed and
created anew.

## Health settings

Rook-Ceph will be default monitor the state of the object store endpoints.
The following CRD settings are available:

* `healthCheck`: main object store health monitoring section

Here is a complete example:

```yaml
healthCheck:
  bucket:
    disabled: false
    interval: 60s
```

The endpoint health check procedure is the following:

1. Create an S3 user
2. Create a bucket with that user
3. PUT the file in the object store
4. GET the file from the object store
5. Verify object consistency
6. Update CR health status check

Rook-Ceph always keeps the bucket and the user for the health check, it just does a PUT and GET of an s3 object since creating a bucket is an expensive operation.
