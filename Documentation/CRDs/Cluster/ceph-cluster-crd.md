---
title: CephCluster CRD
---

Rook allows creation and customization of storage clusters through the custom resource definitions (CRDs).
There are primarily four different modes in which to create your cluster.

1. [Host Storage Cluster](host-cluster.md): Consume storage from host paths and raw devices
2. [PVC Storage Cluster](pvc-cluster.md): Dynamically provision storage underneath Rook by specifying the storage class Rook should use to consume storage (via PVCs)
3. [Stretched Storage Cluster](stretch-cluster.md): Distribute Ceph mons across three zones, while storage (OSDs) is only configured in two zones
4. [External Ceph Cluster](external-cluster/external-cluster.md): Connect your K8s applications to an external Ceph cluster

See the separate topics for a description and examples of each of these scenarios.

## Settings

Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.  If any setting is unspecified, a suitable default will be used automatically.

### Cluster metadata

* `name`: The name that will be used internally for the Ceph cluster. Most commonly the name is the same as the namespace since multiple clusters are not supported in the same namespace.
* `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.

### Cluster Settings

* `external`:
    * `enable`: if `true`, the cluster will not be managed by Rook but via an external entity. This mode is intended to connect to an existing cluster. In this case, Rook will only consume the external cluster. However, Rook will be able to deploy various daemons in Kubernetes such as object gateways, mds and nfs if an image is provided and will refuse otherwise. If this setting is enabled **all** the other options will be ignored except `cephVersion.image` and `dataDirHostPath`. See [external cluster configuration](external-cluster/external-cluster.md). If `cephVersion.image` is left blank, Rook will refuse the creation of extra CRs like object, file and nfs.
* `cephVersion`: The version information for launching the ceph daemons.
    * `image`: The image used for running the ceph daemons. For example, `quay.io/ceph/ceph:v19.2.1`. For more details read the [container images section](#ceph-container-images).
        For the latest ceph images, see the [Ceph DockerHub](https://hub.docker.com/r/ceph/ceph/tags/).
        To ensure a consistent version of the image is running across all nodes in the cluster, it is recommended to use a very specific image version.
        Tags also exist that would give the latest version, but they are only recommended for test environments. For example, the tag `v19` will be updated each time a new Squid build is released.
        Using the general `v19` tag is not recommended in production because it may lead to inconsistent versions of the image running across different nodes in the cluster.
    * `allowUnsupported`: If `true`, allow an unsupported major version of the Ceph release. Currently Reef and Squid are supported. Future versions such as Tentacle (v20) would require this to be set to `true`. Should be set to `false` in production.
    * `imagePullPolicy`: The image pull policy for the ceph daemon pods. Possible values are `Always`, `IfNotPresent`, and `Never`. The default is `IfNotPresent`.
* `dataDirHostPath`: The path on the host ([hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)) where config and data should be stored for each of the services. If there are multiple clusters, the directory must be unique for each cluster. If the directory does not exist, it will be created. Because this directory persists on the host, it will remain after pods are deleted. Following paths and any of their subpaths **must not be used**: `/etc/ceph`, `/rook` or `/var/log/ceph`.
    * **WARNING**: For test scenarios, if you delete a cluster and start a new cluster on the same hosts, the path used by `dataDirHostPath` must be deleted. Otherwise, stale keys and other config will remain from the previous cluster and the new mons will fail to start.
If this value is empty, each pod will get an ephemeral directory to store their config files that is tied to the lifetime of the pod running on that node. More details can be found in the Kubernetes [empty dir docs](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
* `skipUpgradeChecks`: if set to true Rook won't perform any upgrade checks on Ceph daemons during an upgrade. Use this at **YOUR OWN RISK**, only if you know what you're doing. To understand Rook's upgrade process of Ceph, read the [upgrade doc](../../Upgrade/rook-upgrade.md#ceph-version-upgrades).
* `continueUpgradeAfterChecksEvenIfNotHealthy`: if set to true Rook will continue the OSD daemon upgrade process even if the PGs are not clean, or continue with the MDS upgrade even the file system is not healthy.
* `upgradeOSDRequiresHealthyPGs`: if set to true OSD upgrade process won't start until PGs are healthy.
* `dashboard`: Settings for the Ceph dashboard. To view the dashboard in your browser see the [dashboard guide](../../Storage-Configuration/Monitoring/ceph-dashboard.md).
    * `enabled`: Whether to enable the dashboard to view cluster status
    * `urlPrefix`: Allows to serve the dashboard under a subpath (useful when you are accessing the dashboard via a reverse proxy)
    * `port`: Allows to change the default port where the dashboard is served
    * `ssl`: Whether to serve the dashboard via SSL, ignored on Ceph versions older than `13.2.2`
* `monitoring`: Settings for monitoring Ceph using Prometheus. To enable monitoring on your cluster see the [monitoring guide](../../Storage-Configuration/Monitoring/ceph-monitoring.md#prometheus-alerts).
    * `enabled`: Whether to enable the prometheus service monitor for an internal cluster. For an external cluster, whether to create an endpoint port for the metrics. Default is false.
    * `metricsDisabled`: Whether to disable the metrics reported by Ceph. If false, the prometheus mgr module and Ceph exporter are enabled.
    If true, the prometheus mgr module and Ceph exporter are both disabled. Default is false.
    * `externalMgrEndpoints`: external cluster manager endpoints
    * `externalMgrPrometheusPort`: external prometheus manager module port. See [external cluster configuration](./external-cluster/external-cluster.md) for more details.
    * `port`: The internal prometheus manager module port where the prometheus mgr module listens. The port may need to be configured when host networking is enabled.
    * `interval`: The interval for the prometheus module to to scrape targets.
    * `exporter`: Ceph exporter metrics config.
        * `perfCountersPrioLimit`: Specifies which performance counters are exported. Corresponds to `--prio-limit` Ceph exporter flag. `0` - all counters are exported, default is `5`.
        * `statsPeriodSeconds`: Time to wait before sending requests again to exporter server (seconds). Corresponds to `--stats-period` Ceph exporter flag. Default is `5`.
* `network`: For the network settings for the cluster, refer to the [network configuration settings](#network-configuration-settings)
* `mon`: contains mon related options [mon settings](#mon-settings)
For more details on the mons and when to choose a number other than `3`, see the [mon health doc](../../Storage-Configuration/Advanced/ceph-mon-health.md).
* `mgr`: manager top level section
    * `count`: set number of ceph managers between `1` to `2`. The default value is 2.
        If there are two managers, it is important for all mgr services point to the active mgr and not the standby mgr. Rook automatically
        updates the label `mgr_role` on the mgr pods to be either `active` or `standby`. Therefore, services need just to add the label
        `mgr_role=active` to their selector to point to the active mgr. This applies to all services that rely on the ceph mgr such as
        the dashboard or the prometheus metrics collector.
    * `modules`: A list of Ceph manager modules to enable or disable. Note the "dashboard" and "monitoring" modules are already configured by other settings.
* `crashCollector`: The settings for crash collector daemon(s).
    * `disable`: is set to `true`, the crash collector will not run on any node where a Ceph daemon runs
    * `daysToRetain`: specifies the number of days to keep crash entries in the Ceph cluster. By default the entries are kept indefinitely.
* `logCollector`: The settings for log collector daemon.
    * `enabled`: if set to `true`, the log collector will run as a side-car next to each Ceph daemon. The Ceph configuration option `log_to_file` will be turned on, meaning Ceph daemons will log on files in addition to still logging to container's stdout. These logs will be rotated. In case a daemon terminates with a segfault, the coredump files will be commonly be generated in `/var/lib/systemd/coredump` directory on the host, depending on the underlying OS location. (default: `true`)
    * `periodicity`: how often to rotate daemon's log. (default: 24h). Specified with a time suffix which may be `h` for hours or `d` for days. **Rotating too often will slightly impact the daemon's performance since the signal briefly interrupts the program.**
* `annotations`: [annotations configuration settings](#annotations-and-labels)
* `labels`: [labels configuration settings](#annotations-and-labels)
* `placement`: [placement configuration settings](#placement-configuration-settings)
* `resources`: [resources configuration settings](#cluster-wide-resources-configuration-settings)
* `priorityClassNames`: [priority class names configuration settings](#priority-class-names)
* `storage`: Storage selection and configuration that will be used across the cluster.  Note that these settings can be overridden for specific nodes.
    * `useAllNodes`: `true` or `false`, indicating if all nodes in the cluster should be used for storage according to the cluster level storage selection and configuration values.
        If individual nodes are specified under the `nodes` field, then `useAllNodes` must be set to `false`.
    * `nodes`: Names of individual nodes in the cluster that should have their storage included in accordance with either the cluster level configuration specified above or any node specific overrides described in the next section below.
        `useAllNodes` must be set to `false` to use specific nodes and their config.
        See [node settings](#node-settings) below.
    * `config`: Config settings applied to all OSDs on the node unless overridden by `devices`. See the [config settings](#osd-configuration-settings) below.
    * `allowDeviceClassUpdate`: Whether to allow changing the device class of an OSD after it is created. The default is false
        to prevent unintentional data movement or CRUSH changes if the device class is changed accidentally.
    * `allowOsdCrushWeightUpdate`: Whether Rook will resize the OSD CRUSH weight when the OSD PVC size is increased.
        This allows cluster data to be rebalanced to make most effective use of new OSD space.
        The default is false since data rebalancing can cause temporary cluster slowdown.
    * [storage selection settings](#storage-selection-settings)
    * [Storage Class Device Sets](#storage-class-device-sets)
    * `onlyApplyOSDPlacement`: Whether the placement specific for OSDs is merged with the `all` placement. If `false`, the OSD placement will be merged with the `all` placement. If true, the `OSD placement will be applied` and the `all` placement will be ignored. The placement for OSDs is computed from several different places depending on the type of OSD:
        * For non-PVCs: `placement.all` and `placement.osd`
        * For PVCs: `placement.all` and inside the storageClassDeviceSets from the `placement` or `preparePlacement`
    * `flappingRestartIntervalHours`: Defines the time for which an OSD pod will sleep before restarting, if it stopped due to flapping. Flapping occurs where OSDs are marked `down` by Ceph more than 5 times in 600 seconds. The OSDs will stay down when flapping since they likely have a bad disk or other issue that needs investigation. If the issue with the OSD is fixed manually, the OSD pod can be manually restarted. The sleep is disabled if this interval is set to 0.
    * `scheduleAlways`: Whether to always schedule OSD pods on nodes declared explicitly in the "nodes" section, even if they are
        temporarily not schedulable. If set to true, consider adding placement tolerations for unschedulable nodes.
    * `fullRatio`: The ratio at which Ceph should block IO if the OSDs are too full. The default is 0.95.
    * `backfillFullRatio`: The ratio at which Ceph should stop backfilling data if the OSDs are too full. The default is 0.90.
    * `nearFullRatio`: The ratio at which Ceph should raise a health warning if the cluster is almost full. The default is 0.85.
* `disruptionManagement`: The section for configuring management of daemon disruptions
    * `managePodBudgets`: if `true`, the operator will create and manage PodDisruptionBudgets for OSD, Mon, RGW, and MDS daemons. OSD PDBs are managed dynamically via the strategy outlined in the [design](https://github.com/rook/rook/blob/master/design/ceph/ceph-managed-disruptionbudgets.md). The operator will block eviction of OSDs by default and unblock them safely when drains are detected.
    * `osdMaintenanceTimeout`: is a duration in minutes that determines how long an entire failureDomain like `region/zone/host` will be held in `noout` (in addition to the default DOWN/OUT interval) when it is draining. The default value is `30` minutes.
    * `pgHealthyRegex`: The regular expression that is used to determine which PG states should be considered healthy.
    The default is `^(active\+clean|active\+clean\+scrubbing|active\+clean\+scrubbing\+deep)$`.
* `removeOSDsIfOutAndSafeToRemove`: If `true` the operator will remove the OSDs that are down and whose data has been restored to other OSDs. In Ceph terms, the OSDs are `out` and `safe-to-destroy` when they are removed.
* `cleanupPolicy`: [cleanup policy settings](#cleanup-policy)
* `security`: [security page for key management configuration](../../Storage-Configuration/Advanced/key-management-system.md)
* `cephConfig`: [Set Ceph config options using the Ceph Mon config store](#ceph-config)
* `csi`: [Set CSI Driver options](#csi-driver-options)

### Ceph container images

Official releases of Ceph Container images are available from [Docker Hub](https://hub.docker.com/r/ceph
).

These are general purpose Ceph container with all necessary daemons and dependencies installed.

| TAG                  | MEANING                                                   |
| -------------------- | --------------------------------------------------------- |
| vRELNUM              | Latest release in this series (e.g., **v19** = Squid)     |
| vRELNUM.Y            | Latest stable release in this stable series (e.g., v19.2) |
| vRELNUM.Y.Z          | A specific release (e.g., v19.2.1)                        |
| vRELNUM.Y.Z-YYYYMMDD | A specific build (e.g., v19.2.1-20250202)                 |

A specific will contain a specific release of Ceph as well as security fixes from the Operating System.

### Mon Settings

* `count`: Set the number of mons to be started. The number must be between `1` and `9`. The recommended value is most commonly `3`.
    For highest availability, an odd number of mons should be specified.
    For higher durability in case of mon loss, an even number can be specified although availability may be lower.
    To maintain quorum a majority of mons must be up. For example, if there are three mons, two must be up.
    If there are four mons, three must be up. If there are two mons, both must be up.
    If quorum is lost, see the [disaster recovery guide](../../Troubleshooting/disaster-recovery.md#restoring-mon-quorum) to restore quorum from a single mon.
* `allowMultiplePerNode`: Whether to allow the placement of multiple mons on a single node. Default is `false` for production. Should only be set to `true` in test environments.
* `volumeClaimTemplate`: A `PersistentVolumeSpec` used by Rook to create PVCs
    for monitor storage. This field is optional, and when not provided, HostPath
    volume mounts are used.  The current set of fields from template that are used
    are `storageClassName` and the `storage` resource request and limit. The
    default storage size request for new PVCs is `10Gi`. Ensure that associated
    storage class is configured to use `volumeBindingMode: WaitForFirstConsumer`.
    This setting only applies to new monitors that are created when the requested
    number of monitors increases, or when a monitor fails and is recreated. An
    [example CRD configuration is provided below](./pvc-cluster.md).
* `failureDomainLabel`: The label that is expected on each node where the mons
    are expected to be deployed. The labels must be found in the list of
    well-known [topology labels](#osd-topology).
* `externalMonIDs`: ID list of external mons deployed outside of Rook cluster
    and not managed by Rook. If set, Rook will not remove external mons from quorum
    and populate external mons addresses to mon endpoints for CSI.
    This parameter is supported only for local Rook cluster running in normal mode,
    meaning that it will be ignored for external cluster (`spec.external.enabled: true`)
    or for `stretchedCluster`.
    For more details see [external mons](../../Storage-Configuration/Advanced/ceph-mon-health.md#external-monitors).
* `zones`: The failure domain names where the Mons are expected to be deployed.
    There must be **at least three zones** specified in the list. Each zone can be
    backed by a different storage class by specifying the `volumeClaimTemplate`.
    * `name`: The name of the zone, which is the value of the domain label.
    * `volumeClaimTemplate`: A `PersistentVolumeSpec` used by Rook to create PVCs
        for monitor storage. This field is optional, and when not provided, HostPath
        volume mounts are used.  The current set of fields from template that are used
        are `storageClassName` and the `storage` resource request and limit. The
        default storage size request for new PVCs is `10Gi`. Ensure that associated
        storage class is configured to use `volumeBindingMode: WaitForFirstConsumer`.
        This setting only applies to new monitors that are created when the requested
        number of monitors increases, or when a monitor fails and is recreated. An
        [example CRD configuration is provided below](./pvc-cluster.md).

* `stretchCluster`: The stretch cluster settings that define the zones (or other failure domain labels) across which to configure the cluster.
    * `failureDomainLabel`: The label that is expected on each node where the cluster is expected to be deployed. The labels must be found
    in the list of well-known [topology labels](#osd-topology).
    * `subFailureDomain`: With a zone, the data replicas must be spread across OSDs in the subFailureDomain. The default is `host`.
    * `zones`: The failure domain names where the Mons and OSDs are expected to be deployed. There must be **three zones** specified in the list.
    This element is always named `zone` even if a non-default `failureDomainLabel` is specified. The elements have two values:
        * `name`: The name of the zone, which is the value of the domain label.
        * `arbiter`: Whether the zone is expected to be the arbiter zone which only runs a single mon. Exactly one zone must be labeled `true`.
        * `volumeClaimTemplate`: A `PersistentVolumeSpec` used by Rook to create PVCs
            for monitor storage. This field is optional, and when not provided, HostPath
            volume mounts are used.  The current set of fields from template that are used
            are `storageClassName` and the `storage` resource request and limit. The
            default storage size request for new PVCs is `10Gi`. Ensure that associated
            storage class is configured to use `volumeBindingMode: WaitForFirstConsumer`.
            This setting only applies to new monitors that are created when the requested
            number of monitors increases, or when a monitor fails and is recreated. An
            [example CRD configuration is provided below](./pvc-cluster.md).
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
* `selectors`: Used for `multus` provider only. Select NetworkAttachmentDefinitions to use for Ceph networks.
    * `public`: Select the NetworkAttachmentDefinition to use for the public network.
    * `cluster`: Select the NetworkAttachmentDefinition to use for the cluster network.
* `addressRanges`: Used for `host` or `multus` providers only. Allows overriding the address ranges (CIDRs) that Ceph will listen on.
    * `public`: A list of individual network ranges in CIDR format to use for Ceph's public network.
    * `cluster`: A list of individual network ranges in CIDR format to use for Ceph's cluster network.
* `ipFamily`: Specifies the network stack Ceph daemons should listen on.
* `dualStack`: Specifies that Ceph daemon should listen on both IPv4 and IPv6 network stacks.
* `connections`: Settings for network connections using Ceph's msgr2 protocol
    * `requireMsgr2`: Whether to require communication over msgr2. If true, the msgr v1 port (6789) will be disabled
        and clients will be required to connect to the Ceph cluster with the v2 port (3300).
        Requires a kernel that supports msgr2 (kernel 5.11 or CentOS 8.4 or newer). Default is false.
    * `encryption`: Settings for encryption on the wire to Ceph daemons
        * `enabled`: Whether to encrypt the data in transit across the wire to prevent eavesdropping the data on the network.
            The default is false. When encryption is enabled, all communication between clients and Ceph daemons, or between
            Ceph daemons will be encrypted. When encryption is not enabled, clients still establish a strong initial authentication
            and data integrity is still validated with a crc check.
            **IMPORTANT**: Encryption requires the 5.11 kernel for the latest nbd and cephfs drivers. Alternatively for testing only,
            set "mounter: rbd-nbd" in the rbd storage class, or "mounter: fuse" in the cephfs storage class.
            The nbd and fuse drivers are **not** recommended in production since restarting the csi driver pod will disconnect the volumes.
            If this setting is enabled, CephFS volumes also require setting `CSI_CEPHFS_KERNEL_MOUNT_OPTIONS` to `"ms_mode=secure"` in operator.yaml.
    * `compression`:
        * `enabled`: Whether to compress the data in transit across the wire. The default is false.
            See the kernel requirements above for encryption.

!!! caution
    Changing networking configuration after a Ceph cluster has been deployed is only supported for
    the network encryption settings. Changing other network settings is **NOT** supported and will
    likely result in a non-functioning cluster.

#### Provider

Selecting a non-default network provider is an advanced topic. Read more in the
[Network Providers](./network-providers.md) documentation.

#### IPFamily

Provide single-stack IPv4 or IPv6 protocol to assign corresponding addresses to pods and services. This field is optional. Possible inputs are IPv6 and IPv4. Empty value will be treated as IPv4.
To enable dual stack see the [network configuration section](#network-configuration-settings).

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
nodes by taint or affinity modifications must be "confirmed" by deleting the Rook Ceph operator pod
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

Below are the settings for host-based cluster. This type of cluster can specify devices for OSDs, both at the cluster and individual node level, for selecting which storage resources will be included in the cluster.

* `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by OSDs. **Not recommended** unless you have a very controlled environment where you will not risk formatting of devices with existing data. When `true`, all devices and partitions will be used. Is overridden by `deviceFilter` if specified. LVM logical volumes are not picked by `useAllDevices`.
* `deviceFilter`: A regular expression for short kernel names of devices (e.g. `sda`) that allows selection of devices and partitions to be consumed by OSDs.  LVM logical volumes are not picked by `deviceFilter`.If individual devices have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
    * `sdb`: Only selects the `sdb` device if found
    * `^sd.`: Selects all devices starting with `sd`
    * `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
    * `^s`: Selects all devices that start with `s`
    * `^[^r]`: Selects all devices that do **not** start with `r`
* `devicePathFilter`: A regular expression for device paths (e.g. `/dev/disk/by-path/pci-0:1:2:3-scsi-1`) that allows selection of devices and partitions to be consumed by OSDs.  LVM logical volumes are not picked by `devicePathFilter`.If individual devices or `deviceFilter` have been specified for a node then this filter will be ignored.  This field uses [golang regular expression syntax](https://golang.org/pkg/regexp/syntax/). For example:
    * `^/dev/sd.`: Selects all devices starting with `sd`
    * `^/dev/disk/by-path/pci-.*`: Selects all devices which are connected to PCI bus
* `devices`: A list of individual device names belonging to this node to include in the storage cluster.
    * `name`: The name of the devices and partitions (e.g., `sda`). The full udev path can also be specified for devices, partitions, and logical volumes (e.g. `/dev/disk/by-id/ata-ST4000DM004-XXXX` - this will not change after reboots).
    * `config`: Device-specific config settings. See the [config settings](#osd-configuration-settings) below

Host-based cluster supports raw devices, partitions, logical volumes, encrypted devices, and multipath devices. Be sure to see the
[quickstart doc prerequisites](../../Getting-Started/quickstart.md#prerequisites) for additional considerations.

Below are the settings for a PVC-based cluster.

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
* `tuneDeviceClass`: For example, Ceph cannot detect AWS volumes as HDDs from the storage class "gp2-csi", so you can improve Ceph performance by setting this to true.
* `tuneFastDeviceClass`: For example, Ceph cannot detect Azure disks as SSDs from the storage class "managed-premium", so you can improve Ceph performance by setting this to true..
* `volumeClaimTemplates`: A list of PVC templates to use for provisioning the underlying storage devices.
    * `metadata.name`: "data", "metadata", or "wal". If a single template is provided, the name must be "data". If the name is "metadata" or "wal", the devices are used to store the Ceph metadata or WAL respectively. In both cases, the devices must be raw devices or LVM logical volumes.
        * `resources.requests.storage`: The desired capacity for the underlying storage devices.
        * `storageClassName`: The StorageClass to provision PVCs from. Default would be to use the cluster-default StorageClass.
        * `volumeMode`: The volume mode to be set for the PVC. Which should be Block
        * `accessModes`: The access mode for the PVC to be bound by OSD.
* `schedulerName`: Scheduler name for OSD pod placement. (Optional)
* `encrypted`: whether to encrypt all the OSDs in a given storageClassDeviceSet

See the table in [OSD Configuration Settings](#osd-configuration-settings) to know the allowed configurations.

### OSD Configuration Settings

The following storage selection settings are specific to Ceph and do not apply to other backends. All variables are key-value pairs represented as strings.

* `metadataDevice`: Name of a device, [partition](#limitations-of-metadata-device) or lvm to use for the metadata of OSDs on each node.  Performance can be improved by using a low latency device (such as SSD or NVMe) as the metadata device, while other spinning platter (HDD) devices on a node are used to store data. Provisioning will fail if the user specifies a `metadataDevice` but that device is not used as a metadata device by Ceph. Notably, `ceph-volume` will not use a device of the same device class (HDD, SSD, NVMe) as OSD devices for metadata, resulting in this failure.
* `databaseSizeMB`:  The size in MB of a bluestore database. Include quotes around the size.
* `walSizeMB`:  The size in MB of a bluestore write ahead log (WAL). Include quotes around the size.
* `deviceClass`: The [CRUSH device class](https://ceph.io/community/new-luminous-crush-device-classes/) to use for this selection of storage devices. (By default, if a device's class has not already been set, OSDs will automatically set a device's class to either `hdd`, `ssd`, or `nvme`  based on the hardware properties exposed by the Linux kernel.) These storage classes can then be used to select the devices backing a storage pool by specifying them as the value of [the pool spec's `deviceClass` field](../Block-Storage/ceph-block-pool-crd.md#spec). If updating the device class of an OSD after the OSD is already created, `allowDeviceClassUpdate: true` must be set. Otherwise updates to this `deviceClass` will be ignored.
* `initialWeight`: The initial OSD weight in TiB units. By default, this value is derived from OSD's capacity.
* `primaryAffinity`: The [primary-affinity](https://docs.ceph.com/en/latest/rados/operations/crush-map/#primary-affinity) value of an OSD, within range `[0, 1]` (default: `1`).
* `osdsPerDevice`**: The number of OSDs to create on each device. High performance devices such as NVMe can handle running multiple OSDs. If desired, this can be overridden for each node and each device.
* `encryptedDevice`**: Encrypt OSD volumes using dmcrypt ("true" or "false"). By default this option is disabled. See [encryption](http://docs.ceph.com/docs/master/ceph-volume/lvm/encryption/) for more information on encryption in Ceph. (Resizing is not supported for host-based clusters.)
* `crushRoot`: The value of the `root` CRUSH map label. The default is `default`. Generally, you should not need to change this. However, if any of your topology labels may have the value `default`, you need to change `crushRoot` to avoid conflicts, since CRUSH map values need to be unique.
* `enableCrushUpdates`: Enables rook to update the pool crush rule using Pool Spec. Can cause data remapping if crush rule changes, Defaults to false.
* `migration`: Existing PVC based OSDs can be migrated to enable or disable encryption. Refer to the [osd management](../../Storage-Configuration/Advanced/ceph-osd-mgmt.md/#osd-encryption-as-day-2-operation) topic for details.

Allowed configurations are:

| block device type | host-based cluster                                                                                | PVC-based cluster                                                               |
| :---------------- | :------------------------------------------------------------------------------------------------ | :------------------------------------------------------------------------------ |
| disk              |                                                                                                   |                                                                                 |
| part              | `encryptedDevice` must be `false`                                                                 | `encrypted` must be `false`                                                     |
| lvm               | `metadataDevice` must be `""`, `osdsPerDevice` must be `1`, and `encryptedDevice` must be `false` | `metadata.name` must not be `metadata` or `wal` and `encrypted` must be `false` |
| crypt             |                                                                                                   |                                                                                 |
| mpath             |                                                                                                   |                                                                                 |

#### Limitations of metadata device

- If `metadataDevice` is specified in the global OSD configuration or in the node level OSD configuration, the metadata device will be shared between all OSDs on the same node. In other words, OSDs will be initialized by `lvm batch`. In this case, we can't use partition device.
- If `metadataDevice` is specified in the device local configuration, we can use partition as metadata device. In other words, OSDs are initialized by `lvm prepare`.

### Annotations and Labels

Annotations and Labels can be specified so that the Rook components will have those annotations / labels added to them.

You can set annotations / labels for Rook components for the list of key value pairs:

* `all`: Set annotations / labels for all components except `clusterMetadata`.
* `mgr`: Set annotations / labels for MGRs
* `mon`: Set annotations / labels for mons
* `osd`: Set annotations / labels for OSDs
* `dashboard`: Set annotations / labels for the dashboard service
* `prepareosd`: Set annotations / labels for OSD Prepare Jobs
* `monitoring`: Set annotations / labels for service monitor
* `crashcollector`: Set annotations / labels for crash collectors
* `clusterMetadata`: Set annotations  only to `rook-ceph-mon-endpoints` configmap and the  `rook-ceph-mon` and `rook-ceph-admin-keyring` secrets. These annotations will not be merged with the `all` annotations. The common usage is for backing up these critical resources with `kubed`.
Note the clusterMetadata annotation will not be merged with the `all` annotation.
When other keys are set, `all` will be merged together with the specific component.

### Placement Configuration Settings

Placement configuration for the cluster services. It includes the following keys: `mgr`, `mon`, `arbiter`, `osd`, `prepareosd`, `cleanup`, and `all`.
Each service will have its placement configuration generated by merging the generic configuration under `all` with the most specific one (which will override any attributes).

In stretch clusters, if the `arbiter` placement is specified, that placement will only be applied to the arbiter.
Neither will the `arbiter` placement be merged with the `all` placement to allow the arbiter to be fully independent of other daemon placement.
The remaining mons will still use the `mon` and/or `all` sections.

!!! note
    Placement of OSD pods is controlled using the [Storage Class Device Set](#storage-class-device-sets), not the general `placement` configuration.

A Placement configuration is specified (according to the kubernetes PodSpec) as:

* `nodeAffinity`: kubernetes [NodeAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity-beta-feature)
* `podAffinity`: kubernetes [PodAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
* `podAntiAffinity`: kubernetes [PodAntiAffinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature)
* `tolerations`: list of kubernetes [Toleration](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)
* `topologySpreadConstraints`: kubernetes [TopologySpreadConstraints](https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/)

If you use `labelSelector` for `osd` pods, you must write two rules both for `rook-ceph-osd` and `rook-ceph-osd-prepare` like [the example configuration](https://github.com/rook/rook/blob/master/deploy/examples/cluster-on-pvc.yaml#L68). It comes from the design that there are these two pods for an OSD. For more detail, see the [osd design doc](https://github.com/rook/rook/blob/master/design/ceph/dedicated-osd-pod.md) and [the related issue](https://github.com/rook/rook/issues/4582).

The Rook Ceph operator creates a Job called `rook-ceph-detect-version` to detect the full Ceph version used by the given `cephVersion.image`. The placement from the `mon` section is used for the Job except for the `PodAntiAffinity` field.

#### Placement Example

To control where various services will be scheduled by kubernetes, use the placement configuration sections below.
The example under 'all' would have all services scheduled on kubernetes nodes labeled with 'role=storage-node`.
Specific node affinity and tolerations that only apply to the`mon`daemons in this example require the label
`role=storage-mon-node` and also tolerate the control plane taint.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v19.2.1
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
    mon:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - storage-mon-node
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/control-plane
        operator: Exists
