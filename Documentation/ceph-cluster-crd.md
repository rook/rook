---
title: Cluster CRD
weight: 2600
indent: true
---

# Ceph Cluster CRD
Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs).

## Sample

To get you started, here is a simple example of a CRD to configure a Ceph cluster with all nodes and all devices. More examples are included [later in this doc](#samples).

**NOTE** In addition to your CephCluster object, you need to create RBAC rules for the namespace you are going to create the CephCluster in, see [Common Cluster Resources](#common-cluster-resources) section below.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # see the "Cluster Settings" section below for more details on which image of ceph to run
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  storage:
    useAllNodes: true
    useAllDevices: true
```

In addition to the CRD, you will also need to create a namespace, role, and role binding as seen in the [common cluster resources](#common-cluster-resources) below.

## Settings
Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

- `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster Settings

- `cephVersion`: The version information for launching the ceph daemons.
  - `image`: The image used for running the ceph daemons. For example, `ceph/ceph:v12.2.9-20181026` or `ceph/ceph:v13.2.2-20181023`.
  For the latest ceph images, see the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags/).
  To ensure a consistent version of the image is running across all nodes in the cluster, it is recommended to use a very specific image version.
  Tags also exist that would give the latest version, but they are only recommended for test environments. For example, the tag `v13` will be updated each time a new mimic build is released.
  Using the `v13` or similar tag is not recommended in production because it may lead to inconsistent versions of the image running across different nodes in the cluster.
  - `allowUnsupported`: If `true`, allow an unsupported major version of the Ceph release. Currently only `luminous` and `mimic` are supported, so `nautilus` would require this to be set to `true`. Should be set to `false` in production.
- `dataDirHostPath`: The path on the host ([hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)) where config and data should be stored for each of the services. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted.
  - On **Minikube** environments, use `/data/rook`. Minikube boots into a tmpfs but it provides some [directories](https://github.com/kubernetes/minikube/blob/master/docs/persistent_volumes.md) where files can be persisted across reboots. Using one of these directories will ensure that Rook's data and configuration files are persisted and that enough storage space is available.
  - **WARNING**: For test scenarios, if you delete a cluster and start a new cluster on the same hosts, the path used by `dataDirHostPath` must be deleted. Otherwise, stale keys and other config will remain from the previous cluster and the new mons will fail to start.
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
- `dashboard`: Settings for the Ceph dashboard. To view the dashboard in your browser see the [dashboard guide](ceph-dashboard.md).
  - `enabled`: Whether to enable the dashboard to view cluster status
  - `urlPrefix`: Allows to serve the dashboard under a subpath (useful when you are accessing the dashboard via a reverse proxy)
  - `port`: Allows to change the default port where the dashboard is served
  - `ssl`: Whether to serve the dashboard via SSL, ignored on Ceph versions older than `13.2.2`
- `network`: The network settings for the cluster
  - `hostNetwork`: uses network of the hosts instead of using the SDN below the containers.
- `mon`: contains mon related options [mon settings](#mon-settings)
For more details on the mons and when to choose a number other than `3`, see the [mon health design doc](https://github.com/rook/rook/blob/master/design/mon-health.md).
- `rbdMirroring`: The settings for rbd mirror daemon(s). Configuring which pools or images to be mirrored must be completed in the rook toolbox by running the
[rbd mirror](http://docs.ceph.com/docs/mimic/rbd/rbd-mirroring/) command.
  - `workers`: The number of rbd daemons to perform the rbd mirroring between clusters.
- `placement`: [placement configuration settings](#placement-configuration-settings)
- `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
- `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  - `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field below, then `useAllNodes` must be set to `false`.
  - `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  - `config`: Config settings applied to all OSDs on the node unless overridden by `devices` or `directories`. See the [config settings](#osd-configuration-settings) below.
  - [storage selection settings](#storage-selection-settings)

#### Node Updates
Nodes can be added and removed over time by updating the Cluster CRD, for example with `kubectl -n rook-ceph edit cephcluster rook-ceph`.
This will bring up your default text editor and allow you to add and remove storage nodes from the cluster.
This feature is only available when `useAllNodes` has been set to `false`.

### Mon Settings

- `count`: set the number of mons to be started. The number should be odd and between `1` and `9`. If not specified the default is set to `3` and `allowMultiplePerNode` is also set to `true`.
- `allowMultiplePerNode`: enable (`true`) or disable (`false`) the placement of multiple mons on one node. Default is `false`.

If these settings are changed in the CRD the operator will update the number of mons during a periodic check of the mon health, which by default is every 45 seconds.

To change the defaults that the operator uses to determine the mon health and whether to failover a mon, the following environment variables can be changed in [operator.yaml](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml). The intervals should be small enough that you have confidence the mons will maintain quorum, while also being
log enough to ignore network blips where mons are failed over too often.
- `ROOK_MON_HEALTHCHECK_INTERVAL`: The frequency with which to check if mons are in quorum (default is 45 seconds)
- `ROOK_MON_OUT_TIMEOUT`: The interval to wait before marking a mon as "out" and starting a new mon to replace it in the quroum (default is 5 minutes)

### Node Settings
In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.
If a node does not specify any configuration then it will inherit the cluster level settings.

- `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
- `config`: Config settings applied to all OSDs on the node unless overridden by `devices` or `directories`. See the [config settings](#osd-configuration-settings) below.
- [storage selection settings](#storage-selection-settings)

### Storage Selection Settings
Below are the settings available, both at the cluster and individual node level, for selecting which storage resources will be included in the cluster.

- `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by OSDs. **Not recommended** unless you have a very controlled environment where you will not risk formatting of devices with existing data. When `true`, all devices will be used except those with partitions created or a local filesystem. Is overridden by `deviceFilter` if specified.
- `deviceFilter`: A regular expression that allows selection of devices to be consumed by OSDs.  If individual devices have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
  - `sdb`: Only selects the `sdb` device if found
  - `^sd.`: Selects all devices starting with `sd`
  - `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
  - `^s`: Selects all devices that start with `s`
  - `^[^r]`: Selects all devices that do *not* start with `r`
- `devices`: A list of individual device names belonging to this node to include in the storage cluster.
  - `name`: The name of the device (e.g., `sda`).
  - `config`: Device-specific config settings. See the [config settings](#osd-configuration-settings) below.
- `directories`:  A list of directory paths that will be included in the storage cluster. Note that using two directories on the same physical device can cause a negative performance impact.
  - `path`: The path on disk of the directory (e.g., `/rook/storage-dir`).
  - `config`: Directory-specific config settings. See the [config settings](#osd-configuration-settings) below.
- `location`: Location information about the cluster to help with data placement, such as region or data center.  This is directly fed into the underlying Ceph CRUSH map. The type of this field is `string`. For example, to add datacenter location information, set this field to `rack=rack1`.  More information on CRUSH maps can be found in the [ceph docs](http://docs.ceph.com/docs/master/rados/operations/crush-map/).


### OSD Configuration Settings
The following storage selection settings are specific to Ceph and do not apply to other backends. All variables are key-value pairs represented as strings.

- `metadataDevice`: Name of a device to use for the metadata of OSDs on each node.  Performance can be improved by using a low latency device (such as SSD or NVMe) as the metadata device, while other spinning platter (HDD) devices on a node are used to store data.
- `storeType`: `filestore` or `bluestore`, the underlying storage format to use for each OSD. The default is set dynamically to `bluestore` for devices, while `filestore` is the default for directories. Set this store type explicitly to override the default. Warning: Bluestore is **not** recommended for directories in production. Bluestore does not purge data from the directory and over time will grow without the ability to compact or shrink.
- `databaseSizeMB`:  The size in MB of a bluestore database. Include quotes around the size.
- `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL). Include quotes around the size.
- `journalSizeMB`:  The size in MB of a filestore journal. Include quotes around the size.
- `osdsPerDevice`**: The number of OSDs to create on each device. High performance devices such as NVMe can handle running multiple OSDs. If desired, this can be overridden for each node and each device.

** **NOTE:** Depending on the Ceph image running in your cluster, OSDs will be configured differently. Newer images will configure OSDs with `ceph-volume`, which provides support for `osdsPerDevice` as well as other features that will be exposed in future Rook releases. OSDs created prior to Rook v0.9 or with older images of Luminous and Mimic are not created with `ceph-volume` and thus would not support the same features. For `ceph-volume`, the following images are supported:
- Luminous 12.2.10 or newer
- Mimic 13.2.3 or newer
- Nautilus

### Placement Configuration Settings
Placement configuration for the cluster services. It includes the following keys: `mgr`, `mon`, `osd` and `all`. Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

A Placement configuration is specified (according to the kubernetes PodSpec) as:

- `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity-beta-feature)
- `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)

The `mon` pod does not allow `Pod` affinity or anti-affinity.
This is because of the mons having built-in anti-affinity with each other through the operator. The operator chooses which nodes are to run a mon on. Each mon is then tied to a node with a node selector using a hostname.
See the [mon design doc](https://github.com/rook/rook/blob/master/design/mon-health.md) for more details on the mon failover design.

### Cluster-wide Resources Configuration Settings
Resources should be specified so that the rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep rook components running when for example a node runs out of memory and the rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:

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
Here are several samples for configuring Ceph clusters. Each of the samples must also include the namespace and corresponding access granted for management by the Ceph operator. See the [common cluster resources](#common-cluster-resources) below.

### Storage configuration: All devices
```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter:
    location:
    config:
      metadataDevice:
      databaseSizeMB: "1024" # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: "1024"  # this value can be removed for environments with normal sized disks (20 GB or larger)
      osdsPerDevice: "1"
```

### Storage Configuration: Specific devices
Individual nodes and their config can be specified so that only the named nodes below will be used as storage resources.
Each node's 'name' field should match their 'kubernetes.io/hostname' label.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    deviceFilter:
    location:
    config:
      metadataDevice:
      databaseSizeMB: "1024" # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: "1024"  # this value can be removed for environments with normal sized disks (20 GB or larger)
    nodes:
    - name: "172.17.4.101"
      directories:         # specific directories to use for storage can be specified for each node
      - path: "/rook/storage-dir"
    - name: "172.17.4.201"
      devices:             # specific devices to use for storage can be specified for each node
      - name: "sdb"
      - name: "sdc"
      config:         # configuration can be specified at the node level which overrides the cluster level config
        storeType: bluestore
    - name: "172.17.4.301"
      deviceFilter: "^sd."
```

### Storage Configuration: Cluster wide Directories
This example is based up on the [Storage Configuration: Specific devices](#storage-configuration-specific-devices).
Individual nodes can override the cluster wide specified directories list.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    config:
      databaseSizeMB: "1024" # this value can be removed for environments with normal sized disks (100 GB or larger)
      journalSizeMB: "1024"  # this value can be removed for environments with normal sized disks (20 GB or larger)
    directories:
    - path: "/rook/storage-dir"
    nodes:
    - name: "172.17.4.101"
      directories: # specific directories to use for storage can be specified for each node
      # overrides the above `directories` values for this node
      - path: "/rook/my-node-storage-dir"
    - name: "172.17.4.201"
```

### Node Affinity
To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage' and
tolerate taints with a key of 'storage-node'.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  # enable the ceph dashboard for viewing cluster status
  dashboard:
    enabled: true
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
    mon:
      nodeAffinity:
      tolerations:
    osd:
      nodeAffinity:
      tolerations:
```

### Resource Requests/Limits
To control how many resources the rook components can request/use, you can set requests and limits in Kubernetes for them.
You can override these requests/limits for OSDs per node when using `useAllNodes: false` in the `node` item in the `nodes` list.

**WARNING** Before setting resource requests/limits, please take a look at the Ceph documentation for recommendations for each component: [Ceph - Hardware Recommendations](http://docs.ceph.com/docs/master/start/hardware-recommendations/).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  # enable the ceph dashboard for viewing cluster status
  dashboard:
    enabled: true
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

### Custom Location Information On Node Level
For each individual node a `location` can be configured. The provided information is fed directly into the CRUSH map of Ceph. More information on CRUSH maps can be found in the [ceph docs](http://docs.ceph.com/docs/master/rados/operations/crush-map/).

**HINT** When setting this prior to `CephCluster` creation, these settings take immediate effect. However, applying this to an already deployed `CephCluster` requires to remove each node from the cluster first and then re-add it with new configuration to take effect. Do this node by node to keep your data safe! You can check the result with `ceph osd tree` from the [Rook Toolbox](ceph-toolbox.md) in your setup. The OSD tree should display your location hierarchy for the nodes you already re-added. 

This example assumes you have 3 unique racks in your datacenter and want to use them as failure domain

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  # cluster level storage configuration and selection
  storage:
    useAllNodes: false
    useAllDevices: false
    deviceFilter:
    location:
    config:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
    nodes:
    - name: "node1"
      location: rack=rack1   # a location can be specified for every node and will be added to the CRUSH map
      devices:
      - name: "sdb"
      - name: "sdc"
    - name: "node2"
      location: rack=rack2   # a location can be specified for every node and will be added to the CRUSH map
      devices:
      - name: "sdb"
      - name: "sdc"
    - name: "node3"
      location: rack=rack3   # a location can be specified for every node and will be added to the CRUSH map
      devices:
      - name: "sdb"
      - name: "sdc"
```

To utilize the `location` as a `failureDomain`, specify the corresponding option in your [CephBlockPool](ceph-pool-crd.md)

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: rack        # this uses the location setting from the CephCluster
  replicated:
    size: 3
```

This configuration will split replication of your volumes across unique racks in your datacenter setup.

## Common Cluster Resources
Each Ceph cluster must be created in a namespace and also give access to the Rook operator to manage the cluster in the namespace. Creating the namespace and these controls must be added to each of the examples previously shown.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: rook-ceph
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
  namespace: rook-ceph
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: rook-ceph
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
# Aspects of ceph-mgr that require access to the system namespace
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system
  namespace: rook-ceph
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
---
# Aspects of ceph-mgr that operate within the cluster's namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: rook-ceph
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - services
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - ceph.rook.io
  resources:
  - "*"
  verbs:
  - "*"
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph-system
---
# Allow the osd pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-osd
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: rook-ceph
---
# Allow the ceph mgr to access the cluster-specific resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: rook-ceph
---
# Allow the ceph mgr to access the rook system resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-system
  namespace: rook-ceph-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: rook-ceph
---
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-mgr-cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: rook-ceph
```
