---
title: Cluster
weight: 32
indent: true
---

# Cluster CRD

Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs). The following settings are
available for a cluster.

## Settings

Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

- `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
- `namespace`: The Kubernetes namespace that will be created for the Ceph cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Ceph cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster settings

- `dataDirHostPath`: The path on the host ([hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)) where backend config and data should be stored for each of the services. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted.
  - On **Minikube** environments, use `/data/rook`. Minikube boots into a tmpfs but it provides some [directories](https://github.com/kubernetes/minikube/blob/master/docs/persistent_volumes.md) where files can be persisted across reboots. Using one of these directories will ensure that Rook's data and configuration files are persisted and that enough storage space is available.
  - If a path is not specified, an [empty dir](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir) will be used and the config will be lost when the pod or host is restarted. This option is **not recommended**.
  - **WARNING**: For test scenarios, if you delete a cluster and start a new cluster on the same hosts, the path used by `dataDirHostPath` must be deleted. Otherwise, stale keys and other config will remain from the previous cluster and the new mons will fail to start.
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
- `hostNetwork`: uses network of the hosts instead of using the SDN below the containers.
- `monCount`: set the number of mons to be started. The number should be odd and between `1` and `9`. Default if not specified is `3`.
For more details on the mons and when to choose a number other than `3`, see the [mon health design doc](https://github.com/rook/rook/blob/master/design/mon-health.md).
- `placement`: [placement configuration settings](#placement-configuration-settings)
- `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
- `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  - `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field below, then `useAllNodes` must be set to `false`.
  - `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  - [storage selection settings](#storage-selection-settings)
  - [storage configuration settings](#storage-configuration-settings)

#### Node updates
Nodes can be added and removed over time by updating the Cluster CRD, for example with `kubectl -n ceph edit cluster ceph`.
This will bring up your default text editor and allow you to add and remove storage nodes from the cluster.
This feature is only available when `useAllNodes` has been set to `false`.

### Node settings

In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.
If a node does not specify any configuration then it will inherit the cluster level settings.
- `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
- `devices`: A list of individual device names belonging to this node to include in the storage cluster.
  - `name`: The name of the device (e.g., `sda`).
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
- `directories`:  A list of directory paths that will be included in the storage cluster. Note that using two directories on the same physical device can cause a negative performance impact.
  - `path`: The path on disk of the directory (e.g., `/rook/storage-dir`).

### Storage Configuration Settings

Below are the settings available, both at the cluster and individual node level, that affect how the selected storage resources will be configured.
- `location`: Location information about the cluster to help with data placement, such as region or data center.  This is directly fed into the underlying Ceph CRUSH map.  More information on CRUSH maps can be found in the [ceph docs](http://docs.ceph.com/docs/master/rados/operations/crush-map/).
- `storeConfig`: Configuration information about the store format for each OSD.
  - `storeType`: `filestore` or `bluestore` (default: `bluestore`), The underlying storage format to use for each OSD.
  - `databaseSizeMB`:  The size in MB of a bluestore database.
  - `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL).
  - `journalSizeMB`:  The size in MB of a filestore journal.

### Placement Configuration Settings

Placement configuration for the cluster services. It includes the following keys: `api`, `mgr`, `mon`, `osd` and `all`. Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

A Placement configuration is specified (according to the kubernetes [PodSpec](https://kubernetes.io/docs/api-reference/v1.6/#podspec-v1-core)) as:
- `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/api-reference/v1.6/#nodeaffinity-v1-core)
- `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/api-reference/v1.6/#podaffinity-v1-core)
- `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/api-reference/v1.6/#podantiaffinity-v1-core)
- `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/api-reference/v1.6/#toleration-v1-core)

The `mon` pod does not allow `Pod` affinity or anti-affinity.
This is because of the mons having built-in anti-affinity with each other through the operator. The operator chooses which nodes are to run a mon on. Each mon is then tied to a node with a node selector using a hostname.
See the [mon design doc](https://github.com/rook/rook/blob/master/design/mon-health.md) for more details on the mon failover design.

### Cluster-wide Resources Configuration Settings

Resources should be specified so that the rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep rook components running when for example a node runs out of memory and the rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:
- `api`: Set resource requests/limits for the API.
- `mgr`: Set resource requests/limits for MGRs.
- `mon`: Set resource requests/limits for Mons.
- `osd`: Set resource requests/limits for OSDs.

### Resource Requirements/Limits

For more information on resource requests/limits see the official Kubernetes documentation: [Kubernetes - Managing Compute Resources for Containers](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container)
- `requests`: Requests for cpu or memory.
  - `cpu`: Request for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).
- `limits`: Limits for cpu or memory.
  - `cpu`: Limit for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).

## Samples

### Storage configuration: All devices

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/ceph
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

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/ceph
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
      directories:         # specific directories to use for storage can be specified for each node
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

### Storage Configuration: Cluster wide Directories

This example is based up on the [Storage Configuration: Specific devices](#storage-configuration-specific-devices).
Individual nodes can override the cluster wide specified directories list.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/ceph
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    storeConfig:
      storeType: bluestore
      databaseSizeMB: 1024 # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: 1024  # this value can be removed for environments with normal sized disks (20 GB or larger)
    directories:
    - path: "/ceph/storage-dir"
    nodes:
    - name: "172.17.4.101"
      directories: # specific directories to use for storage can be specified for each node
      # overrides the above `directories` values for this node
      - path: "/ceph/my-node-storage-dir"
    - name: "172.17.4.201"
```

### Node Affinity

To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage' and
tolerate taints with a key of 'storage-node'.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/ceph
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

### Resource requests/Limits

To control how many resources the rook components can request/use, you can set requests and limits in Kubernetes for them.
You can override these requests/limits for OSDs per node when using `useAllNodes: false` in the `node` item in the `nodes` list.

**WARNING** Before setting resource requests/limits, please take a look at the Ceph documentation for recommendations for each component: [Ceph - Hardware Recommendations](http://docs.ceph.com/docs/master/start/hardware-recommendations/).

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: ceph
spec:
  dataDirHostPath: /var/lib/ceph
  # cluster level resource requests/limits configuration
  resources:
  storage:
    useAllNodes: false
    nodes:
    - name: "172.17.4.201"
      resources:
        limits:
          cpu: "2"
          memory: "4096Mi"
        requests:
          cpu: "2"
          memory: "4096Mi"
```
