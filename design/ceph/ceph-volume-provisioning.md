# ceph-volume OSD Provisioning

**Targeted for v0.9**

Provisioning OSDs today is done directly by Rook. This needs to be simplified and improved by building
on the functionality provided by the `ceph-volume` tool that is included in the ceph image.

## Legacy Design

As Rook is implemented today, the provisioning has a lot of complexity around:

- Partitioning of devices for bluestore
- Partitioning and configuration of a `metadata` device where the WAL and DB are placed on a different device from the data
- Support for both directories and devices
- Support for bluestore and filestore

Since this is mostly handled by `ceph-volume` now, Rook should replace its own provisioning code and rely on `ceph-volume`.

## ceph-volume Design

`ceph-volume` is a CLI tool included in the `ceph/ceph` image that will be used to configure and run Ceph OSDs.
`ceph-volume` will replace the OSD provisioning mentioned previously in the legacy design.

At a high level this flow remains unchanged from the flow in the [one-osd-per-pod design](dedicated-osd-pod.md#create-new-osds).
No new jobs or pods need to be launched from what we have today. The sequence of events in the OSD provisioning will be the following.

- The cluster CRD specifies what nodes/devices to configure with OSDs
- The operator starts a provisioning job on each node where OSDs are to be configured
- The provisioning job:
  - Detects what devices should be configured
  - Calls `ceph-volume lvm batch` to prepare the OSDs on the node. A single call is made with all of the devices unless more specific settings are included for LVM and partitions.
  - Calls `ceph-volume lvm list` to retrieve the results of the OSD configuration. Store the results in a configmap for the operator to take the next step.
- The operator starts a deployment for each OSD that was provisioned. `rook` is the entrypoint for the container.
  - The configmap with the osd configuration is loaded with info such as ID, FSID, bluestore/filestore, etc
  - `ceph-volume lvm activate` is called to activate the osd, which mounts the config directory such as `/var/lib/ceph/osd-0`, using a tempfs mount. The OSD options such as `--bluestore`, `--filestore`, `OSD_ID`, and `OSD_FSID` are passed to the command as necessary.
  - The OSD daemon is started with `ceph-osd`
  - When `ceph-osd` exits, `rook` will exit and the pod will be restarted by K8s.

### New Features

`ceph-volume` enables rook to expose several new features:

- Multiple OSDs for a single device, which is ideal for NVME devices.
- Configure OSDs on LVM, either consuming the existing LVM or automatically configuring LVM on the raw devices.
- Encrypt the OSD data with dmcrypt

The Cluster CRD will be updated with the following settings to enable these features. All of these settings can be specified
globally if under the `storage` element as in this example. The `config` element can also be specified under individual
nodes or devices.
```yaml
  storage:
    config:
      # whether to encrypt the contents of the OSD with dmcrypt
      encryptedDevice: "true"
      # how many OSDs should be configured on each device. only recommended to be greater than 1 for NVME devices
      osdsPerDevice: 1
      # the class name for the OSD(s) on devices
      crushDeviceClass: ssd
```

If more flexibility is needed that consuming raw devices, LVM or partition names can also be used for specific nodes.
Properties are shown for both bluestore and filestore OSDs.

```yaml
  storage:
    nodes:
    - name: node2
      # OSDs on LVM (open design question: need to re-evaluate the logicalDevice settings when they are implemented after 0.9 and whether they should be under the more general storage node "config" settings)
      logicalDevices:
      # bluestore: the DB, WAL, and Data are on separate LVs
      - db: db_lv1
        wal: wal_lv1
        data: data_lv1
        dbVolumeGroup: db_vg
        walVolumeGroup: wal_vg
        dataVolumeGroup: data_vg
      # bluestore: the DB, WAL, and Data are all on the same LV
      - volume: my_lv1
        volumeGroup: my_vg
      # filestore: data and journal on the same LV
      - data: my_lv2
        dataVolumeGroup: my_vg
      # filestore: data and journal on different LVs
      - data: data_lv3
        dataVolumeGroup: data_vg
        journal: journal_lv3
        journalVolumeGroup: journal_vg
      # devices support both filestore and bluestore configurations based on the "config.storeType" setting at the global, node, or device level
      devices:
      # OSD on a raw device
      - name: sdd
      # OSD on a partition (partition support is new)
      - name: sdf1
      # Multiple OSDs on a high performance device
      - name: nvme01
        config:
          osdsPerDevice: 5
```

The above options for LVM and partitions look very tedious. Questions:

- Is it useful at this level of complexity?
- Is there a simpler way users would configure LVM?
- Do users need all this flexibility? This looks like too many options to maintain.

### Backward compatibility

Rook will need to continue supporting clusters that are running different types of OSDs. All of the v0.8 OSDs must continue running
after Rook is upgraded to v0.9 and beyond, whether they were filestore or bluestore running on directories or devices.

Since `ceph-volume` only supports devices that have **not** been previously configured by Rook:

- Rook will continue to provision OSDs directly when a `directory` is specified in the CRD
  - Support for creating new OSDs on directories will be deprecated. While directories might still be used for test scenarios,
  it's not a mainline scenario. With the legacy design, directories were commonly used on LVM, but LVM is now directly supported.
  In v0.9, support for directories will remain, but documentation will encourage users to provision devices.
- For existing devices configured by Rook, `ceph-volume` will be skipped and the OSDs will be started as previously
- New devices will be provisioned with `ceph-volume`

### Versioning

Rook relies on very recent developments in `ceph-volume` that are not yet available in luminous or mimic releases.
For example, rook needs to run the command:
```
ceph-volume lvm batch --prepare <devices>
```

The `batch` command and the flag `--prepare` have been added recently.
While the latest `ceph-volume` changes will soon be merged to luminous and mimic, Rook needs to know if it is running an image that contains the required functionality.

To detect if `ceph-volume` supports the required options, Rook will run the
command with all the flags that are required. To avoid side effects when testing for the version of `ceph-volume`, no devices
are passed to the `batch` command.
```
ceph-volume lvm batch --prepare
```

- If the flags are supported, `ceph-volume` has an exit code of `0`.
- If the flags are not supported, `ceph-volume` has an exit code of `2`.

Since Rook orchestrates different versions of Ceph, Rook (at least initially) will need to support running images that may not
have the features necessary from `ceph-volume`. When a supported version of `ceph-volume` is not detected, Rook will
execute the legacy code to provision devices.
