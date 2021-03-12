---
title: Cluster CRD
weight: 2600
indent: true
---

# Ceph Cluster CRD

Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs).
There are primarily three different modes in which to create your cluster.

1. Specify [host paths and raw devices](#host-based-cluster)
2. Dynamically provision storage underneath Rook by specifying the storage class Rook should use to consume storage [via PVCs](#pvc-based-cluster)
3. Create a [Stretch cluster](#stretch-cluster) that distributes Ceph mons across three zones, while storage (OSDs) is only configured in two zones

Following is an example for each of these approaches. More examples are included [later in this doc](#samples).

## Host-based Cluster

To get you started, here is a simple example of a CRD to configure a Ceph cluster with all nodes and all devices.
The Ceph persistent data is stored directly on a host path (Ceph Mons) and on raw devices (Ceph OSDs).

> **NOTE**: In addition to your CephCluster object, you need to create the namespace, service accounts, and RBAC rules for the namespace you are going to create the CephCluster in.
> These resources are defined in the example `common.yaml`.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # see the "Cluster Settings" section below for more details on which image of ceph to run
    image: ceph/ceph:v15.2.9
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  storage:
    useAllNodes: true
    useAllDevices: true
```

## PVC-based Cluster

In a "PVC-based cluster", the Ceph persistent data is stored on volumes requested from a storage class of your choice.
This type of cluster is recommended in a cloud environment where volumes can be dynamically created and also
in clusters where a local PV provisioner is available.

> **NOTE**: Kubernetes version 1.13.0 or greater is required to provision OSDs on PVCs.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    # see the "Cluster Settings" section below for more details on which image of ceph to run
    image: ceph/ceph:v15.2.9
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
  storage:
   storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      encrypted: false
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

For a more advanced scenario, such as adding a dedicated device you can refer to the [dedicated metadata device for OSD on PVC section](#dedicated-metadata-and-wal-device-for-osd-on-pvc).

## Stretch Cluster

**Experimental Mode**

For environments that only have two failure domains available where data can be replicated, consider
the case where one failure domain is down and the data is still fully available in the
remaining failure domain. To support this scenario, Ceph has recently integrated support for "stretch" clusters.

Rook requires three zones. Two zones (A and B) will each run all types of Rook pods, which we call the "data" zones.
Two mons run in each of the two data zones, while two replicas of the data are in each zone for a total of four data replicas.
The third zone (arbiter) runs a single mon. No other Rook or Ceph daemons need to be run in the arbiter zone.

For this example, we assume the desired failure domain is a zone. Another failure domain can also be specified with a
known [topology node label](#osd-topology) which is already being used for OSD failure domains.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    # Five mons must be created for stretch mode
    count: 5
    allowMultiplePerNode: false
    stretchCluster:
      failureDomainLabel: topology.kubernetes.io/zone
      subFailureDomain: host
      zones:
      - name: a
        arbiter: true
      - name: b
      - name: c
  cephVersion:
    # Stretch cluster support upstream is only planned starting in Ceph Pacific.
    # Until Pacific is released, the stretch cluster is **experimental**.
    image: ceph/daemon-base:latest-master
    allowUnsupported: true
  # Either storageClassDeviceSets or the storage section can be specified for creating OSDs.
  # This example uses all devices for simplicity.
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter: ""
  # OSD placement is expected to include the non-arbiter zones
  placement:
    osd:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: topology.kubernetes.io/zone
              operator: In
              values:
              - b
              - c
```

For more details, see the [Stretch Cluster design doc](https://github.com/rook/rook/blob/master/design/ceph/ceph-stretch-cluster.md).

## Settings

Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

* `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
* `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster Settings

* `external`:
  * `enable`: if `true`, the cluster will not be managed by Rook but via an external entity. This mode is intended to connect to an existing cluster. In this case, Rook will only consume the external cluster. However, Rook will be able to deploy various daemons in Kubernetes such as object gateways, mds and nfs if an image is provided and will refuse otherwise. If this setting is enabled **all** the other options will be ignored except `cephVersion.image` and `dataDirHostPath`. See [external cluster configuration](#external-cluster). If `cephVersion.image` is left blank, Rook will refuse the creation of extra CRs like object, file and nfs.
* `cephVersion`: The version information for launching the ceph daemons.
  * `image`: The image used for running the ceph daemons. For example, `ceph/ceph:v14.2.12` or `ceph/ceph:v15.2.9`. For more details read the [container images section](#ceph-container-images).
  For the latest ceph images, see the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags/).
  To ensure a consistent version of the image is running across all nodes in the cluster, it is recommended to use a very specific image version.
  Tags also exist that would give the latest version, but they are only recommended for test environments. For example, the tag `v14` will be updated each time a new nautilus build is released.
  Using the `v14` or similar tag is not recommended in production because it may lead to inconsistent versions of the image running across different nodes in the cluster.
  * `allowUnsupported`: If `true`, allow an unsupported major version of the Ceph release. Currently `nautilus` and `octopus` are supported. Future versions such as `pacific` would require this to be set to `true`. Should be set to `false` in production.
* `dataDirHostPath`: The path on the host ([hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)) where config and data should be stored for each of the services. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted. Following paths and any of their subpaths **must not be used**: `/etc/ceph`, `/rook` or `/var/log/ceph`.
  * On **Minikube** environments, use `/data/rook`. Minikube boots into a tmpfs but it provides some [directories](https://github.com/kubernetes/minikube/blob/master/site/content/en/docs/handbook/persistent_volumes.md#a-note-on-mounts-persistence-and-minikube-hosts) where files can be persisted across reboots. Using one of these directories will ensure that Rook's data and configuration files are persisted and that enough storage space is available.
  * **WARNING**: For test scenarios, if you delete a cluster and start a new cluster on the same hosts, the path used by `dataDirHostPath` must be deleted. Otherwise, stale keys and other config will remain from the previous cluster and the new mons will fail to start.
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
* `skipUpgradeChecks`: if set to true Rook won't perform any upgrade checks on Ceph daemons during an upgrade. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](ceph-upgrade.md#ceph-version-upgrades).
* `continueUpgradeAfterChecksEvenIfNotHealthy`: if set to true Rook will continue the OSD daemon upgrade process even if the PGs are not clean, or continue with the MDS upgrade even the file system is not healthy.
* `dashboard`: Settings for the Ceph dashboard. To view the dashboard in your browser see the [dashboard guide](ceph-dashboard.md).
  * `enabled`: Whether to enable the dashboard to view cluster status
  * `urlPrefix`: Allows to serve the dashboard under a subpath (useful when you are accessing the dashboard via a reverse proxy)
  * `port`: Allows to change the default port where the dashboard is served
  * `ssl`: Whether to serve the dashboard via SSL, ignored on Ceph versions older than `13.2.2`
* `monitoring`: Settings for monitoring Ceph using Prometheus. To enable monitoring on your cluster see the [monitoring guide](ceph-monitoring.md#prometheus-alerts).
  * `enabled`: Whether to enable prometheus based monitoring for this cluster
  * `externalMgrEndpoints`: external cluster manager endpoints
  * `externalMgrPrometheusPort`: external prometheus manager module port. See [external cluster configuration](#external-cluster) for more details.
  * `rulesNamespace`: Namespace to deploy prometheusRule. If empty, namespace of the cluster will be used.
      Recommended:
    * If you have a single Rook Ceph cluster, set the `rulesNamespace` to the same namespace as the cluster or keep it empty.
    * If you have multiple Rook Ceph clusters in the same Kubernetes cluster, choose the same namespace to set `rulesNamespace` for all the clusters (ideally, namespace with prometheus deployed). Otherwise, you will get duplicate alerts with duplicate alert definitions.
* `network`: For the network settings for the cluster, refer to the [network configuration settings](#network-configuration-settings)
* `mon`: contains mon related options [mon settings](#mon-settings)
For more details on the mons and when to choose a number other than `3`, see the [mon health doc](ceph-mon-health.md).
* `mgr`: manager top level section
  * `count`: set number of ceph managers between `1` to `2`. The default value is 1. This is only needed if plural ceph managers are needed.
  * `modules`: is the list of Ceph manager modules to enable
* `crashCollector`: The settings for crash collector daemon(s).
  * `disable`: is set to `true`, the crash collector will not run on any node where a Ceph daemon runs
  * `daysToRetain`: specifies the number of days to keep crash entries in the Ceph cluster. By default the entries are kept indefinitely.
* `logCollector`: The settings for log collector daemon.
  * `enabled`: if set to `true`, the log collector will run as a side-car next to each Ceph daemon. The Ceph configuration option `log_to_file` will be turned on, meaning Ceph daemons will log on files in addition to still logging to container's stdout. These logs will be rotated. (default: false)
  * `periodicity`: how often to rotate daemon's log. (default: 24h). Specified with a time suffix which may be 'h' for hours or 'd' for days. **Rotating too often will slightly impact the daemon's performance since the signal briefly interrupts the program.**
* `annotations`: [annotations configuration settings](#annotations-and-labels)
* `labels`: [labels configuration settings](#annotations-and-labels)
* `placement`: [placement configuration settings](#placement-configuration-settings)
* `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
* `priorityClassNames`: [priority class names configuration settings](#priority-class-names-configuration-settings)
* `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
  * `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
  If individual nodes are specified under the `nodes` field, then `useAllNodes` must be set to `false`.
  * `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
  `useAllNodes` must be set to `false` to use specific nodes and their config.
  See [node settings](#node-settings) below.
  * `config`: Config settings applied to all OSDs on the node unless overridden by `devices`. See the [config settings](#osd-configuration-settings) below.
  * [storage selection settings](#storage-selection-settings)
  * [Storage Class Device Sets](#storage-class-device-sets)
* `disruptionManagement`: The section for configuring management of daemon disruptions
  * `managePodBudgets`: if `true`, the operator will create and manage PodDisruptionBudgets for OSD, Mon, RGW, and MDS daemons. OSD PDBs are managed dynamically via the strategy outlined in the [design](https://github.com/rook/rook/blob/master/design/ceph/ceph-managed-disruptionbudgets.md). The operator will block eviction of OSDs by default and unblock them safely when drains are detected.
  * `osdMaintenanceTimeout`: is a duration in minutes that determines how long an entire failureDomain like `region/zone/host` will be held in `noout` (in addition to the default DOWN/OUT interval) when it is draining. This is only relevant when  `managePodBudgets` is `true`. The default value is `30` minutes.
  * `manageMachineDisruptionBudgets`: if `true`, the operator will create and manage MachineDisruptionBudgets to ensure OSDs are only fenced when the cluster is healthy. Only available on OpenShift.
  * `machineDisruptionBudgetNamespace`: the namespace in which to watch the MachineDisruptionBudgets.
* `removeOSDsIfOutAndSafeToRemove`: If `true` the operator will remove the OSDs that are down and whose data has been restored to other OSDs. In Ceph terms, the OSDs are `out` and `safe-to-destroy` when they are removed.
* `cleanupPolicy`: [cleanup policy settings](#cleanup-policy)
* `security`: [security settings](#security)

### Ceph container images

Official releases of Ceph Container images are available from [Docker Hub](https://hub.docker.com/r/ceph
).

These are general purpose Ceph container with all necessary daemons and dependencies installed.

| TAG                  | MEANING                                                   |
| -------------------- | --------------------------------------------------------- |
| vRELNUM              | Latest release in this series (e.g., *v14* = Nautilus)    |
| vRELNUM.Y            | Latest stable release in this stable series (e.g., v14.2) |
| vRELNUM.Y.Z          | A specific release (e.g., v14.2.5)                        |
| vRELNUM.Y.Z-YYYYMMDD | A specific build (e.g., v14.2.5-20191203)                 |

A specific will contain a specific release of Ceph as well as security fixes from the Operating System.

### Mon Settings

* `count`: Set the number of mons to be started. The number must be odd and between `1` and `9`. If not specified the default is set to `3`.
* `allowMultiplePerNode`: Whether to allow the placement of multiple mons on a single node. Default is `false` for production. Should only be set to `true` in test environments.
* `volumeClaimTemplate`: A `PersistentVolumeSpec` used by Rook to create PVCs
  for monitor storage. This field is optional, and when not provided, HostPath
  volume mounts are used.  The current set of fields from template that are used
  are `storageClassName` and the `storage` resource request and limit. The
  default storage size request for new PVCs is `10Gi`. Ensure that associated
  storage class is configured to use `volumeBindingMode: WaitForFirstConsumer`.
  This setting only applies to new monitors that are created when the requested
  number of monitors increases, or when a monitor fails and is recreated. An
  [example CRD configuration is provided below](#using-pvc-storage-for-monitors).
* `stretchCluster`: The stretch cluster settings that define the zones (or other failure domain labels) across which to configure the cluster.
  * `failureDomainLabel`: The label that is expected on each node where the cluster is expected to be deployed. The labels must be found
    in the list of well-known [topology labels](#osd-topology).
  * `subFailureDomain`: With a zone, the data replicas must be spread across OSDs in the subFailureDomain. The default is `host`.
  * `zones`: The failure domain names where the Mons and OSDs are expected to be deployed. There must be **three zones** specified in the list.
    This element is always named `zone` even if a non-default `failureDomainLabel` is specified. The elements have two values:
    * `name`: The name of the zone, which is the value of the domain label.
    * `arbiter`: Whether the zone is expected to be the arbiter zone which only runs a single mon. Exactly one zone must be labeled `true`.
      The two zones that are not the arbiter zone are expected to have OSDs deployed.

If these settings are changed in the CRD the operator will update the number of mons during a periodic check of the mon health, which by default is every 45 seconds.

To change the defaults that the operator uses to determine the mon health and whether to failover a mon, refer to the [health settings](#health-settings). The intervals should be small enough that you have confidence the mons will maintain quorum, while also being long enough to ignore network blips where mons are failed over too often.

### Mgr Settings

You can use the cluster CR to enable or disable any manager module. This can be configured like so:

```yaml
mgr:
  modules:
  - name: <name of the module>
    enabled: true
```

Some modules will have special configuration to ensure the module is fully functional after being enabled. Specifically:

* `pg_autoscaler`: Rook will configure all new pools with PG autoscaling by setting: `osd_pool_default_pg_autoscale_mode = on`

### Network Configuration Settings

If not specified, the default SDN will be used.
Configure the network that will be enabled for the cluster and services.

* `provider`: Specifies the network provider that will be used to connect the network interface. You can choose between `host`, and `multus`.
* `selectors`: List the network selector(s) that will be used associated by a key.

> **NOTE:** Changing networking configuration after a Ceph cluster has been deployed is NOT
> supported and will result in a non-functioning cluster.

#### Host Networking

To use host networking, set `provider: host`.

#### Multus (EXPERIMENTAL)

Rook has experimental support for Multus.
Currently there is an [open issue](https://github.com/ceph/ceph-csi/issues/1323) in ceph-csi which explains the csi-rbdPlugin issue while using multus network.

The selector keys are required to be `public` and `cluster` where each represent:

* `public`: client communications with the cluster (reads/writes)
* `cluster`: internal Ceph replication network

If you want to learn more, please read
* [Ceph Networking reference](https://docs.ceph.com/docs/master/rados/configuration/network-config-ref/).
* [Multus documentation](https://intel.github.io/multus-cni/doc/how-to-use.html)

Based on the configuration, the operator will do the following:

  1. if only the `public` selector is specified both communication and replication will happen on that network
  2. if both `public` and `cluster` selectors are specified the first one will run the communication network and the second the replication network

In order to work, each selector value must match a `NetworkAttachmentDefinition` object name in Multus.
For example, you can do:

* `public`: "rook-ceph/my-public-storage-network"
* `cluster`: "rook-ceph/my-replication-storage-network"

For `multus` network provider, an already working cluster with Multus networking is required. Network attachment definition that later will be attached to the cluster needs to be created before the Cluster CRD.
The Network attachment definitions should be using whereabouts cni.
If Rook cannot find the provided Network attachment definition it will fail running the Ceph OSD pods.
You can add the Multus network attachment selection annotation selecting the created network attachment definition on `selectors`.

A valid NetworkAttachmentDefinition will look like following:

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: rook-public-nw
spec:
  config: '{
      "cniVersion": "0.3.0",
      "name": "public-nad",
      "type": "macvlan",
      "master": "ens5",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "192.168.1.0/24"
      }
    }'
```

* Ensure that `master` matches the network interface of the host that you want to use.
* The NAD should be referenced along with the namespace in which it is present like `public: <namespace>/<name of NAD>`.
  e.g., the network attachment definition are in `rook-multus` namespace:

```yaml
  public: rook-multus/rook-public-nw
  cluster: rook-multus/rook-cluster-nw
```

This is required in order to use the NAD across namespaces.
* In Openshift, to use the NetworkAttachmentDefinition across namespaces, the NAD must be deployed in the default namespace and it can be referenced as `default/myNAD` where `default` is the namespace and `myNAD` is the network attachment definition.

#### IPFamily

Provide single-stack IPv4 or IPv6 protocol to assign corresponding addresses to pods and services. This field is optional. Possible inputs are IPv6 and IPv4. Empty value will be treated as IPv4. Kubernetes version should be at least v1.13 to run IPv6. Dual-stack is not supported by ceph.

### Node Settings

In addition to the cluster level settings specified above, each individual node can also specify configuration to override the cluster level settings and defaults.
If a node does not specify any configuration then it will inherit the cluster level settings.

* `name`: The name of the node, which should match its `kubernetes.io/hostname` label.
* `config`: Config settings applied to all OSDs on the node unless overridden by `devices`. See the [config settings](#osd-configuration-settings) below.
* [storage selection settings](#storage-selection-settings)

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

* `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by OSDs. **Not recommended** unless you have a very controlled environment where you will not risk formatting of devices with existing data. When `true`, all devices/partitions will be used. Is overridden by `deviceFilter` if specified.
* `deviceFilter`: A regular expression for short kernel names of devices (e.g. `sda`) that allows selection of devices to be consumed by OSDs.  If individual devices have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
  * `sdb`: Only selects the `sdb` device if found
  * `^sd.`: Selects all devices starting with `sd`
  * `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
  * `^s`: Selects all devices that start with `s`
  * `^[^r]`: Selects all devices that do *not* start with `r`
* `devicePathFilter`: A regular expression for device paths (e.g. `/dev/disk/by-path/pci-0:1:2:3-scsi-1`) that allows selection of devices to be consumed by OSDs.  If individual devices or `deviceFilter` have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
  * `^/dev/sd.`: Selects all devices starting with `sd`
  * `^/dev/disk/by-path/pci-.*`: Selects all devices which are connected to PCI bus
* `devices`: A list of individual device names belonging to this node to include in the storage cluster.
  * `name`: The name of the device (e.g., `sda`), or full udev path (e.g. `/dev/disk/by-id/ata-ST4000DM004-XXXX` - this will not change after reboots).
  * `config`: Device-specific config settings. See the [config settings](#osd-configuration-settings) below
* `storageClassDeviceSets`: Explained in [Storage Class Device Sets](#storage-class-device-sets)

### Storage Class Device Sets

The following are the settings for Storage Class Device Sets which can be configured to create OSDs that are backed by block mode PVs.

* `name`: A name for the set.
* `count`: The number of devices in the set.
* `resources`: The CPU and RAM requests/limits for the devices. (Optional)
* `placement`: The placement criteria for the devices. (Optional) Default is no placement criteria.

  The syntax is the same as for [other placement configuration](#placement-configuration-settings). It supports `nodeAffinity`, `podAffinity`, `podAntiAffinity` and `tolerations` keys.

  It is recommended to configure the placement such that the OSDs will be as evenly spread across nodes as possible. At a minimum, anti-affinity should be added so at least one OSD will be placed on each available nodes.

  However, if there are more OSDs than nodes, this anti-affinity will not be effective. Another placement scheme to consider is to add labels to the nodes in such a way that the OSDs can be grouped on those nodes, create multiple storageClassDeviceSets, and add node affinity to each of the device sets that will place the OSDs in those sets of nodes.

  Rook will automatically add required nodeAffinity to the OSD daemons to match the topology labels that are found
  on the nodes where the OSD prepare jobs ran. To ensure data durability, the OSDs are required to run in the same
  topology that the Ceph CRUSH map expects. For example, if the nodes are labeled with rack topology labels, the
  OSDs will be constrained to a certain rack. Without the topology labels, Rook will not constrain the OSDs beyond
  what is required by the PVs, for example to run in the zone where provisioned. See the [OSD Topology](#osd-topology)
  section for the related labels.

* `preparePlacement`: The placement criteria for the preparation of the OSD devices. Creating OSDs is a two-step process and the prepare job may require different placement than the OSD daemons. If the `preparePlacement` is not specified, the `placement` will instead be applied for consistent placement for the OSD prepare jobs and OSD deployments. The `preparePlacement` is only useful for `portable` OSDs in the device sets. OSDs that are not portable will be tied to the host where the OSD prepare job initially runs.
  * For example, provisioning may require topology spread constraints across zones, but the OSD daemons may require constraints across hosts within the zones.
* `portable`: If `true`, the OSDs will be allowed to move between nodes during failover. This requires a storage class that supports portability (e.g. `aws-ebs`, but not the local storage provisioner). If `false`, the OSDs will be assigned to a node permanently. Rook will configure Ceph's CRUSH map to support the portability.
* `tuneDeviceClass`: For example, Ceph cannot detect AWS volumes as HDDs from the storage class "gp2", so you can improve Ceph performance by setting this to true.
* `tuneFastDeviceClass`: For example, Ceph cannot detect Azure disks as SSDs from the storage class "managed-premium", so you can improve Ceph performance by setting this to true..
* `volumeClaimTemplates`: A list of PVC templates to use for provisioning the underlying storage devices.
  * `resources.requests.storage`: The desired capacity for the underlying storage devices.
  * `storageClassName`: The StorageClass to provision PVCs from. Default would be to use the cluster-default StorageClass. This StorageClass should provide a raw block device, multipath device, or logical volume. Other types are not supported. If you want to use logical volume, please see [known issue of OSD on LV-backed PVC](ceph-common-issues.md#lvm-metadata-can-be-corrupted-with-osd-on-lv-backed-pvc)
  * `volumeMode`: The volume mode to be set for the PVC. Which should be Block
  * `accessModes`: The access mode for the PVC to be bound by OSD.
* `schedulerName`: Scheduler name for OSD pod placement. (Optional)
* `encrypted`: whether to encrypt all the OSDs in a given storageClassDeviceSet

### OSD Configuration Settings

The following storage selection settings are specific to Ceph and do not apply to other backends. All variables are key-value pairs represented as strings.

* `metadataDevice`: Name of a device to use for the metadata of OSDs on each node.  Performance can be improved by using a low latency device (such as SSD or NVMe) as the metadata device, while other spinning platter (HDD) devices on a node are used to store data. Provisioning will fail if the user specifies a `metadataDevice` but that device is not used as a metadata device by Ceph. Notably, `ceph-volume` will not use a device of the same device class (HDD, SSD, NVMe) as OSD devices for metadata, resulting in this failure.
* `storeType`: `bluestore`, the underlying storage format to use for each OSD. The default is set dynamically to `bluestore` for devices and is the only supported format at this point.
* `databaseSizeMB`:  The size in MB of a bluestore database. Include quotes around the size.
* `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL). Include quotes around the size.
* `deviceClass`: The [CRUSH device class](https://ceph.io/community/new-luminous-crush-device-classes/) to use for this selection of storage devices. (By default, if a device's class has not already been set, OSDs will automatically set a device's class to either `hdd`, `ssd`, or `nvme`  based on the hardware properties exposed by the Linux kernel.) These storage classes can then be used to select the devices backing a storage pool by specifying them as the value of [the pool spec's `deviceClass` field](ceph-pool-crd.md#spec).
* `osdsPerDevice`**: The number of OSDs to create on each device. High performance devices such as NVMe can handle running multiple OSDs. If desired, this can be overridden for each node and each device.
* `encryptedDevice`**: Encrypt OSD volumes using dmcrypt ("true" or "false"). By default this option is disabled. See [encryption](http://docs.ceph.com/docs/nautilus/ceph-volume/lvm/encryption/) for more information on encryption in Ceph.
* `crushRoot`: The value of the `root` CRUSH map label. The default is `default`. Generally, you should not need to change this. However, if any of your topology labels may have the value `default`, you need to change `crushRoot` to avoid conflicts, since CRUSH map values need to be unique.

**NOTE**: Depending on the Ceph image running in your cluster, OSDs will be configured differently. Newer images will configure OSDs with `ceph-volume`, which provides support for `osdsPerDevice`, `encryptedDevice`, as well as other features that will be exposed in future Rook releases. OSDs created prior to Rook v0.9 or with older images of Luminous and Mimic are not created with `ceph-volume` and thus would not support the same features. For `ceph-volume`, the following images are supported:

* Luminous 12.2.10 or newer
* Mimic 13.2.3 or newer
* Nautilus

### Annotations and Labels

Annotations and Labels can be specified so that the Rook components will have those annotations / labels added to them.

You can set annotations / labels for Rook components for the list of key value pairs:

* `all`: Set annotations / labels for all components
* `mgr`: Set annotations / labels for MGRs
* `mon`: Set annotations / labels for mons
* `osd`: Set annotations / labels for OSDs
* `prepareosd`: Set annotations / labels for OSD Prepare Jobs
When other keys are set, `all` will be merged together with the specific component.

### Placement Configuration Settings

Placement configuration for the cluster services. It includes the following keys: `mgr`, `mon`, `arbiter`, `osd`, `cleanup`, and `all`.
Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

In stretch clusters, if the `arbiter` placement is specified, that placement will only be applied to the arbiter.
Neither will the `arbiter` placement be merged with the `all` placement to allow the arbiter to be fully independent of other daemon placement.
The remaining mons will still use the `mon` and/or `all` sections.


**NOTE:** Placement of OSD pods is controlled using the [Storage Class Device Set](#storage-class-device-sets), not the general `placement` configuration.

A Placement configuration is specified (according to the kubernetes PodSpec) as:

* `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity-beta-feature)
* `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
* `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
* `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)
* `topologySpreadConstraints`: kubernetes [TopologySpreadConstraints](https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/)

If you use `labelSelector` for `osd` pods, you must write two rules both for `rook-ceph-osd` and `rook-ceph-osd-prepare` like [the example configuration](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/cluster-on-pvc.yaml#L68). It comes from the design that there are these two pods for an OSD. For more detail, see the [osd design doc](https://github.com/rook/rook/blob/master/design/ceph/dedicated-osd-pod.md) and [the related issue](https://github.com/rook/rook/issues/4582).

The Rook Ceph operator creates a Job called `rook-ceph-detect-version` to detect the full Ceph version used by the given `cephVersion.image`. The placement from the `mon` section is used for the Job except for the `PodAntiAffinity` field.

### Cluster-wide Resources Configuration Settings

Resources should be specified so that the Rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep Rook components running when for example a node runs out of memory and the Rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for Rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:

* `mon`: Set resource requests/limits for mons
* `osd`: Set resource requests/limits for OSDs
* `mgr`: Set resource requests/limits for MGRs
* `mgr-sidecar`: Set resource requests/limits for the MGR sidecar, which is only created when `mgr.count: 2`.
  The sidecar requires very few resources since it only executes every 15 seconds to query Ceph for the active
  mgr and update the mgr services if the active mgr changed.
* `prepareosd`: Set resource requests/limits for OSD prepare job
* `crashcollector`: Set resource requests/limits for crash. This pod runs wherever there is a Ceph pod running.
It scrapes for Ceph daemon core dumps and sends them to the Ceph manager crash module so that core dumps are centralized and can be easily listed/accessed.
You can read more about the [Ceph Crash module](https://docs.ceph.com/docs/master/mgr/crash/).
* `logcollector`: Set resource requests/limits for the log collector. When enabled, this container runs as side-car to each Ceph daemons.
* `cleanup`: Set resource requests/limits for cleanup job, responsible for wiping cluster's data after uninstall

In order to provide the best possible experience running Ceph in containers, Rook internally recommends minimum memory limits if resource limits are passed.
If a user configures a limit or request value that is too low, Rook will still run the pod(s) and print a warning to the operator log.

* `mon`: 1024MB
* `mgr`: 512MB
* `osd`: 2048MB
* `mds`: 4096MB
* `prepareosd`: 50MB
* `crashcollector`: 60MB
* `mgr-sidecar`: 100MB limit, 40MB requests

### Resource Requirements/Limits

For more information on resource requests/limits see the official Kubernetes documentation: [Kubernetes - Managing Compute Resources for Containers](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container)

* `requests`: Requests for cpu or memory.
  * `cpu`: Request for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  * `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).
* `limits`: Limits for cpu or memory.
  * `cpu`: Limit for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
  * `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).

### Priority Class Names Configuration Settings

Priority class names can be specified so that the Rook components will have those priority class names added to them.

You can set priority class names for Rook components for the list of key value pairs:

* `all`: Set priority class names for MGRs, Mons, OSDs.
* `mgr`: Set priority class names for MGRs.
* `mon`: Set priority class names for Mons.
* `osd`: Set priority class names for OSDs.

The specific component keys will act as overrides to `all`.

### Health settings

Rook-Ceph will monitor the state of the CephCluster on various components by default.
The following CRD settings are available:

* `healthCheck`: main ceph cluster health monitoring section

Currently three health checks are implemented:

* `mon`: health check on the ceph monitors, basically check whether monitors are members of the quorum. If after a certain timeout a given monitor has not joined the quorum back it will be failed over and replace by a new monitor.
* `osd`: health check on the ceph osds
* `status`: ceph health status check, periodically check the Ceph health state and reflects it in the CephCluster CR status field.

The liveness probe of each daemon can also be controlled via `livenessProbe`, the setting is valid for `mon`, `mgr` and `osd`.
Here is a complete example for both `daemonHealth` and `livenessProbe`:

```yaml
healthCheck:
  daemonHealth:
    mon:
      disabled: false
      interval: 45s
      timeout: 600s
    osd:
      disabled: false
      interval: 60s
    status:
      disabled: false
  livenessProbe:
    mon:
      disabled: false
    mgr:
      disabled: false
    osd:
      disabled: false
```

The probe itself can also be overridden, refer to the [Kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-a-liveness-command).

For example, you could change the `mgr` probe by applying:

```yaml
healthCheck:
  livenessProbe:
    mgr:
      disabled: false
      probe:
        httpGet:
          path: /
          port: 9283
        initialDelaySeconds: 3
        periodSeconds: 3
```

Changing the liveness probe is an advanced operation and should rarely be necessary. If you want to change these settings then modify the desired settings.

## Status

The operator is regularly configuring and checking the health of the cluster. The results of the configuration
and health checks can be seen in the `status` section of the CephCluster CR.

```
kubectl -n rook-ceph get CephCluster -o yaml
```

```yaml
  ...
  status:
    ceph:
      health: HEALTH_OK
      lastChecked: "2021-03-02T21:22:11Z"
      capacity:
        bytesAvailable: 22530293760
        bytesTotal: 25757220864
        bytesUsed: 3226927104
        lastUpdated: "2021-03-02T21:22:11Z"
    message: Cluster created successfully
    phase: Ready
    state: Created
    storage:
      deviceClasses:
      - name: hdd
    version:
      image: ceph/ceph:v15
      version: 15.2.9-0
    conditions:
    - lastHeartbeatTime: "2021-03-02T21:22:11Z"
      lastTransitionTime: "2021-03-02T21:21:09Z"
      message: Cluster created successfully
      reason: ClusterCreated
      status: "True"
      type: Ready
```

### Ceph Status

Ceph is constantly monitoring the health of the data plane and reporting back if there are
any warnings or errors. If everything is healthy from Ceph's perspective, you will see
`HEALTH_OK`.

If Ceph reports any warnings or errors, the details will be printed to the status.
If further troubleshooting is needed to resolve these issues, the toolbox will likely
be needed where you can run `ceph` commands to find more details.

The `capacity` of the cluster is reported, including bytes available, total, and used.
The available space will be less that you may expect due to overhead in the OSDs.

### Conditions

The `conditions` represent the status of the Rook operator.
- If the cluster is fully configured and the operator is stable, the
  `Ready` condition is raised with `ClusterCreated` reason and no other conditions. The cluster
  will remain in the `Ready` condition after the first successful configuration since it
  is expected the storage is consumable from this point on. If there are issues preventing
  the storage layer from working, they are expected to show as Ceph health errors.
- If the cluster is externally connected successfully, the `Ready` condition will have the reason `ClusterConnected`.
- If the operator is currently being configured or the operator is checking for update,
  there will be a `Progressing` condition.
- If there was a failure, the condition(s) status will be `false` and the `message` will
  give a summary of the error. See the operator log for more details.

### Other Status

There are several other properties for the overall status including:
- `message`, `phase`, and `state`: A summary of the overall current state of the cluster, which
  is somewhat duplicated from the conditions for backward compatibility.
- `storage.deviceClasses`: The names of the types of storage devices that Ceph discovered
  in the cluster. These types will be `ssd` or `hdd` unless they have been overridden
  with the `crushDeviceClass` in the `storageClassDeviceSets`.
- `version`: The version of the Ceph image currently deployed.

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
    image: ceph/ceph:v15.2.9
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
    deviceFilter:
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
    image: ceph/ceph:v15.2.9
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
        storeType: bluestore
    - name: "172.17.4.301"
      deviceFilter: "^sd."
```

### Node Affinity

To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage-node' and
tolerate taints with a key of 'storage-node'.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v15.2.9
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
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

> **WARNING**: Before setting resource requests/limits, please take a look at the Ceph documentation for recommendations for each component: [Ceph - Hardware Recommendations](http://docs.ceph.com/docs/master/start/hardware-recommendations/).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v15.2.9
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
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

### OSD Topology

The topology of the cluster is important in production environments where you want your data spread across failure domains. The topology
can be controlled by adding labels to the nodes. When the labels are found on a node at first OSD deployment, Rook will add them to
the desired level in the [CRUSH map](https://docs.ceph.com/en/latest/rados/operations/crush-map/).

The complete list of labels in hierarchy order from highest to lowest is:

```text
topology.kubernetes.io/region
topology.kubernetes.io/zone
topology.rook.io/datacenter
topology.rook.io/room
topology.rook.io/pod
topology.rook.io/pdu
topology.rook.io/row
topology.rook.io/rack
topology.rook.io/chassis
```

For example, if the following labels were added to a node:

```console
kubectl label node mynode topology.kubernetes.io/zone=zone1
kubectl label node mynode topology.rook.io/rack=zone1-rack1
```

> For versions previous to K8s 1.17, use the topology key: failure-domain.beta.kubernetes.io/zone or region

These labels would result in the following hierarchy for OSDs on that node (this command can be run in the Rook toolbox):

```console
ceph osd tree
```

>```
>ID CLASS WEIGHT  TYPE NAME                 STATUS REWEIGHT PRI-AFF
>-1       0.01358 root default
>-5       0.01358     zone zone1
>-4       0.01358         rack rack1
>-3       0.01358             host mynode
>0   hdd 0.00679                 osd.0         up  1.00000 1.00000
>1   hdd 0.00679                 osd.1         up  1.00000 1.00000
>```

Ceph requires unique names at every level in the hierarchy (CRUSH map). For example, you cannot have two racks
with the same name that are in different zones. Racks in different zones must be named uniquely.

Note that the `host` is added automatically to the hierarchy by Rook. The host cannot be specified with a topology label.
All topology labels are optional.

> **HINT** When setting the node labels prior to `CephCluster` creation, these settings take immediate effect. However, applying this to an already deployed `CephCluster` requires removing each node from the cluster first and then re-adding it with new configuration to take effect. Do this node by node to keep your data safe! Check the result with `ceph osd tree` from the [Rook Toolbox](ceph-toolbox.md). The OSD tree should display the hierarchy for the nodes that already have been re-added.

To utilize the `failureDomain` based on the node labels, specify the corresponding option in the [CephBlockPool](ceph-pool-crd.md)

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: rack  # this matches the topology labels on nodes
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
    image: ceph/ceph:v15.2.9
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
    image: ceph/ceph:v15.2.9
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
                topologyKey: "topology.kubernetes.io/zone"
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

### Dedicated metadata and wal device for OSD on PVC

In the simplest case, Ceph OSD BlueStore consumes a single (primary) storage device.
BlueStore is the engine used by the OSD to store data.

The storage device is normally used as a whole, occupying the full device that is managed directly by BlueStore.
It is also possible to deploy BlueStore across additional devices such as a DB device.
This device can be used for storing BlueStore’s internal metadata.
BlueStore (or rather, the embedded RocksDB) will put as much metadata as it can on the DB device to improve performance.
If the DB device fills up, metadata will spill back onto the primary device (where it would have been otherwise).
Again, it is only helpful to provision a DB device if it is faster than the primary device.

You can have multiple `volumeClaimTemplates` where each might either represent a device or a metadata device.
So just taking the `storage` section this will give something like:

```yaml
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
          storageClassName: gp2
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
      - metadata:
          name: metadata
        spec:
          resources:
            requests:
              # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
              storage: 5Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
          storageClassName: io1
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
```

> **NOTE**: Note that Rook only supports three naming convention for a given template:

* "data": represents the main OSD block device, where your data is being stored.
* "metadata": represents the metadata (including block.db and block.wal) device used to store the Ceph Bluestore database for an OSD.
* "wal": represents the block.wal device used to store the Ceph Bluestore database for an OSD. If this device is set, "metadata" device will refer specifically to block.db device.
It is recommended to use a faster storage class for the metadata or wal device, with a slower device for the data.
Otherwise, having a separate metadata device will not improve the performance.

The bluestore partition has the following reference combinations supported by the ceph-volume utility:

* A single "data" device.

  ```yaml
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
            storageClassName: gp2
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
  ```

* A "data" device and a "metadata" device.

  ```yaml
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
            storageClassName: gp2
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
        - metadata:
            name: metadata
          spec:
            resources:
              requests:
                # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                storage: 5Gi
            # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
            storageClassName: io1
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
  ```

* A "data" device and a "wal" device.
A WAL device can be used for BlueStore’s internal journal or write-ahead log (block.wal), it is only useful to use a WAL device if the device is faster than the primary device (data device).
There is no separate "metadata" device in this case, the data of main OSD block and block.db located in "data" device.

  ```yaml
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
            storageClassName: gp2
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
        - metadata:
            name: wal
          spec:
            resources:
              requests:
                # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                storage: 5Gi
            # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
            storageClassName: io1
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
  ```

* A "data" device, a "metadata" device and a "wal" device.

  ```yaml
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
            storageClassName: gp2
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
        - metadata:
            name: metadata
          spec:
            resources:
              requests:
                # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                storage: 5Gi
            # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
            storageClassName: io1
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
        - metadata:
            name: wal
          spec:
            resources:
              requests:
                # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                storage: 5Gi
            # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
            storageClassName: io1
            volumeMode: Block
            accessModes:
              - ReadWriteOnce
  ```

To determine the size of the metadata block follow the [official Ceph sizing guide](https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing).

With the present configuration, each OSD will have its main block allocated a 10GB device as well a 5GB device to act as a bluestore database.

### External cluster

**The minimum supported Ceph version for the External Cluster is Luminous 12.2.x.**

The features available from the external cluster will vary depending on the version of Ceph. The following table shows the minimum version of Ceph for some of the features:

| FEATURE                                      | CEPH VERSION |
| -------------------------------------------- | ------------ |
| Dynamic provisioning RBD                     | 12.2.X       |
| Configure extra CRDs (object, file, nfs)[^1] | 13.2.3       |
| Dynamic provisioning CephFS                  | 14.2.3       |

[^1]: Configure an object store, shared filesystem, or NFS resources in the local cluster to connect to the external Ceph cluster

#### Pre-requisites

In order to configure an external Ceph cluster with Rook, we need to inject some information in order to connect to that cluster.
You can use the `cluster/examples/kubernetes/ceph/import-external-cluster.sh` script to achieve that.
The script will look for the following populated environment variables:

* `NAMESPACE`: the namespace where the configmap and secrets should be injected
* `ROOK_EXTERNAL_FSID`: the fsid of the external Ceph cluster, it can be retrieved via the `ceph fsid` command
* `ROOK_EXTERNAL_CEPH_MON_DATA`: is a common-separated list of running monitors IP address along with their ports, e.g: `a=172.17.0.4:6789,b=172.17.0.5:6789,c=172.17.0.6:6789`. You don't need to specify all the monitors, you can simply pass one and the Operator will discover the rest. The name of the monitor is the name that appears in the `ceph status` output.

Now, we need to give Rook a key to connect to the cluster in order to perform various operations such as health cluster check, CSI keys management etc...
It is recommended to generate keys with minimal access so the admin key does not need to be used by the external cluster.
In this case, the admin key is only needed to generate the keys that will be used by the external cluster.
But if the admin key is to be used by the external cluster, set the following variable:

* `ROOK_EXTERNAL_ADMIN_SECRET`: **OPTIONAL:** the external Ceph cluster admin secret key, it can be retrieved via the `ceph auth get-key client.admin` command.

> **WARNING**: If you plan to create CRs (pool, rgw, mds, nfs) in the external cluster, you **MUST** inject the client.admin keyring as well as injecting `cluster-external-management.yaml`

**Example**:

```console
export NAMESPACE=rook-ceph-external
export ROOK_EXTERNAL_FSID=3240b4aa-ddbc-42ee-98ba-4ea7b2a61514
export ROOK_EXTERNAL_CEPH_MON_DATA=a=172.17.0.4:6789
export ROOK_EXTERNAL_ADMIN_SECRET=AQC6Ylxdja+NDBAAB7qy9MEAr4VLLq4dCIvxtg==
```

If the Ceph admin key is not provided, the following script needs to be executed on a machine that can connect to the Ceph cluster using the Ceph admin key.
On that machine, run `cluster/examples/kubernetes/ceph/create-external-cluster-resources.sh`.
The script will automatically create users and keys with the lowest possible privileges and populate the necessary environment variables for `cluster/examples/kubernetes/ceph/import-external-cluster.sh` to work correctly.

Finally, you can simply execute the script like this from a machine that has access to your Kubernetes cluster:

```console
bash cluster/examples/kubernetes/ceph/import-external-cluster.sh
```

#### CephCluster example (consumer)

Assuming the above section has successfully completed, here is a CR example:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph-external
  namespace: rook-ceph-external
spec:
  external:
    enable: true
  crashCollector:
    disable: true
  # optionally, the ceph-mgr IP address can be pass to gather metric from the prometheus exporter
  #monitoring:
    #enabled: true
    #rulesNamespace: rook-ceph
    #externalMgrEndpoints:
      #- ip: 192.168.39.182
    #externalMgrPrometheusPort: 9283
```

Choose the namespace carefully, if you have an existing cluster managed by Rook, you have likely already injected `common.yaml`.
Additionally, you now need to inject `common-external.yaml` too.

You can now create it like this:

```console
kubectl create -f cluster/examples/kubernetes/ceph/cluster-external.yaml
```

If the previous section has not been completed, the Rook Operator will still acknowledge the CR creation but will wait forever to receive connection information.

> **WARNING**: If no cluster is managed by the current Rook Operator, you need to inject `common.yaml`, then modify `cluster-external.yaml` and specify `rook-ceph` as `namespace`.

If this is successful you will see the CepCluster status as connected.

```console
kubectl get CephCluster -n rook-ceph-external
```

>```
>NAME                 DATADIRHOSTPATH   MONCOUNT   AGE    STATE       HEALTH
>rook-ceph-external   /var/lib/rook                162m   Connected   HEALTH_OK
>```

Before you create a StorageClass with this cluster you will need to create a Pool in your external Ceph Cluster.

#### Example StorageClass based on external Ceph Pool

In Ceph Cluster let us list the pools available:

```console
rados df
```

>```
>POOL_NAME     USED OBJECTS CLONES COPIES MISSING_ON_PRIMARY UNFOUND DEGRADED RD_OPS  RD WR_OPS  WR USED COMPR UNDER COMPR
>replicated_2g  0 B       0      0      0                  0       0        0      0 0 B      0 0 B        0 B         0 B
> ```

Here is an example StorageClass configuration that uses the `replicated_2g` pool from the external cluster:

```console
cat << EOF | kubectl apply -f -
```

>```
>apiVersion: storage.k8s.io/v1
>kind: StorageClass
>metadata:
>   name: rook-ceph-block-ext
># Change "rook-ceph" provisioner prefix to match the operator namespace if needed
>provisioner: rook-ceph.rbd.csi.ceph.com
>parameters:
>    # clusterID is the namespace where the rook cluster is running
>    clusterID: rook-ceph-external
>    # Ceph pool into which the RBD image shall be created
>    pool: replicated_2g
>
>    # RBD image format. Defaults to "2".
>    imageFormat: "2"
>
>    # RBD image features. Available for imageFormat: "2". CSI RBD currently supports only `layering` feature.
>    imageFeatures: layering
>
>    # The secrets contain Ceph admin credentials.
>    csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
>    csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph-external
>    csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
>    csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph-external
>    csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
>    csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph-external
>
>    # Specify the filesystem type of the volume. If not specified, csi-provisioner
>    # will set default as `ext4`. Note that `xfs` is not recommended due to potential deadlock
>    # in hyperconverged settings where the volume is mounted on the same node as the osds.
>    csi.storage.k8s.io/fstype: ext4
>
># Delete the rbd volume when a PVC is deleted
>reclaimPolicy: Delete
>allowVolumeExpansion: true
>EOF
>```

You can now create a persistent volume based on this StorageClass.

#### CephCluster example (management)

The following CephCluster CR represents a cluster that will perform management tasks on the external cluster.
It will not only act as a consumer but will also allow the deployment of other CRDs such as CephFilesystem or CephObjectStore.
As mentioned above, you would need to inject the admin keyring for that.

The corresponding YAML example:

```yaml
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
    image: ceph/ceph:v15.2.9 # Should match external cluster version
```

### Cleanup policy

Rook has the ability to cleanup resources and data that were deployed when a CephCluster is removed.
The policy settings indicate which data should be forcibly deleted and in what way the data should be wiped.
The `cleanupPolicy` has several fields:

* `confirmation`: Only an empty string and `yes-really-destroy-data` are valid values for this field.
  If this setting is empty, the cleanupPolicy settings will be ignored and Rook will not cleanup any resources during cluster removal.
  To reinstall the cluster, the admin would then be required to follow the [cleanup guide](ceph-teardown.md) to delete the data on hosts.
  If this setting is `yes-really-destroy-data`, the operator will automatically delete the data on hosts.
  Because this cleanup policy is destructive, after the confirmation is set to `yes-really-destroy-data`
  Rook will stop configuring the cluster as if the cluster is about to be destroyed.
* `sanitizeDisks`: sanitizeDisks represents advanced settings that can be used to delete data on drives.
  * `method`: indicates if the entire disk should be sanitized or simply ceph's metadata. Possible choices are 'quick' (default) or 'complete'
  * `dataSource`: indicate where to get random bytes from to write on the disk. Possible choices are 'zero' (default) or 'random'.
  Using random sources will consume entropy from the system and will take much more time then the zero source
  * `iteration`: overwrite N times instead of the default (1). Takes an integer value
* `allowUninstallWithVolumes`: If set to true, then the cephCluster deletion doesn't wait for the PVCs to be deleted. Default is false.

To automate activation of the cleanup, you can use the following command. **WARNING: DATA WILL BE PERMANENTLY DELETED**:

```console
kubectl -n rook-ceph patch cephcluster rook-ceph --type merge -p '{"spec":{"cleanupPolicy":{"confirmation":"yes-really-destroy-data"}}}'
```

Nothing will happen until the deletion of the CR is requested, so this can still be reverted.
However, all new configuration by the operator will be blocked with this cleanup policy enabled.

Rook waits for the deletion of PVs provisioned using the cephCluster before proceeding to delete the cephCluster. To force deletion of the cephCluster without waiting for the PVs to be deleted,  you can set the allowUninstallWithVolumes to true under spec.CleanupPolicy.

### Security

Rook has the ability to encrypt OSDs of clusters running on PVC via the flag (`encrypted: true`) in your `storageClassDeviceSets` [template](#pvc-based-cluster).
By default, the Key Encryption Keys (also known as Data Encryption Keys) are stored in a Kubernetes Secret.

However, if a Key Management System exists Rook is capable of using it. HashiCorp Vault is the only KMS currently supported by Rook.
Please refer to the next section.

Ceph RGW supports encryption via KMS using HashiCorp Vault. If the below settings are defined, then RGW establish a connection between Vault
and whenever S3 client sends a request with Server Side Encryption, it encrypts that using the key specified by the client.
For more details w.r.t RGW, please refer [Ceph Vault documentation](https://docs.ceph.com/en/latest/radosgw/vault/)

The `security` section contains settings related to encryption of the cluster.

* `security`:
  * `kms`: Key Management System settings
    * `connectionDetails`: the list of parameters representing kms connection details
    * `tokenSecretName`: the name of the Kubernetes Secret containing the kms authentication token

#### Vault KMS

In order for Rook to connect to Vault, you must configure the following in your `CephCluster` template:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_BACKEND_PATH: rook
      VAULT_SECRET_ENGINE: kv
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```

Note: Rook supports **all** the Vault [environment variables](https://www.vaultproject.io/docs/commands#environment-variables).

The Kubernetes Secret `rook-vault-token` should contain:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rook-vault-token
  namespace: rook-ceph
data:
  token: <TOKEN> # base64 of a token to connect to Vault, for example: cy5GWXpsbzAyY2duVGVoRjhkWG5Bb3EyWjkK
```

As part of the token, here is an example of a policy that can be used:

```hcl
path "rook/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
path "sys/mounts" {
capabilities = ["read"]
}
```

You can write the policy like so and then create a token:

```console
vault policy write rook /tmp/rook.hcl
vault token create -policy=rook
```
>```
>Key                  Value
>---                  -----
>token                s.FYzlo02cgnTehF8dXnAoq2Z9
>token_accessor       oMo7sAXQKbYtxU4HtO8k3pko
>token_duration       768h
>token_renewable      true
>token_policies       ["default" "rook"]
>identity_policies    []
>policies             ["default" "rook"]
>```

In this example the backend path named `rook` is used it must be enabled in Vault with the following:

```console
vault secrets enable -path=rook kv
```

If a different path is used, the `VAULT_BACKEND_PATH` key in `connectionDetails` must be changed.

Currently the token-based authentication is the only supported method.
Later Rook is planning on supporting the [Vault Kubernetes native authentication](https://www.vaultproject.io/docs/auth/kubernetes).

##### TLS configuration

This is an advanced but recommended configuration for production deployments, in this case the `vault-connection-details` will look like:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_CACERT: <name of the k8s secret containing the PEM-encoded CA certificate>
      VAULT_CLIENT_CERT: <name of the k8s secret containing the PEM-encoded client certificate>
      VAULT_CLIENT_KEY: <name of the k8s secret containing the PEM-encoded private key>
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```

Each secret keys are expected to be:

* VAULT_CACERT: `cert`
* VAULT_CLIENT_CERT: `cert`
* VAULT_CLIENT_KEY: `key`

For instance `VAULT_CACERT` Secret named `vault-tls-ca-certificate` will look like:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vault-tls-ca-certificate
  namespace: rook-ceph
data:
  cert: <PEM base64 encoded CA certificate>
```

Note: if you are using self-signed certificates (not known/approved by a proper CA) you must pass `VAULT_SKIP_VERIFY: true`.
Communications will remain encrypted but the validity of the certificate will not be verified.

For RGW, please note the following:

* `VAULT_SECRET_ENGINE` option is specifically for RGW to mention about the secret engine which can be used, currently supports two: [kv](https://www.vaultproject.io/docs/secrets/kv) and [transit](https://www.vaultproject.io/docs/secrets/transit).
* The Storage administrator needs to create a secret in the Vault server so that S3 clients use that key for encryption
```console
# kv engine
vault kv put rook/mybucketkey key=$(openssl rand -base64 32)

# transit engine
vault write -f transit/keys/mybucketkey exportable=true
```
* TLS authentication with custom certs between Vault and RGW are yet to support.
