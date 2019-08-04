---
title: Object Store CRD
weight: 2800
indent: true
---

# Ceph Object Store CRD

Rook allows creation and customization of object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object stores.

## Sample

### Erasure Coded

Erasure coded pools require the OSDs to use `bluestore` for the configured [`storeType`](ceph-cluster-crd.md#osd-configuration-settings). Additionally, erasure coded pools can only be used with `dataPools`. The `metadataPool` must use a replicated pool.

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
    securePort:
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
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
```

## Object Store Settings

### Metadata

* `name`: The name of the object store to create, which will be reflected in the pool and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

* `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
* `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.
* `preservePoolsOnDelete`: If it is set to 'true' the pools used to support the object store will remain when the object store will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified is also deemed as 'false'.

## Gateway Settings

The gateway settings correspond to the RGW daemon settings.

* `type`: `S3` is supported
* `sslCertificateRef`: If the certificate is not specified, SSL will not be configured. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections to the object store. Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be in the format expected by the [RGW service](http://docs.ceph.com/docs/master/install/install-ceph-gateway/#using-ssl-with-civetweb): "The server key, server certificate, and any other CA or intermediate certificates be supplied in one file. Each of these items must be in pem form."
* `port`: The port on which the RGW pods and the RGW service will be listening (not encrypted).
* `securePort`: The secure port on which RGW pods will be listening. An SSL certificate must be specified.
* `instances`: The number of pods that will be started to load balance this object store.
* `annotations`: Key value pair list of annotations to add.
* `placement`: The Kubernetes placement settings to determine where the RGW pods should be started in the cluster.
* `resources`: Set resource requests/limits for the Gateway Pod(s), see [Resource Requirements/Limits](ceph-cluster-crd.md#resource-requirementslimits).
* `priorityClassName`: Set priority class name for the Gateway Pod(s)

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