```

### Cluster-wide Resources Configuration Settings

Resources should be specified so that the Rook components are handled after [Kubernetes Pod Quality of Service classes](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/).
This allows to keep Rook components running when for example a node runs out of memory and the Rook components are not killed depending on their Quality of Service class.

You can set resource requests/limits for Rook components through the [Resource Requirements/Limits](#resource-requirementslimits) structure in the following keys:

* `mon`: Set resource requests/limits for mons
* `osd`: Set resource requests/limits for OSDs.
    This key applies for all OSDs regardless of their device classes.
    In case of need to apply resource requests/limits for OSDs with particular device class use specific osd keys below.
    If the memory resource is declared Rook will automatically set the OSD configuration `osd_memory_target` to the same value.
    This aims to ensure that the actual OSD memory consumption is consistent with the OSD pods' resource declaration.
* `osd-<deviceClass>`: Set resource requests/limits for OSDs on a specific device class.
    Rook will automatically detect `hdd`, `ssd`, or `nvme` device classes. Custom device classes can also be set.
* `mgr`: Set resource requests/limits for MGRs
* `mgr-sidecar`: Set resource requests/limits for the MGR sidecar, which is only created when `mgr.count: 2`.
    The sidecar requires very few resources since it only executes every 15 seconds to query Ceph for the active
    mgr and update the mgr services if the active mgr changed.
* `prepareosd`: Set resource requests/limits for OSD prepare job
* `crashcollector`: Set resource requests/limits for crash. This pod runs wherever there is a Ceph pod running.
It scrapes for Ceph daemon core dumps and sends them to the Ceph manager crash module so that core dumps are centralized and can be easily listed/accessed.
You can read more about the [Ceph Crash module](https://docs.ceph.com/docs/master/mgr/crash/).
* `logcollector`: Set resource requests/limits for the log collector. When enabled, this container runs as side-car to each Ceph daemons.
* `cmd-reporter`: Set resource requests/limits for the jobs that detect the ceph version and collect network info.
* `cleanup`: Set resource requests/limits for cleanup job, responsible for wiping cluster's data after uninstall
* `exporter`: Set resource requests/limits for Ceph exporter.

In order to provide the best possible experience running Ceph in containers, Rook internally recommends minimum memory limits if resource limits are passed.
If a user configures a limit or request value that is too low, Rook will still run the pod(s) and print a warning to the operator log.

* `mon`: 1024MB
* `mgr`: 512MB
* `osd`: 2048MB
* `crashcollector`: 60MB
* `mgr-sidecar`: 100MB limit, 40MB requests
* `prepareosd`: no limits (see the note)
* `exporter`: 128MB limit, 50MB requests

!!! note
    We recommend not setting memory limits on the OSD prepare job to prevent OSD provisioning failure due to memory constraints.
    The OSD prepare job bursts memory usage during the OSD provisioning depending on the size of the device, typically
    1-2Gi for large disks. The OSD prepare job only bursts a single time per OSD.
    All future runs of the OSD prepare job will detect the OSD is already provisioned and skip the provisioning.

!!! hint
    The resources for MDS daemons are not configured in the Cluster. Refer to the [Ceph Filesystem CRD](../Shared-Filesystem/ceph-filesystem-crd.md) instead.

### Resource Requirements/Limits

For more information on resource requests/limits see the official Kubernetes documentation: [Kubernetes - Managing Compute Resources for Containers](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container)

* `requests`: Requests for cpu or memory.
    * `cpu`: Request for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
    * `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).
