---
title: Cluster CRD
weight: 2600
indent: true
---

# Ceph Cluster CRD
Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs).
There are two different modes to create your cluster, depending on whether storage can be dynamically provisioned on which to base the Ceph cluster.

1. Specify host paths and raw devices
2. Specify the storage class Rook should use to consume storage via PVCs

Following is an example for each of these approaches.


## Host-based Cluster

To get you started, here is a simple example of a CRD to configure a Ceph cluster with all nodes and all devices. Next example is where Mons and OSDs are backed by PVCs.
More examples are included [later in this doc](#samples).

**NOTE** In addition to your CephCluster object, you need to create the namespace, service accounts, and RBAC rules for the namespace you are going to create the CephCluster in.
These resources are defined in the example `common.yaml`.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # see the "Cluster Settings" section below for more details on which image of ceph to run
    image: ceph/ceph:v14.2.3-20190904
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  storage:
    useAllNodes: true
    useAllDevices: true
```

## PVC-based Cluster

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    volumeClaimTemplate:
      spec:
        storageClassName: local-storage
        resources:
          requests:
            storage: 10Gi
  storage:
   storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
          storageClassName: local-storage
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
```

## Settings
Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

- `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster Settings

- `external`:
  - `enable`: if `true`, the cluster will not be managed by Rook but via an external entity. This mode is intended to connect to an existing cluster. In this case, Rook will only consume the external cluster. However, Rook will be able to deploy various daemons in Kubernetes such as object gateways, mds and nfs. If this setting is enabled **all** the other options will be ignored except `cephVersion.image` and `dataDirHostPath`. See [external cluster configuration](#external-cluster).
- `cephVersion`: The version information for launching the ceph daemons.
  - `image`: The image used for running the ceph daemons. For example, `ceph/ceph:v13.2.6-20190604` or `ceph/ceph:v14.2.3-20190904`.
  For the latest ceph images, see the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags/).
  To ensure a consistent version of the image is running across all nodes in the cluster, it is recommended to use a very specific image version.
  Tags also exist that would give the latest version, but they are only recommended for test environments. For example, the tag `v14` will be updated each time a new nautilus build is released.
  Using the `v14` or similar tag is not recommended in production because it may lead to inconsistent versions of the image running across different nodes in the cluster.
  - `allowUnsupported`: If `true`, allow an unsupported major version of the Ceph release. Currently `mimic` and `nautilus` are supported, so `octopus` would require this to be set to `true`. Should be set to `false` in production.
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
- `annotations`: [annotations configuration settings](#annotations-configuration-settings)
- `placement`: [placement configuration settings](#placement-configuration-settings)
- `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
- `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  - `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field, then `useAllNodes` must be set to `false`.
  - `topologyAware`: `true` or `false`, indicating whether Rook will look for and use topology/failure domain labels on Kubernetes nodes (e.g. "region" or "zone") as part of the CRUSH location for OSDs.
  Node labels must follow the formatting of the failure domain [well-known labels](https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/).
  - `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  See [node settings](#node-settings) below.
  - `config`: Config settings applied to all OSDs on the node unless overridden by `devices` or `directories`. See the [config settings](#osd-configuration-settings) below.
  - [storage selection settings](#storage-selection-settings)
  - [Storage Class Device Sets](#storage-class-device-sets)
- `disruptionManagement`: The section for configuring management of daemon disruptions
  - `managePodBudgets`: if `true`, the operator will create and manage PodDsruptionBudgets for OSD, Mon, RGW, and MDS daemons. OSD PDBs are managed dynamically via the strategy outlined in the [design](https://github.com/rook/rook/blob/master/design/ceph-managed-disruptionbudgets.md). The operator will block eviction of OSDs by default and unblock them safely when drains are detected.
  - `osdMaintenanceTimeout`: is a duration in minutes that determines how long an entire failureDomain like `region/zone/host` will be held in `noout` (in addition to the default DOWN/OUT interval) when it is draining. This is only relevant when  `managePodBudgets` is `true`. The default value is `30` minutes.

### Mon Settings

- `count`: Set the number of mons to be started. The number should be odd and between `1` and `9`. If not specified the default is set to `3` and `allowMultiplePerNode` is also set to `true`.
- `allowMultiplePerNode`: Enable (`true`) or disable (`false`) the placement of multiple mons on one node. Default is `false`.
- `volumeClaimTemplate`: A `PersistentVolumeSpec` used by Rook to create PVCs
  for monitor storage. This field is optional, and when not provided, HostPath
  volume mounts are used.  The current set of fields from template that are used
  are `storageClassName` and the `storage` resource request and limit. The
  default storage size request for new PVCs is `10Gi`. Ensure that associated
  storage class is configured to use `volumeBindingMode: WaitForFirstConsumer`.
  This setting only applies to new monitors that are created when the requested
  number of monitors increases, or when a monitor fails and is recreated. An
  [example CRD configuration is provided below](#using-pvc-storage-for-monitors).

If these settings are changed in the CRD the operator will update the number of mons during a periodic check of the mon health, which by default is every 45 seconds.

To change the defaults that the operator uses to determine the mon health and whether to failover a mon, the following environment variables can be changed in [operator.yaml](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml). The intervals should be small enough that you have confidence the mons will maintain quorum, while also being long enough to ignore network blips where mons are failed over too often.
- `ROOK_MON_HEALTHCHECK_INTERVAL`: The frequency with which to check if mons are in quorum (default is 45 seconds)
- `ROOK_MON_OUT_TIMEOUT`: The interval to wait before marking a mon as "out" and starting a new mon to replace it in the quorum (default is 600 seconds)

### Node Settings
In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.
If a node does not specify any configuration then it will inherit the cluster level settings.

- `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
- `config`: Config settings applied to all OSDs on the node unless overridden by `devices` or `directories`. See the [config settings](#osd-configuration-settings) below.
- [storage selection settings](#storage-selection-settings)

When `useAllNodes` is set to `true`, Rook attempts to make Ceph cluster management as hands-off as
possible while still maintaining reasonable data safety. If a usable node comes online, Rook will
begin to use it automatically. To maintain a balance between hands-off usability and data safety,
Nodes are removed from Ceph as OSD hosts only (1) if the node is deleted from Kubernetes itself or
(2) if the node has its taints or affinities modified in such a way that the node is no longer
usable by Rook. Any changes to taints or affinities, intentional or unintentional, may affect the
data reliability of the Ceph cluster. In order to help protect against this somewhat, deletion of
nodes by taint or affinity modifications must be "confirmed" by deleting the Rook-Ceph operator pod
and allowing the operator deployment to restart the pod.

For production clusters, we recommend that `useAllNodes` is set to `false` to prevent the Ceph
cluster from suffering reduced data reliability unintentionally due to a user mistake. When
`useAllNodes` is set to `false`, Rook relies on the user to be explicit about when nodes are added
to or removed from the Ceph cluster. Nodes are only added to the Ceph cluster if the node is added
to the Ceph cluster resource. Similarly, nodes are only removed if the node is removed from the Ceph
cluster resource.

#### Node Updates
Nodes can be added and removed over time by updating the Cluster CRD, for example with `kubectl -n rook-ceph edit cephcluster rook-ceph`.
This will bring up your default text editor and allow you to add and remove storage nodes from the cluster.
This feature is only available when `useAllNodes` has been set to `false`.

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
  - `name`: The name of the device (e.g., `sda`)
  - `config`: Device-specific config settings. See the [config settings](#osd-configuration-settings) below
- `directories`:  A list of directory paths that will be included in the storage cluster. Note that using two directories on the same physical device can cause a negative performance impact.
  - `path`: The path on disk of the directory (e.g., `/rook/storage-dir`)
  - `config`: Directory-specific config settings. See the [config settings](#osd-configuration-settings) below
- `location`: Location information about the cluster to help with data placement, such as region or data center.  This is directly fed into the underlying Ceph CRUSH map. The type of this field is `string`. For example, to add data center location information, set this field to `rack=rack1`.  More information on CRUSH maps can be found in the [ceph docs](http://docs.ceph.com/docs/master/rados/operations/crush-map/)
- `storageClassDeviceSets`: Explained in [Storage Class Device Sets](#storage-class-device-sets)

### Storage Class Device Sets
The following are the settings for Storage Class Device Sets which can be configured to create OSDs that are backed by block mode PVs.

- `name`: A name for the set.
- `count`: The number of devices in the set.
- `resources`: The CPU and RAM requests/limits for the devices.(Optional)
- `placement`: The placement criteria for the devices. Default is no placement criteria.(Optional)
- `portable`: If `true`, the OSDs will be allowed to move between nodes during failover. This requires a storage class that supports portability (e.g. `aws-ebs`, but not the local storage provisioner). If `false`, the OSDs will be assigned to a node permanently. Rook will configure Ceph's CRUSH map to support the portability.
- `volumeClaimTemplates`: A list of PVC templates to use for provisioning the underlying storage devices.
  - `resources.requests.storage`: The desired capacity for the underlying storage devices.
  - `storageClassName`: The StorageClass to provision PVCs from. Default would be to use the cluster-default StorageClass.
  - `volumeMode`: The volume mode to be set for the PVC. Which should be Block
  - `accessModes`: The access mode for the PVC to be bound by OSD.

### OSD Configuration Settings
The following storage selection settings are specific to Ceph and do not apply to other backends. All variables are key-value pairs represented as strings.

- `metadataDevice`: Name of a device to use for the metadata of OSDs on each node.  Performance can be improved by using a low latency device (such as SSD or NVMe) as the metadata device, while other spinning platter (HDD) devices on a node are used to store data. Provisioning will fail if the user specifies a `metadataDevice` but that device is not used as a metadata device by Ceph. Notably, `ceph-volume` will not use a device of the same device class (HDD, SSD, NVMe) as OSD devices for metadata, resulting in this failure.
- `storeType`: `filestore` or `bluestore`, the underlying storage format to use for each OSD. The default is set dynamically to `bluestore` for devices, while `filestore` is the default for directories. Set this store type explicitly to override the default. Warning: Bluestore is **not** recommended for directories in production. Bluestore does not purge data from the directory and over time will grow without the ability to compact or shrink.
- `databaseSizeMB`:  The size in MB of a bluestore database. Include quotes around the size.
- `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL). Include quotes around the size.
- `journalSizeMB`:  The size in MB of a filestore journal. Include quotes around the size.
- `osdsPerDevice`**: The number of OSDs to create on each device. High performance devices such as NVMe can handle running multiple OSDs. If desired, this can be overridden for each node and each device.
- `encryptedDevice`**: Encrypt OSD volumes using dmcrypt ("true" or "false"). By default this option is disabled. See http://docs.ceph.com/docs/nautilus/ceph-volume/lvm/encryption/ for more information on encryption in Ceph.

** **NOTE:** Depending on the Ceph image running in your cluster, OSDs will be configured differently. Newer images will configure OSDs with `ceph-volume`, which provides support for `osdsPerDevice`, `encryptedDevice`, as well as other features that will be exposed in future Rook releases. OSDs created prior to Rook v0.9 or with older images of Luminous and Mimic are not created with `ceph-volume` and thus would not support the same features. For `ceph-volume`, the following images are supported:
- Luminous 12.2.10 or newer
- Mimic 13.2.3 or newer
- Nautilus

### Annotations Configuration Settings
Annotations can be specified so that the Rook components will have those annotations added to them.

You can set annotations for Rook components through the a list of key value pairs:

- `all`: Set annotations for all components
- `mgr`: Set annotations for MGRs
- `mon`: Set annotations for mons
- `osd`: Set annotations for OSDs
- `rbdmirror`: Set annotations for RBD Mirrors

When other keys are set, `all` will be merged together with the specific component.

### Placement Configuration Settings
Placement configuration for the cluster services. It includes the following keys: `mgr`, `mon`, `osd`, `rbdmirror` and `all`. Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

A Placement configuration is specified (according to the kubernetes PodSpec) as:

- `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity-beta-feature)
- `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
- `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)

The `mon` pod does not allow `Pod` affinity or anti-affinity. Instead, `mon`s have built-in anti-affinity with each other through the operator. The operator determines which nodes should run a `mon`. Each `mon` is then tied to a node with a node selector using a hostname.
See the [mon design doc](https://github.com/rook/rook/blob/master/design/mon-health.md) for more details on the `mon` failover design.

The Rook Ceph operator creates a Job called `rook-ceph-detect-version` to detect the full Ceph version used by the given `cephVersion.image`. The placement from the `mon` section is used for the Job.

### Cluster-wide Resources Configuration Settings
Resources should be specified so that the Rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep Rook components running when for example a node runs out of memory and the Rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for Rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:

- `mgr`: Set resource requests/limits for MGRs
- `mon`: Set resource requests/limits for mons
- `osd`: Set resource requests/limits for OSDs
- `rbdmirror`: Set resource requests/limits for RBD Mirrors

In order to provide the best possible experience running Ceph in containers, Rook internally enforces minimum memory limits if resource limits are passed.
If a user configures a limit or request value that is too low, Rook will refuse to run the pod(s).
Here are the current minimum amounts of memory in MB to apply so that Rook will agree to run Ceph pods:

- `mon`: 1024MB
- `mgr`: 512MB
- `osd`: 4096MB
- `mds`: 4096MB
- `rbdmirror`: 512MB

### Resource Requirements/Limits
For more information on resource requests/limits see the official Kubernetes documentation: [Kubernetes - Managing Compute Resources for Containers](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container)

- `requests`: Requests for cpu or memory.
  - `cpu`: Request for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).
- `limits`: Limits for cpu or memory.
  - `cpu`: Limit for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  - `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).

### Ceph config overrides
Users are advised to use Ceph's CLI or dashboard to configure Ceph; however, some users may want to
specify some settings they want set in the `CephCluster` manifest. This is possible using the
`configOverrides` section. Config overrides follow the same syntax as [Ceph's documentation for
setting config values](https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#commands),
and configuration is stored in Ceph's centralized configuration database.

Each item in `configOverrides` has `who`, `option`, and `value` properties. `who` can be `global` to
affect all Ceph daemons, a daemon type, an individual Ceph daemon, or a glob matching multiple
daemons. It may also use a
[mask](https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#sections-and-masks).
`option` is the configuration option to be overridden, and `value` is the value to set on the
configuration option. All properties are strings. `value` may also be a null value (`null` or `~` in
YAML; `null` in JSON), which will configure Ceph to use the default value for a parameter.

If an item is removed from the `configOverrides` section, Rook will no longer control the option,
and Ceph will retain the most recently set value.

Rook will only set configuration overrides once when an orchestration is run. For example, on node
addition or removal, when disks change on a node, or when the operator starts (or restarts). This
means that users can change Ceph's configuration from the CLI or dashboard as desired without Rook
constantly fighting to control the setting. This is particularly valuable in situations where values
need to be changed temporarily to debug errors or test changes to tuning parameters.

**WARNING:** Remember that Rook will set these overrides any time an orchestration is run, so user
changes via Ceph's CLI or dashboard to values set here will not persist forever.

Some examples:
```yaml
  configOverrides:
    - who: global
      option: log_file
      value: /var/log/custom-log-dir
    - who: mon
      option: mon_compact_on_start
      value: "true"
    - who: mgr.a
      option: mgr_stats_period
      value: "10"
    - who: osd.*
      option: osd_memory_target
      value: "2147483648"
    - who: osd/rack:foo
      option: debug_ms
      value: "20"
    - who: mon.b
      option: debug_ms
      value: ~
    - who: mon.c
      option: debug_ms
      value: null
```

#### Advanced config
Setting configs in the Ceph mons' centralized database this way requires that at least one mon be
available for the configs to be set. Ceph may also have a small number of very advanced settings
that aren't able to be modified easily in the configuration database. In order to set configurations
before monitors are available or to set problematic configuration settings, the
`rook-config-override` ConfigMap exists, and the `config` field can be set with the contents of a
`ceph.conf` file. It is highly recommended that this only be used when absolutely necessary and that
the `config` be reset to an empty string if/when the configurations are no longer necessary, as this
will make the Ceph cluster less configurable from the CLI and dashboard and may make future tuning
or debugging difficult. Read more about this in the
[advanced configuration docs](ceph-advanced-configuration.md#custom-cephconf-settings).


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
    image: ceph/ceph:v14.2.3-20190904
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
    image: ceph/ceph:v14.2.3-20190904
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
This example is based on the [Storage Configuration: Specific devices](#storage-configuration-specific-devices).
Individual nodes can override the cluster wide specified directories list.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v14.2.3-20190904
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
    image: ceph/ceph:v14.2.3-20190904
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
To control how many resources the Rook components can request/use, you can set requests and limits in Kubernetes for them.
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
    image: ceph/ceph:v14.2.3-20190904
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

**HINT** When setting this prior to `CephCluster` creation, these settings take immediate effect. However, applying this to an already deployed `CephCluster` requires removing each node from the cluster first and then re-adding it with new configuration to take effect. Do this node by node to keep your data safe! Check the result with `ceph osd tree` from the [Rook Toolbox](ceph-toolbox.md). The OSD tree should display the location hierarchy for the nodes that already have been re-added.

This example assumes that there are 3 unique racks (`rack1`, `rack2`, `rack3`) in a data center to use as failure domains.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v14.2.3-20190904
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

To utilize the `location` as a `failureDomain`, specify the corresponding option in the [CephBlockPool](ceph-pool-crd.md)

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

This configuration will split the replication of volumes across unique
racks in the data center setup.

### Using PVC storage for monitors

In the CRD specification below three monitors are created each using a 10Gi PVC
created by Rook using the `local-storage` storage class.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v14.2.3-20190904
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    volumeClaimTemplate:
      spec:
        storageClassName: local-storage
        resources:
          requests:
            storage: 10Gi
  dashboard:
    enabled: true
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

### Using StorageClassDeviceSets

In the CRD specification below, 3 OSDs (having specific placement and resource values) and 3 mons with each using a 10Gi PVC, are created by Rook using the `local-storage` storage class.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    volumeClaimTemplate:
      spec:
        storageClassName: local-storage
        resources:
          requests:
            storage: 10Gi
  cephVersion:
    image: ceph/ceph:v14.2.3-20190904
    allowUnsupported: false
  dashboard:
    enabled: true
  network:
    hostNetwork: false
  storage:
    storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      resources:
        limits:
          cpu: "500m"
          memory: "4Gi"
        requests:
          cpu: "500m"
          memory: "4Gi"
      placement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
                topologyKey: "failure-domain.beta.kubernetes.io/zone"
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          storageClassName: local-storage
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
```

### External cluster

#### Pre-requisites

In order to configure an external Ceph cluster with Rook, we need to inject some information in order to connect to that cluster.
You can use the `cluster/examples/kubernetes/ceph/import-external-cluster.sh` script to achieve that.
The script will look for the following populated environment variables:

* `NAMESPACE`: the namespace where the configmap and secrets should be injected
* `ROOK_EXTERNAL_FSID`: the fsid of the external Ceph cluster, it can be retrieved via the `ceph fsid` command
* `ROOK_EXTERNAL_ADMIN_SECRET`: the external Ceph cluster admin secret key, it can be retrieved via the `ceph auth get-key client.admin` command
* `ROOK_EXTERNAL_CEPH_MON_DATA`: is a common-separated list of running monitors IP address along with their ports, e.g: `a=172.17.0.4:6789,b=172.17.0.5:6789,c=172.17.0.6:6789`. You don't need to specify all the monitors, you can simply pass one and the Operator will discover the rest. The name of the monitor is the name that appears in the `ceph status` ouput.

Example:

```bash
export NAMESPACE=rook-ceph-external
export ROOK_EXTERNAL_FSID=3240b4aa-ddbc-42ee-98ba-4ea7b2a61514
export ROOK_EXTERNAL_ADMIN_SECRET=AQC6Ylxdja+NDBAAB7qy9MEAr4VLLq4dCIvxtg==
export ROOK_EXTERNAL_CEPH_MON_DATA=a=172.17.0.4:6789
```

Then you can simply execute the script like this:

```bash
bash cluster/examples/kubernetes/ceph/import-external-cluster.sh
```

#### CR example

Assuming the above section has successfully completed, here is a CR example:

```
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph-external
  namespace: rook-ceph-external
spec:
  external:
    enable: true
  dataDirHostPath: /var/lib/rook
  cephVersion:
    image: ceph/ceph:v14.2.3-20190904 # the image version **must** match the version of the external Ceph cluster
```

Choose the namespace carefully, if you have an existing cluster managed by Rook, you have likely already injected `common.yaml`.
Additionnally, you now need to inject `common-external.yaml` too.

You can now create it like this:

```bash
kubectl create -f cluster/examples/kubernetes/ceph/cluster-external.yaml
```

If the previous section has not been completed, the Rook Operator will still acknowledge the CR creation but will wait forever to receive connection information.

**WARNING**
If no cluster is managed by the current Rook Operator, you need to inject `common.yaml`, then modify `cluster-external.yaml` and specify `rook-ceph` as `namespace`.
