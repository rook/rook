# Rook Shared File System

## Overview

A shared file system is a collection of resources and services that work together to serve a files for multiple users across multiple clients. Rook will automate the configuration of the Ceph resources and services that are necessary to start and maintain a highly available, durable, and performant shared file system.

### Prerequisites

A Rook storage cluster must be configured and running in Kubernetes. In this example, it is assumed the cluster is in the `rook` namespace.

## File System Walkthrough

When the storage admin is ready to create a shared file system, he will specify his desired configuration settings in a yaml file such as the following `filesystem.yaml`. This example is a simple configuration with metadata that is replicated across different hosts, and the data is erasure coded across multiple devices in the cluster. One active MDS instance is started, with one more MDS instance started in standby mode.
```yaml
apiVersion: ceph.rook.io/v1beta1
kind: Filesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataPool:
    replicated:
      size: 3
  dataPools:
    - erasureCoded:
       dataChunks: 2
       codingChunks: 1
  metadataServer:
    activeCount: 1
    activeStandby: true
```

Now create the file system.
```bash
kubectl create -f filesystem.yaml
```

At this point the Rook operator recognizes that a new file system needs to be configured. The operator will create all of the necessary resources.
1. The metadata pool is created (`myfs-meta`)
1. The data pools are created (only one data pool for the example above: `myfs-data0`)
1. The Ceph file system is created with the name `myfs`
1. If multiple data pools were created, they would be added to the file system
1. The file system is configured for the desired active count of MDS (`max_mds`=3)
1. A Kubernetes deployment is created to start the MDS pods with the settings for the file system. Twice the number of instances are started as requested for the active count, with half of them in standby.

After the MDS pods start, the file system is ready to be mounted.


## File System CRD

The file system settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources. The operator stays in a control loop to watch for a new file system, changes to an existing file system, or requests to delete a file system.

### Pools

The pools are the backing data store for the file system and are created with specific names to be private to a file system. Pools can be configured with all of the settings that can be specified in the [Pool CRD](/Documentation/ceph-pool-crd.md). The underlying schema for pools defined by a pool CRD is the same as the schema under the `metadataPool` element and the `dataPools` elements of the file system CRD.

```yaml
  metadataPool:
    replicated:
      size: 3
  dataPools:
    - replicated:
       size: 3
    - erasureCoded:
       dataChunks: 2
       codingChunks: 1
```

Multiple data pools can be configured for the file system. Assigning users or files to a pool is left as an exercise for the reader with the [CephFS documentation](http://docs.ceph.com/docs/master/cephfs/file-layouts/).

### Metadata Server

The metadata server settings correspond to the MDS service.
- `activeCount`: The number of active MDS instances. As load increases, CephFS will automatically partition the file system across the MDS instances. Rook will create double the number of MDS instances as requested by the active count. The extra instances will be in standby mode for failover.
- `activeStandby`: If true, the extra MDS instances will be in active standby mode and will keep a warm cache of the file system metadata for faster failover. The instances will be assigned by CephFS in failover pairs. If false, the extra MDS instances will all be on passive standby mode and will not maintain a warm cache of the metadata.
- `placement`: The mds pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/ceph/cluster.yaml).

```yaml
  metadataServer:
    activeCount: 1
    activeStandby: true
    placement:
```

### Multiple File Systems
In Ceph Luminous, multiple file systems is still considered an experimental feature. While Rook seamlessly enables this scenario, be aware of the issues in the [CephFS docs](http://docs.ceph.com/docs/master/cephfs/experimental-features/#multiple-filesystems-within-a-ceph-cluster) with snapshots and security implications.


### CephFS data model

For a description of the underlying Ceph data model, see the [CephFS Terminology](http://docs.ceph.com/docs/master/cephfs/standby/#terminology).
