---
title: Shared File System CRD
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
# Ceph Shared File System CRD

Rook allows creation and customization of shared file systems through the custom resource definitions (CRDs). The following settings are available
for Ceph file systems.

## Samples

### Replicated

**NOTE** This example requires you to have **at least 3 OSDs each on a different node**.
This is because the `replicated.size: 3` (in both defined Pools) will require at least 3 OSDs and as [`failureDomain` setting](ceph-pool-crd.md#spec) to `host` (default), each OSD needs to be on a different nodes.
In case you added another location type to your nodes in the [Storage Selection Settings](ceph-cluster-crd.md#storage-selection-settings) (e.g. `rack`), you can also specify this type as your failure domain.

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

If you want to use erasure coded pool with filesystem, your OSDs must use `bluestore` as their `storeType`.
Additionally erasure coded can only be used as a data pool and not as a metadata pool. The metadata pool must still be a replicated pool.

The sample below requires that you have at least 3 `bluestore` OSDs on different nodes.
For erasure coded to make sense, you need **at least three OSDs for the below `dataPools` config** to work.

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


## File System Settings

### Metadata

- `name`: The name of the file system to create, which will be reflected in the pool and other resource names.
- `namespace`: The namespace of the Rook cluster where the file system is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least eight devices (6 data + 2 coding chunks) in the cluster.

- `metadataPool`: The settings used to create the file system metadata pool. Must use replication.
- `dataPools`: The settings to create the file system data pools. If multiple pools are specified, Rook will add the pools to the file system. Assigning users or files to a pool is left as an exercise for the reader with the [CephFS documentation](http://docs.ceph.com/docs/master/cephfs/file-layouts/). The data pools can use replication or erasure coding. If erasure coding pools are specified, the cluster must be running with bluestore enabled on the OSDs.

## Metadata Server Settings

The metadata server settings correspond to the MDS daemon settings.

- `activeCount`: The number of active MDS instances. As load increases, CephFS will automatically partition the file system across the MDS instances. Rook will create double the number of MDS instances as requested by the active count. The extra instances will be in standby mode for failover.
- `activeStandby`: If true, the extra MDS instances will be in active standby mode and will keep a warm cache of the file system metadata for faster failover. The instances will be assigned by CephFS in failover pairs. If false, the extra MDS instances will all be on passive standby mode and will not maintain a warm cache of the metadata.
- `annotations`: Key value pair list of annotations to add.
- `placement`: The mds pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml).
- `resources`: Set resource requests/limits for the Filesystem MDS Pod(s), see [Resource Requirements/Limits](ceph-cluster-crd.md#resource-requirementslimits).
