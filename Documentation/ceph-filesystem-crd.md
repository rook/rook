---
title: Shared Filesystem CRD
weight: 3000
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Ceph Shared Filesystem CRD

Rook allows creation and customization of shared filesystems through the custom resource definitions (CRDs). The following settings are available for Ceph filesystems.

## Samples

### Replicated

> **NOTE**: This sample requires *at least 1 OSD per node*, with each OSD located on *3 different nodes*.

Each OSD must be located on a different node, because both of the defined pools set the [`failureDomain`](ceph-pool-crd.md#spec) to `host` and the `replicated.size` to `3`.

The `failureDomain` can also be set to another location type (e.g. `rack`), if it has been added as a `location` in the [Storage Selection Settings](ceph-cluster-crd.md#storage-selection-settings).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPools:
    - failureDomain: host
      replicated:
        size: 3
  preservePoolsOnDelete: true
  metadataServer:
    activeCount: 1
    activeStandby: true
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
    #          - mds-node
    #  tolerations:
    #  - key: mds-node
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

(These definitions can also be found in the [`filesystem.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/filesystem.yaml) file)

### Erasure Coded

Erasure coded pools require the OSDs to use `bluestore` for the configured [`storeType`](ceph-cluster-crd.md#osd-configuration-settings). Additionally, erasure coded pools can only be used with `dataPools`. The `metadataPool` must use a replicated pool.

> **NOTE**: This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](ceph-pool-crd.md#spec) will be set to `host` by default, and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: myfs-ec
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

(These definitions can also be found in the [`filesystem-ec.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/filesystem-ec.yaml) file)

## Filesystem Settings

### Metadata

* `name`: The name of the filesystem to create, which will be reflected in the pool and other resource names.
* `namespace`: The namespace of the Rook cluster where the filesystem is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least eight devices (6 data + 2 coding chunks) in the cluster.

* `metadataPool`: The settings used to create the filesystem metadata pool. Must use replication.
* `dataPools`: The settings to create the filesystem data pools. If multiple pools are specified, Rook will add the pools to the filesystem. Assigning users or files to a pool is left as an exercise for the reader with the [CephFS documentation](http://docs.ceph.com/docs/master/cephfs/file-layouts/). The data pools can use replication or erasure coding. If erasure coding pools are specified, the cluster must be running with bluestore enabled on the OSDs.
* `preservePoolsOnDelete`: If it is set to 'true' the pools used to support the filesystem will remain when the filesystem will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified is also deemed as 'false'.

## Metadata Server Settings

The metadata server settings correspond to the MDS daemon settings.

* `activeCount`: The number of active MDS instances. As load increases, CephFS will automatically partition the filesystem across the MDS instances. Rook will create double the number of MDS instances as requested by the active count. The extra instances will be in standby mode for failover.
* `activeStandby`: If true, the extra MDS instances will be in active standby mode and will keep a warm cache of the filesystem metadata for faster failover. The instances will be assigned by CephFS in failover pairs. If false, the extra MDS instances will all be on passive standby mode and will not maintain a warm cache of the metadata.
* `annotations`: Key value pair list of annotations to add.
* `placement`: The mds pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml).
* `resources`: Set resource requests/limits for the Filesystem MDS Pod(s), see [Resource Requirements/Limits](ceph-cluster-crd.md#resource-requirementslimits).
* `priorityClassName`: Set priority class name for the Filesystem MDS Pod(s)
