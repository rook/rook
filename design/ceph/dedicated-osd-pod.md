# Run OSD in its own Pod

## TL;DR

A one-OSD-per-Pod placement should be implemented to improve reliability and resource efficiency for Ceph OSD daemon.

## Background

Currently in Rook 0.7, Rook Operator starts a ReplicaSet to run [`rook osd`](https://github.com/rook/rook/blob/master/cmd/rook/osd.go) command (hereafter referred to as `OSD Provisioner`)  on each storage node. The ReplicaSet has just one replica. `OSD Provisioner` scans and prepares devices, creates OSD IDs and data directories or devices, generates Ceph configuration. At last, `OSD Provisioner` starts all Ceph OSD, i.e. `ceph-osd`, daemons in foreground and tracks `ceph-osd` processes.

As observed, all Ceph OSDs are running in the same Pod.

## Limitations

The limitations of current design are:

- Reliability issue. One Pod for all OSDs doesn't have the highest reliability nor efficiency. If the Pod is deleted, accidentally or during maintenance, all OSDs are down till the ReplicaSet restart.
- Efficiency issue. Resource limits cannot be set effectively on the OSDs since the number of osds per in the pod could vary from node to node. The operator cannot make decisions about the topology because it doesn't know in advance what devices are available on the nodes.
- Tight Ceph coupling. The monolithic device discovery and provisioning code cannot be reused for other backends.
- Process management issue. Rook's process management is very simple. Using Kubernetes pod management is much more reliable.


A more comprehensive discussions can be found at [this issue](https://github.com/rook/rook/issues/1341).

## Terms

- Device Discovery. A DaemonSet that discovers unformatted devices on the host. The DaemonSet populates a per node Raw Device Configmap with device information. The daemonSet is running on nodes that are labelled as storage nodes. The DaemonSet can start independently of Rook Operator. Device Discovery is storage backend agnostic.

- Device Provisioner. A Pod that is given device or directory paths upon start and make backend specific storage types. For instance, the provisioner prepares OSDs for Ceph backend. It is a Kubernetes batch job and exits after the devices are prepared.

## Proposal

We propose the following change to address the limitations.


### Create new OSDs
| Sequence |Rook Operator | Device Discovery  | Device Provisioner   | Ceph OSD Deployment |
|---|---|---|---|---|
| 0  |  | Start on labeled storage nodes, discover unformatted devices and store device information in per node Raw Device Configmap  |   |
| 1  | Read devices and node filters from cluster CRD |   |   |
| 2  | parse Raw Device Configmap, extract nodes and device paths and filters them based on cluster CRD, and create an Device Provisioner deployment for each device  || | |
| 3  | Watch device provisioning Configmap | | Prepare OSDs, Persist OSD ID, datapath, and node info in a per node device provisioning Configmap | |
| 4  | Detect device provisioning Configmap change, parse Configmap, extract OSD info, construct OSD Pod command and arg | | | |
| 5  | Create one deployment per OSD | | |Start `ceph-osd` Daemon one Pod per device |


This change addresses the above limitations in the follow ways:
- High reliability. Each `ceph-osd` daemon runs its own Pod, their restart and upgrade are by Kubernetes controllers. Upgrading Device Provisioner Pod no longer restarts `ceph-osd` daemons.
- More efficient resource requests. Once Device Discovery detects all devices, Rook Operator is informed of the topology and assigns appropriate resources to each Ceph OSD deployment.
- Reusable. Device discovery can be used for other storage backends.


### Detailed Device Discovery Process

Each `Device Discovery` DaemonSet walks through device trees to unformatted block devices and stores the device information in a per node `Raw Device Configmap`.

A sample of `Raw Device Configmap` from Node `node1` is as the following:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: rook-system
  name: node1-raw-devices
data:
   devices
   -  device-path: /dev/disk/by-id/scsi-dead     # persistent device path
      size: 214748364800                         # size in byte
      rotational: 0                              # 0 for ssd/nvme, 1 for hdd, based on reading from sysfs
      extra: '{ "vendor": "X", "model": "Y" }'   # extra information under sysfs about the device in json, such as vendor/model, scsi level, target info, etc.
   -  device-path: /dev/disk/by-id/scsi-beef     # persistent device path
      size: 214748364800                         # size in byte
      rotational: 1                              # 0 for ssd/nvme, 1 for hdd, based on reading from sysfs
```

### Discussions

It is expected Device Discovery will be merged into Rook Operator once local PVs are supported in Rook Cluster CRD. Rook Operator can infer the device topology from local PV Configmaps. However, as long as raw devices or directories are still in use, a dedicated Device Discovery Pod is still needed.

If the storage nodes are also compute nodes, it is possible that dynamically attached and unformatted devices to those nodes are discovered by Device Discovery DaemonSet. To avoid this race condition, admin can choose to use separate device tree directories: one for devices used for Rook and the other for compute. Or the Cluster CRD should explicitly identify which devices should be used for Rook.

Alternatively, since `rook agent` is currently running as a DaemonSet on all nodes, it is conceivable to make `rook agent` to poll devices and update device orchestration Configmap. This approach, however, needs to give `rook agent` the privilege to modify Configmaps. Moreover, `Device Discovery` Pod doesn't need privileged mode, host network, or write access to hosts' `/dev` directory, all of which are required by `rook agent`.

## Impact

- Security. Device Provisioner Pod needs privilege to access Configmaps but Ceph OSD Pod don't need to access Kubernetes resources and thus don't need any RBAC rules.

- Rook Operator. Rook Operator watches two Configmaps: the raw device Configmaps that created by Device Discovery Pod and storage specific device provisioning Configmaps that are created by Device Provisioner Pod. For raw device Configmap, Operator creates storage specific device provisioner deployment to prepare these devices. For device provisioning Configmaps, Operator creates storage specific daemon deployment (e.g. Ceph OSD Daemon deployments) with the device information in Configmaps and resource information in Cluster CRD.

- Device Discovery. It is a new long running process in a DaemonSet that runs on each node that has matching labels. It discovers storage devices on the nodes and populates the raw devices Configmaps.

- Device Provisioner. Device Provisioner becomes a batch job, it no longer exec Ceph OSD daemon.

- Ceph OSD Daemon. `ceph-osd` is no longer exec'ed by Device Provisioner, it becomes the Pod entrypoint.

- Ceph OSD Pod naming. Rook Operator creates Ceph OSD Pod metadata using cluster name, node name, and OSD ID.
