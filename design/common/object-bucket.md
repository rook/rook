# Rook Object Store Bucket

## Overview

An object store bucket is a container holding immutable objects. The Rook-Ceph [operator](https://github.com/yard-turkey/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml) creates a controller which automates the provisioning of new and existing buckets.

A user requests bucket storage by creating an _ObjectBucketClaim_ (OBC). Upon detecting a new OBC, the Rook-Ceph bucket provisioner does the following:
- creates a new bucket and grants user-level access (greenfield), or
- grants user-level access to an existing bucket (brownfield), and
- creates a Kubernetes Secret in the same namespace as the OBC
- creates a Kubernetes ConfigMap in the same namespace as the OBC.

The secret contains bucket access keys. The configmap contains bucket endpoint information. Both are consumed by an application pod in order to access the provisioned bucket.

When the _ObjectBucketClaim_ is deleted all of the Kubernetes resources created by the Rook-Ceph provisioner are deleted, and provisioner specific artifacts, such as dynamic users and access policies are removed. And, depending on the _reclaimPolicy_ in the storage class referenced in the OBC, the bucket will be retained or deleted.

We welcome contributions! In the meantime, features that are not yet implemented may be configured by using the [Rook toolbox](/Documentation/ceph-toolbox.md) to run the `radosgw-admin` and other tools for advanced bucket policies.

### Prerequisites

- A Rook storage cluster must be configured and running in Kubernetes. In this example, it is assumed the cluster is in the `rook` namespace.
- The following resources, or equivalent, need to be created:
  - [crd](/cluster/examples/kubernetes/ceph/crds.yaml)
  - [common](/cluster/examples/kubernetes/ceph/common.yaml)
  - [operator](/cluster/examples/kubernetes/ceph/operator.yaml)
  - [cluster](/cluster/examples/kubernetes/ceph/cluster-test.yaml)
  - [object](/cluster/examples/kubernetes/ceph/object-test.yaml)
  - [user](/cluster/examples/kubernetes/ceph/object-user.yaml)
  - [storageclass](/cluster/examples/kubernetes/ceph/storageclass-bucket-retain.yaml)
  - [claim](/cluster/examples/kubernetes/ceph/object-bucket-claim-retain.yaml)


## Object Store Bucket Walkthrough

When the storage admin is ready to create an object storage, he will specify his desired configuration settings in a yaml file such as the following `object-store.yaml`. This example is a simple object store with metadata that is replicated across different hosts, and the data is erasure coded across multiple devices in the cluster.
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: ObjectStore
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
1. Metadata pools are created (`.rgw.root`, `my-store.rgw.control`, `my-store.rgw.meta`, `my-store.rgw.log`, `my-store.rgw.buckets.index`, `my-store.rgw.buckets.non-ec`)
1. The data pool is created (`my-store.rgw.buckets.data`)
1. A Ceph realm is created
1. A Ceph zone group is created in the new realm
1. A Ceph zone is created in the new zone group
1. A cephx key is created for the rgw daemon
1. A Kubernetes service is created to provide load balancing for the RGW pod(s)
1. A Kubernetes deployment is created to start the RGW pod(s) with the settings for the new zone

When the RGW pods start, the object store is ready to receive the http or https requests as configured.


## Object Store CRD

The object store settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources. The operator stays in a control loop to watch for a new object store, changes to an existing object store, or requests to delete an object store.

### Pools

The pools are the backing data store for the object store and are created with specific names to be private to an object store. Pools can be configured with all of the settings that can be specified in the [Pool CRD](/Documentation/ceph-pool-crd.md). The underlying schema for pools defined by a pool CRD is the same as the schema under the `metadataPool` and `dataPool` elements of the object store CRD. All metadata pools are created with the same settings, while the data pool can be created with independent settings. The metadata pools must use replication, while the data pool can use replication or erasure coding.

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
```

### Gateway

The gateway settings correspond to the RGW service.
- `type`: Can be `s3`. In the future support for `swift` can be added.
- `sslCertificateRef`: If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections to the object store. The secret must be in the same namespace as the Rook cluster. Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be in the format expected by the [RGW service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb): "The server key, server certificate, and any other CA or intermediate certificates be supplied in one file. Each of these items must be in pem form." If the certificate is not specified, SSL will not be configured.
- `port`: The service port where the RGW service will be listening (http)
- `securePort`: The service port where the RGW service will be listening (https)
- `instances`: The number of RGW pods that will be started for this object store (ignored if allNodes=true)
- `allNodes`: Whether all nodes in the cluster should run RGW as a daemonset
- `placement`: The rgw pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/ceph/cluster.yaml).

The RGW service can be configured to listen on both http and https by specifying both `port` and `securePort`.

```yaml
 gateway:
    sslCertificateRef: my-ssl-cert-secret
    port: 80
    securePort: 443
    instances: 1
    allNodes: false
```

### Realms, Zone Groups, and Zones

By default, the object store will be created independently from any other object stores and replication to another object store will not be configured. This done by creating a new realm, zone group, and zone all with the name of the new object store. The zone group and zone are tagged as the `master`. If this is the first object store in the cluster, the realm, zone group, and zone will also be marked as the default.

By implementing on the independent realms, zone groups, and zones, Rook supports multiple objects stores in a cluster. The set of users with access to the object store, the metadata, and the data are isolated from other object stores.

If desired to configure the object store to replicate from another cluster or zone, the following settings would be specified on a new object store that is *not* the master. (This feature is not yet implemented.)
- `realm`: If specified, the new zone will be created in the existing realm with that name
- `group`: If specified, the new zone will be created in the existing zone group with that name
- `master`: If specified, settings indicate the RGW endpoint where this object store will need to connect to the master zone in order to initialize the replication. The Rook operator will execute `pull` commands for the realm and zone group as necessary.
```yaml
  zone:
    realm: myrealm
    group: mygroup
    master:
      url: https://my-master-zone-gateway:443/
      accessKey: my-master-zone-access-key
      secret: my-master-zone-secret
```

Failing over the master could be handled by updating the affected object store CRDs, although more design is needed here.

See the ceph docs [here](http://docs.ceph.com/docs/master/radosgw/multisite/) on the concepts around zones and replicating between zones. For reference, a diagram of two zones working across different cluster can be found on page 5 of [this doc](http://ceph.com/wp-content/uploads/2017/01/Understanding-a-Multi-Site-Ceph-Gateway-Installation-170119.pdf).


### Ceph multi-site object store data model

For reference, here is a description of the underlying Ceph data model.

```
A cluster has one or more realms

A realm spans one or more clusters
A realm has one or more zone groups
A realm has one master zone group
A realm defined in another cluster is replicated with the pull command

A zone group has one or more zones
A zone group has one master zone
A zone group spans one or more clusters
A zone group defined in another cluster is replicated with the pull command
A zone group defines a namespace for object IDs unique across its zones
Zone group metadata is replicated to other zone groups in the realm

A zone belongs to one cluster
A zone has a set of pools that store the user and object metadata and object data
Zone data and metadata is replicated to other zones in the zone group
```
