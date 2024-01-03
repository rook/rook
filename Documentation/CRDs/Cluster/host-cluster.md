---
title: Host Storage Cluster
---

A host storage cluster is one where Rook configures Ceph to store data directly on the host. The Ceph mons will store the metadata on the host (at a path defined by the `dataDirHostPath`), and the OSDs will consume raw devices or partitions.

The Ceph persistent data is stored directly on a host path (Ceph Mons) and on raw devices (Ceph OSDs).

To get you started, here are several example of the Cluster CR to configure the host.

## All Devices

For the simplest possible configuration, this example shows that all devices or partitions should be consumed by Ceph.
The mons will store the metadata on the host node under `/var/lib/rook`.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # see the "Cluster Settings" section below for more details on which image of ceph to run
    image: quay.io/ceph/ceph:v18.2.1
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  storage:
    useAllNodes: true
    useAllDevices: true
```


## Node and Device Filters

More commonly, you will want to be more specific about which nodes and devices where Rook should configure the storage.
The placement settings are very flexible to add node affinity, anti-affinity, or tolerations. For more options, see the [placement documentation](ceph-cluster-crd.md#placement-configuration-settings).

In this example, Rook will only configure Ceph daemons to run on nodes that are labeled with `role=rook-node`,
and more specifically the OSDs will only be created on nodes labeled with `role=rook-osd-node`.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.1
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: true
    useAllDevices: true
    # Only create OSDs on devices that match the regular expression filter, "sdb" in this example
    deviceFilter: sdb
  # To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
  # The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=rook-node' and
  # the OSDs would specifically only be created on nodes labeled with roke=rook-osd-node.
  placement:
    all:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - rook-node
    osd:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - rook-osd-node
```

## Specific Nodes and Devices

If you need fine-grained control for every node and every device that is being configured, individual nodes and their config can be specified. In this example, we see that specific node names and devices can be specified.

!!! hint
    Each node's 'name' field should match their 'kubernetes.io/hostname' label.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.1
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    deviceFilter:
    config:
      metadataDevice:
      databaseSizeMB: "1024" # this value can be removed for environments with normal sized disks (100 GB or larger)
    nodes:
    - name: "172.17.4.201"
      devices:             # specific devices to use for storage can be specified for each node
      - name: "sdb" # Whole storage device
      - name: "sdc1" # One specific partition. Should not have a file system on it.
      - name: "/dev/disk/by-id/ata-ST4000DM004-XXXX" # both device name and explicit udev links are supported
      config:         # configuration can be specified at the node level which overrides the cluster level config
    - name: "172.17.4.301"
      deviceFilter: "^sd."
```
