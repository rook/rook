---
title: NFS CRD
weight: 3100
indent: true
---

# Ceph NFS Gateway CRD

## Overview

Rook allows exporting NFS shares of the filesystem or object store through the CephNFS custom resource definition. This will spin up a cluster of [NFS Ganesha](https://github.com/nfs-ganesha/nfs-ganesha) servers that coordinate with one another via shared RADOS objects. The servers will be configured for NFSv4.1+ access, as serving earlier protocols can inhibit responsiveness after a server restart.

## Samples

The following sample will create a two-node active-active cluster of NFS Ganesha gateways. The recovery objects are stored in a RADOS pool named `myfs-data0` with a RADOS namespace of `nfs-ns`.

This example requires the filesystem to first be configured by the [Filesystem](ceph-filesystem-crd.md) because here recovery objects are stored in filesystem data pool.

> **NOTE**: For an RGW object store, a data pool of `my-store.rgw.buckets.data` can be used after configuring the [Object Store](ceph-object-store-crd.md).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: my-nfs
  namespace: rook-ceph
spec:
  rados:
    # RADOS pool where NFS client recovery data and per-daemon configs are
    # stored. In this example the data pool for the "myfs" filesystem is used.
    # If using the object store example, the data pool would be
    # "my-store.rgw.buckets.data". Note that this has nothing to do with where
    # exported CephFS' or objectstores live.
    pool: myfs-data0
    # RADOS namespace where NFS client recovery data is stored in the pool.
    namespace: nfs-ns
  # Settings for the NFS server
  server:
    # the number of active NFS servers
    active: 2
    # A key/value list of annotations
    annotations:
    #  key: value
    # where to run the NFS server
    placement:
    #  nodeAffinity:
    #    requiredDuringSchedulingIgnoredDuringExecution:
    #      nodeSelectorTerms:
    #      - matchExpressions:
    #        - key: role
    #          operator: In
    #          values:
    #          - mds-node
    #  tolerations:
    #  - key: mds-node
    #    operator: Exists
    #  podAffinity:
    #  podAntiAffinity:
    #  topologySpreadConstraints:

    # The requests and limits set here allow the ganesha pod(s) to use half of one CPU core and 1 gigabyte of memory
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
    # the priority class to set to influence the scheduler's pod preemption
    priorityClassName:
```

 Enable the creation of NFS exports in the dashboard for a given cephfs or object gateway pool by running the following command in the toolbox container:

[For single NFS-GANESHA cluster](https://docs.ceph.com/en/latest/mgr/dashboard/#configuring-nfs-ganesha-in-the-dashboard)  

```console 
ceph dashboard set-ganesha-clusters-rados-pool-namespace <ganesha_pool_name>[/<ganesha_namespace>]
```

[For multiple NFS-GANESHA cluster](https://docs.ceph.com/en/latest/mgr/dashboard/#support-for-multiple-nfs-ganesha-clusters)

```console
ceph dashboard set-ganesha-clusters-rados-pool-namespace <cluster_id>:<pool_name>[/<namespace>](,<cluster_id>:<pool_name>[/<namespace>])*
```

## NFS Settings

### RADOS Settings

* `pool`: The pool where ganesha recovery backend and supplemental configuration objects will be stored
* `namespace`: The namespace in `pool` where ganesha recovery backend and supplemental configuration objects will be stored

> **NOTE**: Don't use EC pools for NFS because ganesha uses omap in the recovery objects and grace db. EC pools do not support omap.

## EXPORT Block Configuration

All daemons within a cluster will share configuration with no exports defined, and that includes a RADOS object via:

```ini
%url  rados://<pool>/<namespace>/conf-nfs.<clustername>
```

> **NOTE**: This format of nfs-ganesha config object name was introduced in Ceph Octopus Version. In older versions, each daemon has it's own config object and with the name as *conf-<clustername>.<nodeid>*. The nodeid is a value automatically assigned internally by rook. Nodeids start with "a" and go through "z", at which point they become two letters ("aa" to "az").

The pool and namespace are configured via the spec's RADOS block.

When a server is started, it will create the included object if it does not already exist. It is possible to prepopulate the included objects prior to starting the server. The format for these objects is documented in the [NFS Ganesha](https://github.com/nfs-ganesha/nfs-ganesha/wiki) project.

## Scaling the active server count

It is possible to scale the size of the cluster up or down by modifying
the `spec.server.active` field. Scaling the cluster size up can be done at
will. Once the new server comes up, clients can be assigned to it
immediately.

The CRD always eliminates the highest index servers first, in reverse
order from how they were started. Scaling down the cluster requires that
clients be migrated from servers that will be eliminated to others. That
process is currently a manual one and should be performed before
reducing the size of the cluster.