* `limits`: Limits for cpu or memory.
    * `cpu`: Limit for CPU (example: one CPU core `1`, 50% of one CPU core `500m`).
    * `memory`: Limit for Memory (example: one gigabyte of memory `1Gi`, half a gigabyte of memory `512Mi`).

!!! warning
    Before setting resource requests/limits, please take a look at the Ceph documentation for recommendations for each component: [Ceph - Hardware Recommendations](http://docs.ceph.com/docs/master/start/hardware-recommendations/).

#### Node Specific Resources for OSDs

This example shows that you can override these requests/limits for OSDs per node when using `useAllNodes: false` in the `node` item in the `nodes` list.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v19.2.1
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  storage:
    useAllNodes: false
    nodes:
    - name: "172.17.4.201"
      resources:
        limits:
          memory: "4096Mi"
        requests:
          cpu: "2"
          memory: "4096Mi"
```

### Priority Class Names

Priority class names can be specified so that the Rook components will have those priority class names added to them.

You can set priority class names for Rook components for the list of key value pairs:

* `all`: Set priority class names for MGRs, Mons, OSDs, and crashcollectors.
* `mgr`: Set priority class names for MGRs. Examples default to system-cluster-critical.
* `mon`: Set priority class names for Mons. Examples default to system-node-critical.
* `osd`: Set priority class names for OSDs. Examples default to system-node-critical.
* `crashcollector`: Set priority class names for crashcollectors.
* `exporter`: Set priority class names for exporters.
* `cleanup`: Set priority class names for cleanup Jobs.

The specific component keys will act as overrides to `all`.

### Health settings

The Rook Ceph operator will monitor the state of the CephCluster on various components by default.
The following CRD settings are available:

* `healthCheck`: main ceph cluster health monitoring section

Currently three health checks are implemented:

* `mon`: health check on the ceph monitors, basically check whether monitors are members of the quorum. If after a certain timeout a given monitor has not joined the quorum back it will be failed over and replace by a new monitor.
* `osd`: health check on the ceph osds
* `status`: ceph health status check, periodically check the Ceph health state and reflects it in the CephCluster CR status field.

The liveness probe and startup probe of each daemon can also be controlled via `livenessProbe` and
`startupProbe` respectively. The settings are valid for `mon`, `mgr` and `osd`.
Here is a complete example for both `daemonHealth`, `livenessProbe`, and `startupProbe`:

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
  startupProbe:
    mon:
      disabled: false
    mgr:
      disabled: false
    osd:
      disabled: false
```

The probe's timing values and thresholds (but not the probe itself) can also be overridden.
For more info, refer to the
[Kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-a-liveness-command).

For example, you could change the `mgr` probe by applying:

```yaml
healthCheck:
  startupProbe:
    mgr:
      disabled: false
      probe:
        initialDelaySeconds: 3
        periodSeconds: 3
        failureThreshold: 30
  livenessProbe:
    mgr:
      disabled: false
      probe:
        initialDelaySeconds: 3
        periodSeconds: 3
```

Changing the liveness probe is an advanced operation and should rarely be necessary. If you want to change these settings then modify the desired settings.

## Status

The operator is regularly configuring and checking the health of the cluster. The results of the configuration
and health checks can be seen in the `status` section of the CephCluster CR.

```console
kubectl -n rook-ceph get CephCluster -o yaml
```

```yaml
[...]
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
      image: quay.io/ceph/ceph:v19.2.1
      version: 16.2.6-0
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

* If the cluster is fully configured and the operator is stable, the
    `Ready` condition is raised with `ClusterCreated` reason and no other conditions. The cluster
    will remain in the `Ready` condition after the first successful configuration since it
    is expected the storage is consumable from this point on. If there are issues preventing
    the storage layer from working, they are expected to show as Ceph health errors.
* If the cluster is externally connected successfully, the `Ready` condition will have the reason `ClusterConnected`.
* If the operator is currently being configured or the operator is checking for update,
    there will be a `Progressing` condition.
* If there was a failure, the condition(s) status will be `false` and the `message` will
    give a summary of the error. See the operator log for more details.

### Other Status

There are several other properties for the overall status including:

* `message`, `phase`, and `state`: A summary of the overall current state of the cluster, which
    is somewhat duplicated from the conditions for backward compatibility.
* `storage.deviceClasses`: The names of the types of storage devices that Ceph discovered
    in the cluster. These types will be `ssd` or `hdd` unless they have been overridden
    with the `crushDeviceClass` in the `storageClassDeviceSets`.
* `version`: The version of the Ceph image currently deployed.

## OSD Topology

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

These labels would result in the following hierarchy for OSDs on that node (this command can be run in the Rook toolbox):

```console
$ ceph osd tree
ID CLASS WEIGHT  TYPE NAME                 STATUS REWEIGHT PRI-AFF
-1       0.01358 root default
-5       0.01358     zone zone1
-4       0.01358         rack rack1
-3       0.01358             host mynode
0   hdd 0.00679                 osd.0         up  1.00000 1.00000
1   hdd 0.00679                 osd.1         up  1.00000 1.00000
```

Ceph requires unique names at every level in the hierarchy (CRUSH map). For example, you cannot have two racks
with the same name that are in different zones. Racks in different zones must be named uniquely.

Note that the `host` is added automatically to the hierarchy by Rook. The host cannot be specified with a topology label.
All topology labels are optional.

!!! hint
    When setting the node labels prior to `CephCluster` creation, these settings take immediate effect. However, applying this to an already deployed `CephCluster` requires removing each node from the cluster first and then re-adding it with new configuration to take effect. Do this node by node to keep your data safe! Check the result with `ceph osd tree` from the [Rook Toolbox](../../Troubleshooting/ceph-toolbox.md). The OSD tree should display the hierarchy for the nodes that already have been re-added.

To utilize the `failureDomain` based on the node labels, specify the corresponding option in the [CephBlockPool](../Block-Storage/ceph-block-pool-crd.md)

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

## Deleting a CephCluster

During deletion of a CephCluster resource, Rook protects against accidental or premature destruction
of user data by blocking deletion if there are any other Rook Ceph Custom Resources that reference
the CephCluster being deleted. Rook will warn about which other resources are blocking deletion in
three ways until all blocking resources are deleted:

1. An event will be registered on the CephCluster resource
2. A status condition will be added to the CephCluster resource
3. An error will be added to the Rook Ceph operator log

## Cleanup policy

Rook has the ability to cleanup resources and data that were deployed when a CephCluster is removed.
The policy settings indicate which data should be forcibly deleted and in what way the data should be wiped.
The `cleanupPolicy` has several fields:

* `confirmation`: Only an empty string and `yes-really-destroy-data` are valid values for this field.
    If this setting is empty, the `cleanupPolicy` settings will be ignored and Rook will not cleanup any resources during cluster removal.
    To reinstall the cluster, the admin would then be required to follow the [cleanup guide](../../Storage-Configuration/ceph-teardown.md) to delete the data on hosts.
    If this setting is `yes-really-destroy-data`, the operator will automatically delete the data on hosts.
    Because this cleanup policy is destructive, after the confirmation is set to `yes-really-destroy-data`
    Rook will stop configuring the cluster as if the cluster is about to be destroyed.
* `sanitizeDisks`: sanitizeDisks represents advanced settings that can be used to delete data on drives.
    * `method`: indicates if the entire disk should be sanitized or simply ceph's metadata. Possible choices are `quick` (default) or `complete`
    * `dataSource`: indicate where to get random bytes from to write on the disk. Possible choices are `zero` (default) or `random`.
        Using random sources will consume entropy from the system and will take much more time then the zero source
    * `iteration`: overwrite N times instead of the default (1). Takes an integer value
* `allowUninstallWithVolumes`: If set to true, then the cephCluster deletion doesn't wait for the PVCs to be deleted. Default is `false`.

To automate activation of the cleanup, you can use the following command. **WARNING: DATA WILL BE PERMANENTLY DELETED**:

```console
kubectl -n rook-ceph patch cephcluster rook-ceph --type merge -p '{"spec":{"cleanupPolicy":{"confirmation":"yes-really-destroy-data"}}}'
```

Nothing will happen until the deletion of the CR is requested, so this can still be reverted.
However, all new configuration by the operator will be blocked with this cleanup policy enabled.

Rook waits for the deletion of PVs provisioned using the cephCluster before proceeding to delete the
cephCluster. To force deletion of the cephCluster without waiting for the PVs to be deleted, you can
set the `allowUninstallWithVolumes` to true under `spec.CleanupPolicy`.

## Ceph Config

The Ceph config options are applied after the MONs are all in quorum and running.
To set Ceph config options, you can add them to your `CephCluster` spec as shown below.
See the [Ceph config reference](https://docs.ceph.com/en/latest/rados/configuration/general-config-ref/)
for detailed information about how to configure Ceph.

```yaml
spec:
  # [...]
  cephConfig:
    # Who's the target for these config options?
    global:
      # All values must be quoted so they are considered a string in YAML
      osd_pool_default_size: "3"
      mon_warn_on_pool_no_redundancy: "false"
      osd_crush_update_on_start: "false"
    # Make sure to quote special characters
    "osd.*":
      osd_max_scrubs: "10"
```

The Rook operator will actively apply these values, whereas the
[ceph.conf settings](../../Storage-Configuration/Advanced/ceph-configuration/#custom-cephconf-settings)
only take effect after the Ceph daemon pods are restarted.

If both these `cephConfig` and [ceph.conf settings](../../Storage-Configuration/Advanced/ceph-configuration/#custom-cephconf-settings)
are applied, the `cephConfig` settings will take higher precedence if there is an overlap.

If Ceph settings need to be applied to mons before quorum is initially created, the
[ceph.conf settings](../../Storage-Configuration/Advanced/ceph-configuration/#custom-cephconf-settings)
should be used instead.

!!! warning
    Rook performs no direct validation on these config options, so the validity of the settings is the
    user's responsibility.

The operator does not unset any removed config options, it is the user's responsibility to unset or set the default value for each removed option manually using the Ceph CLI.

## CSI Driver Options

The CSI driver options mentioned here are applied per Ceph cluster. The following options are available:

* `readAffinity`: RBD and CephFS volumes allow serving reads from an OSD in proximity to the client. Refer to the read affinity section in the [Ceph CSI Drivers](../../Storage-Configuration/Ceph-CSI/ceph-csi-drivers.md#enable-read-affinity-for-rbd-and-cephfs-volumes) for more details.
    * `enabled`: Whether to enable read affinity for the CSI driver. Default is `false`.
    * `crushLocationLabels`:  Node labels to use as CRUSH location, corresponding to the values set in the CRUSH map. Defaults to the labels mentioned in the
[OSD topology](#osd-topology) topic.
* `cephfs`:
    * `kernelMountOptions`: Mount options for kernel mounter. Refer to the [kernel mount options](https://docs.ceph.com/en/latest/man/8/mount.ceph/#options) for more details.
    * `fuseMountOptions`: Mount options for fuse mounter. Refer to the [fuse mount options](https://docs.ceph.com/en/latest/man/8/ceph-fuse/#options) for more details.
