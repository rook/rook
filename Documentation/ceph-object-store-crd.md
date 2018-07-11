---
title: Ceph Object Store
weight: 34
indent: true
---

# Ceph Object Store CRD

Rook allows creation and customization of object stores through the custom resource definitions (CRDs). The following settings are available
for Ceph object stores.

## Sample

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

- `name`: The name of the object store to create, which will be reflected in the pool and other resource names.
- `namespace`: The namespace of the Rook cluster where the object store is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

- `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
- `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.

## Gateway Settings

The gateway settings correspond to the RGW daemon settings.

- `type`: `S3` is supported
- `sslCertificateRef`: If the certificate is not specified, SSL will not be configured. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections to the object store. Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be in the format expected by the [RGW service](http://docs.ceph.com/docs/master/install/install-ceph-gateway/#using-ssl-with-civetweb): "The server key, server certificate, and any other CA or intermediate certificates be supplied in one file. Each of these items must be in pem form."
- `port`: The port on which the RGW pods and the RGW service will be listening (not encrypted).
- `securePort`: The secure port on which RGW pods will be listening. An SSL certificate must be specified.
- `instances`: The number of pods that will be started to load balance this object store. Ignored if `allNodes` is true.
- `allNodes`: Whether RGW pods should be started on all nodes. If true, a daemonset is created. If false, `instances` must be set.
- `placement`: The Kubernetes placement settings to determine where the RGW pods should be started in the cluster.
- `resources`: Set resource requests/limits for the Gateway Pod(s), see [Resource Requirements/Limits](ceph-cluster-crd.md#resource-requirementslimits).
