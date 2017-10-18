---
title: Cluster CRD
weight: 17
indent: true
---

# Creating Rook Clusters

Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs). The following settings are
available for a cluster.

## Settings

Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

- `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster settings

- `versionTag`: The version (tag) of the `rook/rook` container that will be deployed. Upgrades are not yet supported if this setting is updated for an existing cluster, but upgrades will be coming.
- `dataDirHostPath`: The host path where config and data should be stored for each of the services. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted.  Therefore, for test scenarios, the path must be deleted if you are going to delete a cluster and start a new cluster on the same hosts.  More details can be found in the Kubernetes [host path docs](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath).
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
- `hostNetwork`: uses network of the hosts instead of using the SDN below the containers.
- `monCount`: set the amount of mons to be started. The number must be odd and between `1` and `9`. Default if not specified is `3`.
- `placement`: [placement configuration settings](#placement-configuration-settings)
- `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  - `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field below, then `useAllNodes` must be set to `false`.
  - `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  - [storage selection settings](#storage-selection-settings)
  - [storage configuration settings](#storage-configuration-settings)

### Node settings

In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.  If a node does not specify any configuration then it will inherit the cluster level settings.
- `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
- `devices`: A list of individual device names belonging to this node to include in the storage cluster.
  - `name`: The name of the device (e.g., `sda`).
- `directories`:  A list of directory paths on this node that will be included in the storage cluster.  Note that using two directories on the same physical device can cause a negative performance impact.
  - `path`: The path on disk of the directory (e.g., `/rook/storage-dir`).
- [storage selection settings](#storage-selection-settings)
- [storage configuration settings](#storage-configuration-settings)

### Storage Selection Settings

Below are the settings available, both at the cluster and individual node level, for selecting which storage resources will be included in the cluster.
- `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by OSDs. **Not recommended** unless you have a very controlled environment where you will not risk formatting of devices with existing data. When `true`, all devices will be used except those with partitions created or a local filesystem. Is overridden by `deviceFilter` if specified.
- `deviceFilter`: A regular expression that allows selection of devices to be consumed by OSDs.  If individual devices have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
  - `sdb`: Only selects the `sdb` device if found
  - `^sd.`: Selects all devices starting with `sd`
  - `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
  - `^s`: Selects all devices that start with `s`
  - `^[^r]`: Selects all devices that do *not* start with `r`
- `metadataDevice`: Name of a device to use for the metadata of OSDs on each node.  Performance can be improved by using a low latency device (such as SSD or NVMe) as the metadata device, while other spinning platter (HDD) devices on a node are used to store data.

### Storage Configuration Settings

Below are the settings available, both at the cluster and individual node level, that affect how the selected storage resources will be configured.
- `location`: Location information about the cluster to help with data placement, such as region or data center.  This is directly fed into the underlying Ceph CRUSH map.  More information on CRUSH maps can be found in the [ceph docs](http://docs.ceph.com/docs/master/rados/operations/crush-map/).
- `storeConfig`: Configuration information about the store format for each OSD.
  - `storeType`: `filestore` or `bluestore` (default: `bluestore`), The underlying storage format to use for each OSD.
  - `databaseSizeMB`:  The size in MB of a bluestore database.
  - `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL).
  - `journalSizeMB`:  The size in MB of a filestore journal.

### Placement Configuration Settings

Placement configuration for the cluster services. It includes the following keys: `api`, `mds`, `mon`, `osd`, `rgw` and `all`. Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

A Placement configuration is specified (according to the kubernetes [PodSpec](https://kubernetes.io/docs/api-reference/v1.6/#podspec-v1-core)) as:
- `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/api-reference/v1.6/#nodeaffinity-v1-core)
- `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/api-reference/v1.6/#podaffinity-v1-core)
- `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/api-reference/v1.6/#podantiaffinity-v1-core)
- `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/api-reference/v1.6/#toleration-v1-core)

## Samples

### Storage configuration: All devices

```
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook
  namespace: rook
spec:
  versionTag: master
  dataDirHostPath:
  # cluster level storage configuration and selection
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter:
    metadataDevice:
    location:
    storeConfig:
      storeType: bluestore
      databaseSizeMB: 1024 # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: 1024  # this value can be removed for environments with normal sized disks (20 GB or larger)
```

### Storage Configuration: Specific devices

Individual nodes and their config can be specified so that only the named nodes below will be used as storage resources.
Each node's 'name' field should match their 'kubernetes.io/hostname' label.

```
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook
  namespace: rook
spec:
  versionTag: master
  dataDirHostPath:
  # cluster level storage configuration and selection
  storage:                
    useAllNodes: false
    useAllDevices: false
    deviceFilter:
    metadataDevice:
    location:
    storeConfig:
      storeType: bluestore
      databaseSizeMB: 1024 # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: 1024  # this value can be removed for environments with normal sized disks (20 GB or larger)
    nodes:
    - name: "172.17.4.101"
      directories:         # specific directores to use for storage can be specified for each node
      - path: "/rook/storage-dir"
    - name: "172.17.4.201"
      devices:             # specific devices to use for storage can be specified for each node
      - name: "sdb"
      - name: "sdc"
      storeConfig:         # configuration can be specified at the node level which overrides the cluster level config
        storeType: filestore
    - name: "172.17.4.301"
      deviceFilter: "^sd."
```

### Node Affinity

To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage' and
tolerate taints with a key of 'storage-node'.

```
apiVersion: v1
kind: Namespace
metadata:
  name: rook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook
  namespace: rook
spec:
  versionTag: master
  dataDirHostPath:
  placement:
    all:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - storage-node
      tolerations:
      - key: storage-node
        operator: Exists
    api:
      nodeAffinity:
      tolerations:
    mgr:
      nodeAffinity:
      tolerations:
    mon:
      nodeAffinity:
      tolerations:
    osd:
      nodeAffinity:
      tolerations:
```
