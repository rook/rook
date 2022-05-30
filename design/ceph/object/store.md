# Rook Object Store

## Overview

An object store is a collection of resources and services that work together to serve HTTP requests to PUT and GET objects. Rook will automate the configuration of the Ceph resources and services that are necessary to start and maintain a highly available, durable, and performant object store.

The Ceph object store supports S3 and Swift APIs and a multitude of features such as replication of object stores between different zones. The Rook object store is designed to support all of these features, though will take some time to implement them. We welcome contributions! In the meantime, features that are not yet implemented can be configured by using the [Rook toolbox](/Documentation/Troubleshooting/ceph-toolbox.md) to run the `radosgw-admin` and other tools for advanced configuration.

### Prerequisites

A Rook storage cluster must be configured and running in Kubernetes. In this example, it is assumed the cluster is in the `rook` namespace.

## Object Store Walkthrough

When the storage admin is ready to create an object storage, the admin will specify his desired configuration settings in a yaml file such as the following `object-store.yaml`. This example is a simple object store with metadata that is replicated across different hosts, and the data is erasure coded across multiple devices in the cluster.
```yaml
apiVersion: ceph.rook.io/v1alpha1
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
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
  gateway:
    port: 80
    securePort: 443
    instances: 3
```

Now create the object store.
```bash
kubectl create -f object-store.yaml
```

At this point the Rook operator recognizes that a new object store resource needs to be configured. The operator will create all of the resources to start the object store.
1. Metadata pools are created (`.rgw.root`, `my-store.rgw.control`, `my-store.rgw.meta`, `my-store.rgw.log`, `my-store.rgw.buckets.index`)
2. The data pool is created (`my-store.rgw.buckets.data`)
3. A Kubernetes service is created to provide load balancing for the RGW pod(s)
4. A Kubernetes deployment is created to start the RGW pod(s) with the settings for the new zone
5. The zone is modified to add the RGW pod endpoint(s) if zone is mentioned in the configuration

When the RGW pods start, the object store is ready to receive the http or https requests as configured.


## Object Store CRD

The object store settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources. The operator stays in a control loop to watch for a new object store, changes to an existing object store, or requests to delete an object store.

### Pools

The pools are the backing data store for the object store and are created with specific names to be private to an object store. Pools can be configured with all of the settings that can be specified in the [Pool CRD](/Documentation/CRDs/Block-Storage/ceph-block-pool-crd.md). The underlying schema for pools defined by a pool CRD is the same as the schema under the `metadataPool` and `dataPool` elements of the object store CRD. All metadata pools are created with the same settings, while the data pool can be created with independent settings. The metadata pools must use replication, while the data pool can use replication or erasure coding.

If `preservePoolsOnDelete` is set to 'true' the pools used to support the object store will remain when the object store will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified is also deemed as 'false'.

```yaml
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
  preservePoolsOnDelete: true
```

If there is a `zone` section in object-store configuration, then the pool section in the ceph-object-zone resource will be used to define the pools.

### Gateway

The gateway settings correspond to the RGW service.
- `type`: Can be `s3`. In the future support for `swift` can be added.
- `sslCertificateRef`: If specified, this is the name of the Kubernetes secret that contains the SSL
  certificate to be used for secure connections to the object store. The secret must be in the same
  namespace as the Rook cluster. If it is an opaque Kubernetes Secret, Rook will look in the secret
  provided at the `cert` key name. The value of the `cert` key must be in the format expected by the
  [RGW
  service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb):
  "The server key, server certificate, and any other CA or intermediate certificates be supplied in
  one file. Each of these items must be in pem form." If the certificate is not specified, SSL will
  not be configured.
- `caBundleRef`: If specified, this is the name of the Kubernetes secret (type `opaque`) that contains ca-bundle to use. The secret must be in the same namespace as the Rook cluster. Rook will look in the secret provided at the `cabundle` key name.
- `port`: The service port where the RGW service will be listening (http)
- `securePort`: The service port where the RGW service will be listening (https)
- `instances`: The number of RGW pods that will be started for this object store
- `placement`: The rgw pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, `podAntiAffinity`, and `topologySpreadConstraints` similar to placement defined for daemons configured by the [cluster CRD](/deploy/examples/cluster.yaml).

The RGW service can be configured to listen on both http and https by specifying both `port` and `securePort`.

```yaml
 gateway:
    sslCertificateRef: my-ssl-cert-secret
    securePort: 443
    instances: 1
```

### Multisite

By default, the object store will be created independently from any other object stores and replication to another object store will not be configured. This done by creating a new Ceph realm, zone group, and zone all with the name of the new object store.

If desired to configure the object store to replicate and sync data amongst object-store or Ceph clusters, the `zone` section would be required.

This section enables the the object store to be part of a specified ceph-object-zone.

Specifying this section also ensures that the pool section in the ceph-object-zone is used for the object-store. If pools are specified for the object-store they are neither created nor deleted.

- `name`: name of the [ceph-object-zone](/design/ceph/object/ceph-object-zone.md) the object store is in. This name must be of a ceph-object-zone resource not just of a zone that has been already created.

```yaml
  zone:
    name: "name"
```
