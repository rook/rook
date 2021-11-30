---
title: Shared Filesystem CRD
weight: 3000
indent: true
---
{% include_relative branch.liquid %}

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
  preserveFilesystemOnDelete: true
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
    #  topologySpreadConstraints:
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
```

(These definitions can also be found in the [`filesystem.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/filesystem.yaml) file)

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

(These definitions can also be found in the [`filesystem-ec.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/filesystem-ec.yaml) file.
Also see an example in the [`storageclass-ec.yaml`](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/csi/cephfs/storageclass-ec.yaml) for how to configure the volume.)

### Mirroring

Ceph filesystem mirroring is a process of asynchronous replication of snapshots to a remote CephFS file system.
Snapshots are synchronized by mirroring snapshot data followed by creating a snapshot with the same name (for a given directory on the remote file system) as the snapshot being synchronized.
It is generally useful when planning for Disaster Recovery.
Mirroring is for clusters that are geographically distributed and stretching a single cluster is not possible due to high latencies.

The following will enable mirroring of the filesystem:

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
  preserveFilesystemOnDelete: true
  metadataServer:
    activeCount: 1
    activeStandby: true
  mirroring:
    enabled: true
    # list of Kubernetes Secrets containing the peer token
    # for more details see: https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#bootstrap-peers
    peers:
      secretNames:
        - secondary-cluster-peer
    # specify the schedule(s) on which snapshots should be taken
    # see the official syntax here https://docs.ceph.com/en/latest/cephfs/snap-schedule/#add-and-remove-schedules
    snapshotSchedules:
      - path: /
        interval: 24h # daily snapshots
        startTime: 11:55
    # manage retention policies
    # see syntax duration here https://docs.ceph.com/en/latest/cephfs/snap-schedule/#add-and-remove-retention-policies
    snapshotRetention:
      - path: /
        duration: "h 24"
```

Once mirroring is enabled, Rook will by default create its own [bootstrap peer token](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/?#bootstrap-peers) so that it can be used by another cluster.
The bootstrap peer token can be found in a Kubernetes Secret. The name of the Secret is present in the Status field of the CephFilesystem CR:

```yaml
status:
  info:
    fsMirrorBootstrapPeerSecretName: fs-peer-token-myfs
```

This secret can then be fetched like so:

```console
kubectl get secret -n rook-ceph fs-peer-token-myfs -o jsonpath='{.data.token}'|base64 -d
```
>```
>eyJmc2lkIjoiOTFlYWUwZGQtMDZiMS00ZDJjLTkxZjMtMTMxMWM5ZGYzODJiIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFEN1psOWZ3V1VGRHhBQWdmY0gyZi8xeUhYeGZDUTU5L1N0NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjEwLjEwMS4xOC4yMjM6MzMwMCx2MToxMC4xMDEuMTguMjIzOjY3ODldIn0=
>```

The secret must be decoded. The result will be another base64 encoded blob that you will import in the destination cluster:

```console
external-cluster-console # ceph fs snapshot mirror peer_bootstrap import <fs_name> <token file path>
```

See the official cephfs mirror documentation on [how to add a bootstrap peer](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/).

## Filesystem Settings

### Metadata

* `name`: The name of the filesystem to create, which will be reflected in the pool and other resource names.
* `namespace`: The namespace of the Rook cluster where the filesystem is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](ceph-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least eight devices (6 data + 2 coding chunks) in the cluster.

* `metadataPool`: The settings used to create the filesystem metadata pool. Must use replication.
* `dataPools`: The settings to create the filesystem data pools. If multiple pools are specified, Rook will add the pools to the filesystem. Assigning users or files to a pool is left as an exercise for the reader with the [CephFS documentation](http://docs.ceph.com/docs/master/cephfs/file-layouts/). The data pools can use replication or erasure coding. If erasure coding pools are specified, the cluster must be running with bluestore enabled on the OSDs.
* `preserveFilesystemOnDelete`: If it is set to 'true' the filesystem will remain when the
  CephFilesystem resource is deleted. This is a security measure to avoid loss of data if the
  CephFilesystem resource is deleted accidentally. The default value is 'false'. This option
  replaces `preservePoolsOnDelete` which should no longer be set.
* (deprecated) `preservePoolsOnDelete`: This option is replaced by the above
  `preserveFilesystemOnDelete`. For backwards compatibility and upgradeability, if this is set to
  'true', Rook will treat `preserveFilesystemOnDelete` as being set to 'true'.

## Metadata Server Settings

The metadata server settings correspond to the MDS daemon settings.

* `activeCount`: The number of active MDS instances. As load increases, CephFS will automatically partition the filesystem across the MDS instances. Rook will create double the number of MDS instances as requested by the active count. The extra instances will be in standby mode for failover.
* `activeStandby`: If true, the extra MDS instances will be in active standby mode and will keep a warm cache of the filesystem metadata for faster failover. The instances will be assigned by CephFS in failover pairs. If false, the extra MDS instances will all be on passive standby mode and will not maintain a warm cache of the metadata.
* `mirroring`: Sets up mirroring of the filesystem
  * `enabled`: whether mirroring is enabled on that filesystem (default: false)
  * `peers`: to configure mirroring peers
    * `secretNames`:  a list of peers to connect to. Currently (Ceph Pacific release) **only a single** peer is supported where a peer represents a Ceph cluster.
  * `snapshotSchedules`: schedule(s) snapshot.One or more schedules are supported.
    * `path`: filesystem source path to take the snapshot on
    * `interval`: frequency of the snapshots. The interval can be specified in days, hours, or minutes using d, h, m suffix respectively.
    * `startTime`: optional, determines at what time the snapshot process starts, specified using the ISO 8601 time format.
  * `snapshotRetention`: allow to manage retention policies:
    * `path`: filesystem source path to apply the retention on
    * `duration`:
* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `placement`: The mds pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](https://github.com/rook/rook/blob/{{ branchName }}/deploy/examples/cluster.yaml).
* `resources`: Set resource requests/limits for the Filesystem MDS Pod(s), see [MDS Resources Configuration Settings](#mds-resources-configuration-settings)
* `priorityClassName`: Set priority class name for the Filesystem MDS Pod(s)

### MDS Resources Configuration Settings

The format of the resource requests/limits structure is the same as described in the [Ceph Cluster CRD documentation](ceph-cluster-crd.md#resource-requirementslimits).

If the memory resource limit is declared Rook will automatically set the MDS configuration `mds_cache_memory_limit`. The configuration value is calculated with the aim that the actual MDS memory consumption remains consistent with the MDS pods' resource declaration.

In order to provide the best possible experience running Ceph in containers, Rook internally recommends the memory for MDS daemons to be at least 4096MB.
If a user configures a limit or request value that is too low, Rook will still run the pod(s) and print a warning to the operator log.
