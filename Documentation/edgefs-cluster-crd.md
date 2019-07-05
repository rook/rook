---
title: Cluster CRD
weight: 4100
indent: true
---

# EdgeFS Cluster CRD
Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs).

## Sample
To get you started, here is a simple example of a CRD to configure a EdgeFS cluster with just one local per-host directory /data:

```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  serviceAccount: rook-edgefs-cluster
  dataDirHostPath: /data
  storage:
    useAllNodes: true
  # A key/value list of annotations
  annotations:
  #  all:
  #    key: value
  #  prepare:
  #  mgr:
  #  target:
```

or if you have raw block devices provisioned, it can dynamically detect, format and utilize all raw devices on all nodes with simple CRD as below:

```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  serviceAccount: rook-edgefs-cluster
  dataDirHostPath: /data
  storage:
    useAllNodes: true
    useAllDevices: true
  # A key/value list of annotations
  annotations:
  #  all:
  #    key: value
  #  prepare:
  #  mgr:
  #  target:
```

In addition to the CRD, you will also need to create a namespace, role, and role binding as seen in the [common cluster resources](#common-cluster-resources) below.

## Settings
Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels, e.g. individual nodes.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata
- `name`: The name that will be used internally for the EdgeFS cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.
- `edgefsImageName`: EdgeFS image to use. If not specified then `edgefs/edgefs:latest` is used. We recommend to specify particular image version for production use, for example `edgefs/edgefs:1.1.8`.

### Cluster Settings
- `dataDirHostPath`: The path on the host ([hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)) where config and data should be stored for each of the services. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted. If `storage` settings not provided then provisioned hostPath will also be used as a storage device for Target pods (automatic provisioning via `rtlfs`).
  - On **Minikube** environments, use `/data/rook`. Minikube boots into a tmpfs but it provides some [directories](https://github.com/kubernetes/minikube/blob/master/docs/persistent_volumes.md) where files can be persisted across reboots. Using one of these directories will ensure that Rook's data and configuration files are persisted and that enough storage space is available.
  - **WARNING**: For test scenarios, if you delete a cluster and start a new cluster on the same hosts, the path used by `dataDirHostPath` must be deleted. Otherwise, stale information and other config will remain from the previous cluster and the new target will fail to start.
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
- `dataVolumeSize`: Alternative to `dataDirHostPath`. If defined then Cluster CRD operator will disregard `dataDirHostPath` setting and instead will automatically claim persistent volume. If `storage` settings not provided then provisioned volume will also be used as a storage device for Target pods (automatic provisioning via `rtlfs`).
- `dashboard`: This specification may be used to override and enable additional [EdgeFS UI Dashboard](edgefs-ui.md) functionality.
  - `localAddr`: Specifies local IP address to be used as Kubernetes external IP.
- `network`: If defined then host network will be enabled for the cluster and services. This is optional and if not defined then `eth0` will be used to construct cluster bucket network.
  - `serverIfName`: Specifies data daemon networking interface name. If not defined then `eth0` is assumed.
  - `brokerIfName`: Specifies broker daemon networking interface name. If not defined then `eth0` is assumed.
- `devicesResurrectMode`: When enabled, this mode attempts to recreate cluster based on previous CRD definition. If this flag set to one of the parameters, then operator will only adjust networking. Often used when clean up of old devices is needed. Only applicable when used with `dataDirHostPath`.
  - `restore`: Attempt to restart and restore previously enabled cluster CRD.
  - `restoreZap`: Attempt to re-initialize previously selected `devices` prior to restore. By default cluster assumes that selected devices have no logical partitions and considered empty.
  - `restoreZapWait`: Attempt to cleanup previously selected `devices` and wait for cluster delete. This is useful when clean up of old devices is needed. Additional containers count should be specified if cluster was originally created with a total per-node capacity that exceeding `maxContainerCapacity` option, e.g., `devicesResurrectMode: "restoreZapWait: 2"`.
- `serviceAccount`: The service account under which the EdgeFS pods will run that will give access to ConfigMaps in the cluster's namespace. If not set, the default of `rook-edgefs-cluster` will be used.
- `chunkCacheSize`: Limit amount of memory allocated for dynamic chunk cache. By default Target pod uses up to 75% of available memory as chunk caching area. This option can influence this allocation strategy.
- `placement`: [placement configuration settings](#placement-configuration-settings)
- `resourceProfile`: Cluster segment wide resource utilization profile (Memory and CPU). Can be `embedded` or `performance` (default). In case of `performance` each Target pod requires at least 8Gi of memory and 4 CPU cores in terms of to operate efficiently. If `resources` limits are set to less then 8Gi of memory then operator will automatically set profile to `embedded`. In `embedded` profile case, Target pod requires 1Gi of memory and 2 CPU cores, where memory allocation is split between number of PLevels (see `rtPLevelOverride` option) with 64Mi minimally per one PLevel, 64Mi for Target pod itself and the rest for chunk cache (see `chunkCacheSize` option) that allocates up to 75% of available memory.
- `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
- `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  - `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field below, then `useAllNodes` must be set to `false`.
  - `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  - [storage selection settings](#storage-selection-settings)
  - [storage configuration settings](#storage-configuration-settings)
- `skipHostPrepare`: By default all nodes selected for EdgeFS deployment will be automatically configured via preparation jobs. If this option set to `true` node configuration will be skipped.
- `trlogProcessingInterval`: Controls for how many seconds cluster would aggregate object modifications prior to processing it by accounting, bucket updates, ISGW Links and notifications components. Has to be defined in seconds and must be composite of 60, i.e. 1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30. Default is 10. Recommended range is 2 - 20. This is cluster wide setting and cannot be easily changed after cluster is created. Any new node added has to reflect exactly the same setting.
- `trlogKeepDays`: Controls for how many days cluster need to keep transaction log interval batches with version manifest references. If you planning to have cluster disconnected from ISGW downlinks for longer period time, consider to increase this value. Default is 7. This is cluster wide setting and cannot be easily changed after cluster is created.
- `maxContainerCapacity`: Overrides default total disks capacity per target container. Default is "132Ti".
#### Node Updates
Nodes can be added and removed over time by updating the Cluster CRD, for example with `kubectl -n rook-edgefs edit cluster.edgefs.rook.io rook-edgefs`.
This will bring up your default text editor and allow you to add and remove storage nodes from the cluster.
This feature is only available when `useAllNodes` has been set to `false`.

### Node Settings
In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.
If a node does not specify any configuration then it will inherit the cluster level settings.
- `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
- `config`: Config settings applied to all VDEVs on the node unless overridden by `devices` or `directories`. See the [config settings](#storage-configuration-settings) below.
- [storage selection settings](#storage-selection-settings)
- [storage configuration settings](#storage-configuration-settings)

### Storage Selection Settings
Below are the settings available, both at the cluster and individual node level, for selecting which storage resources will be included in the cluster.
- `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by Targets. This is recommended for controlled environments where you will not risk formatting of devices with existing data. When `true`, all devices will be used except those with partitions created or a local filesystem. This can be overridden by `deviceFilter`.
- `deviceFilter`: A regular expression that allows selection of devices to be consumed by target.  If individual devices have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
  - `sdb`: Only selects the `sdb` device if found
  - `^sd.`: Selects all devices starting with `sd`
  - `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
  - `^s`: Selects all devices that start with `s`
  - `^[^r]`: Selects all devices that do *not* start with `r`
- `devices`: A list of individual device names belonging to this node to include in the storage cluster. Mixing of `devices` and `directories` on the same node isn't supported.
  - `name`: The name of the device (e.g., `sda`).
  - `fullpath`: The full path to the device (e.g., `/dev/disk/by-id/scsi-35000c5008335c83f`). If specified then `name` can be omitted.
  - `config`: Device-specific config settings. See the [config settings](#target-configuration-settings) below.
- `directories`:  A list of directory paths on the nodes that will be included in the storage cluster. Note that using two directories on the same physical device can cause a negative performance impact. Mixing of `devices` and `directories` on the same node isn't supported. Since EdgeFS is leveraging StatefulSet, directories can only be defined at cluster level.
  - `path`: The path on disk of the directory (e.g., `/rook/storage-dir`).
  - `config`: Directory-specific config settings. See the [config settings](#target-configuration-settings) below.


### Storage Configuration Settings
The following storage selection settings are specific to EdgeFS and do not apply to other backends. All variables are key-value pairs represented as strings. While EdgeFS supports multiple backends, it is not recommended to mix them within same cluster. In case of `devices` (physical or emulated raw disks), EdgeFS will automatically use `rtrd` backend. In all other cases `rtlfs` (local file system) will be used.
**IMPORTANT** Keys needs to be case-sensitive and values has to be provided as strings.
  - `useMetadataOffload`: Dynamically detect appropriate SSD/NVMe device to use for the metadata on each node. Performance can be improved by using a low latency device as the metadata device, while other spinning platter (HDD) devices on a node are used to store data. Typical and recommended proportion is in range of 1:1 - 1:6. Default is false. Applicable only to rtrd.
  - `useMetadataMask`: Defines what parts of metadata needs to be stored on offloaded devices. Default is 0x7d, offload all except second level manifests. For maximum performance, when you have enough SSD/NVMe capacity provisioned, set it to 0xff, i.e. all metadata. Applicable only to rtrd.
  - `useBCache`: When `useMetadataOffload` is true, enable use of BCache. Default is false. Applicable only to rtrd and when host has "bcache" kernel module preloaded.
  - `useBCacheWB`:  When `useMetadataOffload` and `useBCache` is true, this option can enable use of BCache write-back cache. By default BCache only used as read cache in front of HDD. Applicable only to rtrd.
  - `useAllSSD`: When set to true, only SSD/NVMe non rotational devices will be used. Default is false and if `useMetadataOffload` not defined then only rotational devices (HDDs) will be picked up during node provisioning phase.
  - `rtPLevelOverride`:  In case of large devices or directories, it will be automatically partitioned into smaller parts around 500GB each. In case of embedded use cases, lowering the value would allow to operate with smaller memory footprint devices at the cost of performance. This option allows partitioning number override. Default is automatic. Typical and recommended range is 1 - 32.
  - `hddReadAhead`: For all HDD or hybrid (SSD/HDD) use cases, adjusting hddReadAhead may provide significant boost in performance. Set to a value higher then 0, in KBs.
  - `mdReserved`: For hybrid (SSD/HDD) use case, adjusting mdReserved can be necessary when combined with BCache read/write caches. Allowed range 10-99% of automatically calcuated slice.
  - `rtVerifyChid`:  Verify transferred or read payload. Payload can be data or metadata chunk of flexible size between 4K and 8MB. EdgeFS uses SHA-3 variant to cryptographically sign each chunk and uses it for self validation, self healing and FlexHash addressing. In case of low CPU systems verification after networking transfer prior to write can be disabled by setting this parameter to 0. In case of high CPU systems, verification after read but before networking transfer can be enabled by setting this parameter to 2. Default is 1, i.e. verify after networking transfer only. Setting it to 0 may improve CPU utilization at the cost of reduced availability. However, for objects with 3 or more replicas, availability isn't going to be visibly affected.
  - `lmdbPageSize`: Defines default LMDB page size in bytes. Default is 16384. For capacity (all HDD) or hybrid (HDD/SSD) systems consider to increase this value to 32768 to achieve higher throughput performance. For all SSD and small database workloads, consider to decrease this to 8192 to achieve lower latency and higher IOPS. Please be advised that smaller values MAY cause fragmentation. Acceptable values are 4096, 8192, 16384 and 32768.
  - `sync`: Defines default behavior of write operations at device or directory level. Acceptable values are 0, 1 (default), 2, 3.
    - `0`: No syncing will happen. Highest performance possible and good for HPC scratch types of deployments. This option will still sustain crash of pods or software bugs. It will not sustain server power loss an may cause node / device level inconsistency.
    - `1`: Default method. Will guarantee node / device consistency in case of power loss with reduced durability.
    - `2`: Provides better durability in case of power loss at the cost of extra metadata syncing.
    - `3`: Most durable and reliable option at the cost of significant performance impact.
  - `maxSizeGB`: Defines maximum allowed size to use per directory in gigabytes. Applicable only to rtlfs.
  - `zone`: Enables the node's failure domain number. Default value is 0 (no zoning). Zoning number is a logical failure domain tagging mechanism and if enabled then it has to be set for all the nodes in the cluster.

### Placement Configuration Settings
Placement configuration for the cluster services. It includes the following keys: `mgr`, `target` and `all`. Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

A Placement configuration is specified (according to the Kubernetes PodSpec) as:
- `nodeAffinity`: Kubernetes [NodeAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity-beta-feature)
- `podAffinity`: Kubernetes [PodAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `podAntiAffinity`: Kubernetes [PodAntiAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `tolerations`: list of Kubernetes [Toleration](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)

The `mgr` pod does not allow `Pod` affinity or anti-affinity. This is because of the mgrs having built-in anti-affinity with each other through the operator. The operator chooses which nodes are to run a mgr on. Each mgr is then tied to a node with a node selector using a hostname.

### Cluster-wide Resources Configuration Settings
Resources should be specified so that the rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep rook components running when for example a node runs out of memory and the rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:
- `mgr`: Set resource requests/limits for Mgrs.
- `target`: Set resource requests/limits for Targets.

### Resource Requirements/Limits
For more information on resource requests/limits see the official Kubernetes documentation: [Kubernetes - Managing Compute Resources for Containers](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container)
- `requests`: Requests for cpu or memory.
  - `cpu`: Request for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).
- `limits`: Limits for cpu or memory.
  - `cpu`: Limit for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).

### Kubernetes node labeling and selection
By default each Kubernetes node, available to deploy EdgeFS over it, will be treated as "target" EdgeFS instance. But cluster administrator able to label node as "gateway" node, such node will have no devices prepared for EdgeFS and will be used as EdgeFS dedicated service node.
To mark a node as a "gateway", the administrator can add a specific label to a node.
Label format: `<edgefs-namespace>-nodetype=gateway`, where `edgefs-namespace` is current namespace for EdgeFS cluster deployment, by default is `rook-edgefs`.
Example: `kubectl label node "k8s node name" rook-edgefs-nodetype=gateway`

## Samples
Here are several samples for configuring EdgeFS clusters. Each of the samples must also include the namespace and corresponding access granted for management by the EdgeFS operator. See the [common cluster resources](#common-cluster-resources) below.

### Storage configuration: All devices, All SSD/NVMes.
```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  dataDirHostPath: /var/lib/rook
  serviceAccount: rook-edgefs-cluster
  # cluster level storage configuration and selection
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter:
    location:
    config:
      useAllSSD: true
```

### Storage Configuration: Specific devices
Individual nodes and their config can be specified so that only the named nodes below will be used as storage resources.
Each node's 'name' field should match their 'kubernetes.io/hostname' label.

```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  dataDirHostPath: /var/lib/rook
  serviceAccount: rook-edgefs-cluster
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    deviceFilter:
    location:
    config:
      rtVerifyChid: "0"
    nodes:
    - name: "172.17.4.201"
      devices:             # specific devices to use for storage can be specified for each node
      - name: "sdb"
      - name: "sdc"
      config:         # configuration can be specified at the node level which overrides the cluster level config
        rtPLevelOverride: 8
    - name: "172.17.4.301"
      deviceFilter: "^sd."
```

### Node Affinity
To control where various services will be scheduled by Kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on Kubernetes nodes labeled with 'role=storage' and
tolerate taints with a key of 'storage-node'.

```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  dataDirHostPath: /var/lib/rook
  serviceAccount: rook-edgefs-cluster
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
    mgr:
      nodeAffinity:
      tolerations:
    target:
      nodeAffinity:
      tolerations:
```

### Resource requests/Limits
To control how many resources the rook components can request/use, you can set requests and limits in Kubernetes for them.
You can override these requests/limits for Targts per node when using `useAllNodes: false` in the `node` item in the `nodes` list.

```yaml
apiVersion: edgefs.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-edgefs
  namespace: rook-edgefs
spec:
  edgefsImageName: edgefs/edgefs:1.1.8
  dataDirHostPath: /var/lib/rook
  serviceAccount: rook-edgefs-cluster
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

## Common Cluster Resources
Each EdgeFS cluster must be created in a namespace and also give access to the Rook operator to manage the cluster in the namespace. Creating the namespace and these controls must be added to each of the examples previously shown.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-edgefs
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-edgefs-cluster
  namespace: rook-edgefs
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster
  namespace: rook-edgefs
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
- apiGroups: [""]
  resources: ["pods"]
  verbs: [ "get", "list" ]
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster-mgmt
  namespace: rook-edgefs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-edgefs-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: rook-edgefs-system
---
# Allow the pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-edgefs-cluster
  namespace: rook-edgefs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-edgefs-cluster
subjects:
- kind: ServiceAccount
  name: rook-edgefs-cluster
  namespace: rook-edgefs
---
apiVersion: apps/v1
kind: PodSecurityPolicy
metadata:
  name: privileged
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
  - '*'
  allowedCapabilities:
  - '*'
  hostPID: true
  hostIPC: true
  hostNetwork: false
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: privileged-psp-user
rules:
- apiGroups:
  - apps
  resources:
  - podsecuritypolicies
  resourceNames:
  - privileged
  verbs:
  - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-edgefs-system-psp
  namespace: rook-edgefs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: rook-edgefs-system
  namespace: rook-edgefs-system
```
