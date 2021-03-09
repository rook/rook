# Cluster Updates

## Background
Currently, a Rook admin can declare how they want their cluster deployed by specifying values in the [Cluster CRD](../Documentation/ceph-cluster-crd.md).
However, after a cluster has been initially declared and deployed, it is not currently possible to update the Cluster CRD and have those desired changes reflected in the actual cluster state.
This document will describe a design for how cluster updating can be implemented, along with considerations, trade-offs, and a suggested scope of work.

## Design Overview
As previously mentioned, the interface for a user who wants to update their cluster will be the Cluster CRD.
To specify changes to a Rook cluster, the user could run a command like the following:
```console
kubectl -n rook-ceph edit cluster.ceph.rook.io rook-ceph
```
This will bring up a text editor with the current value of the cluster CRD.
After their desired edits are made, for instance to add a new storage node, they will save and exit the editor.
Of course, it is also possible to update a cluster CRD via the Kubernetes API instead of `kubectl`.

This will trigger an update of the CRD object, which the operator is already subscribed to events for.
The update event is provided both the new and old cluster objects, making it possible to perform a diff between desired and actual state.
Once the difference is calculated, the operator will begin to bring actual state in alignment with desired state by performing similar operations to what it does to create a cluster in the first place.
Controllers, pod templates, config maps, etc. will be updated and configured with the end result of the Rook cluster pods and state representing the users desired cluster state.

The most common case for updating a Rook cluster will be to add and remove storage resources.
This will essentially alter the number of OSDs in the cluster which will cause data rebalancing and migration.
Therefore, updating storage resources should be performed by the operator with special consideration as to not degrade cluster performance and health beyond acceptable levels.

## Design Details
### Cluster CRD
The Cluster CRD has many fields, but not all of them will be updatable (i.e., the operator will not attempt to make any changes to the cluster for updates to some fields).
#### Supported Fields
The following fields will be **supported** for updates:
* `mon`: Ceph mon specific settings can be changed.
  * `count`: The number of monitors can be updated and the operator will ensure that as monitors are scaled up or down the cluster remains in quorum.
  * `allowMultiplePerNode`: The policy to allow multiple mons to be placed on one node can be toggled.
* `deviceFilter`: The regex filter for devices allowed to be used for storage can be updated and OSDs will be added or removed to match the new filter pattern.
* `devicePathFilter`: The regex filter for paths of devices allowed to be used for storage can be updated and OSDs will be added or removed to match the new filter pattern.
* `useAllDevices`: If this value is updated to `true`, then OSDs will be added to start using all devices on nodes.
However, if this value is updated to `false`, the operator will only allow OSDs to be removed if there is a value set for `deviceFilter`.
This is to prevent an unintentional action by the user that would effectively remove all data in the cluster.
* `useAllNodes`: This value will be treated similarly to `useAllDevices`.
Updating it to `true` is a safe action as it will add more nodes and their storage to the cluster, but updating it to `false` is not always a safe action.
If there are no individual nodes listed under the `nodes` field, then updating this field to `false` will not be allowed.
* `resources`: The CPU and memory limits can be dynamically updated.
* `placement`: The placement of daemons across the cluster can be updated, but it is dependent on the specific daemon.
For example, monitors can dynamically update their placement as part of their ongoing health checks.
OSDs can not update their placement at all since they have data gravity that is tied to specific nodes.
Other daemons can decide when and how to update their placement, for example doing nothing for current pods and only honoring new placement settings for new pods.
* `nodes`: Specific storage nodes can be added and removed, as well as additional properties on the individual nodes that have not already been described above:
  * `devices`:  The list of devices to use for storage can have entries added and removed.
  * `directories`: The list of directories to use for storage can also be updated.

#### Unsupported Fields
All other properties not listed above are **not supported** for runtime updates.
Some particular unsupported fields to note:
* `dataDirHostPath`: Once the local host directory for storing cluster metadata and config has been set and populated, migrating it to a new location is not supported.
* `hostNetwork`: After the cluster has been initialized to either use host networking or pod networking, the value can not be changed.
Changing this value dynamically would very likely cause a difficult to support transition period while pods are transferring between networks and would certainly impact cluster health.

#### Validation
It is in the user's best interests to provide early feedback if they have made an update to their Cluster CRD that is invalid or not supported.
Along with [issue 1000](https://github.com/rook/rook/issues/1000), we should use the Kubernetes CRD validation feature to verify any changes to the Cluster CRD and provide helpful error messages in the case that their update can not be fulfilled.

#### Device Name Changes
It is important to remember that [Linux device names can change across reboots](https://wiki.archlinux.org/index.php/persistent_block_device_naming).
Because of this, we need to be very careful when determining whether it is a safe operation to remove an OSD.
We need to be absolutely sure that the user really intended to remove the OSD from a device, as opposed to the device name randomly changing and becoming out of the device filter or list.

What is especially challenging here is that before the initial deployment of OSDs onto a node, which creates the UUIDs for each device, there is no known consistent and user friendly way to specify devices.
A lot of environments do **not** have labels, IDs, UUIDs, etc. for their devices at first boot and the only way to address them is by device name, such as `sda`.
This is unfortunate because it is a volatile identifier.
Some environments do have IDs at first boot and we should consider allowing users to specify devices by those IDs instead of names in the near future.
That effort is being tracked by [issue 1228](https://github.com/rook/rook/issues/1228).

The main approach that will be taken to solve this issue is to always compare the device UUID from a node's saved OSD config map against the device UUIDs of the current set of device names.
If the two do not match, then it is not a safe operation to remove the OSD from the device.
Let's walk through a couple simple scenarios to illustrate this approach:

**NOT SAFE: Device name has changed, but filter has not been updated by the user:**
* User initially specifies `sda` via device filter or list. Rook configures `sda` and gets an OSD up and running.
* The node reboots which causes the OSD pod to restart.
* The filter still specifies `sda`, but the device has changed its name to `sdb`.  The device is now out of the filter.
* We look at the node's saved OSD config and see that we originally set up `sda` with device UUID `wxyz-1234`.
* The user's filter still says to use `sda`, so going by the saved config and not what the current devices names are, we know that the old `sda` (device UUID `wxyz-1234`), which is now `sdb` should NOT be removed.

**SAFE: User has updated the filter and the device name has not changed:**
* User initially specifies `sda` via device filter or list. Rook configures `sda` and gets an OSD up and running.
* User updates the Cluster CRD to change the device filter or list to now be `sdb`.
* The OSD pod restarts and when it comes back up it sees that the previously configured `sda` is no longer in the filter.
* The pod checks the device UUID of `sda` in its saved config and compares that to the device UUID of the current `sda`.
* The two match, so the pod knows it's a safe (user intended) operation to remove the OSD from `sda`.

### Orchestration
When the operator receives an event that the Cluster CRD has been updated, it will need to perform some orchestration in order to bring actual state of the cluster in agreement with the desired state.
For example, when `mon.count` is updated, the operator will add or remove a single monitor at a time, ensuring that quorum is restored before moving onto the next monitor.
Updates to the storage spec for the cluster require even more careful consideration and management by the operator, which will be discussed in this section.

First and foremost, changes to the cluster state should not be carried out when the cluster is not in a healthy state.
The operator should wait until cluster health is restored until any orchestration is carried out.

It is important to remember that a single OSD pod can contain multiple OSD processes and that the operator itself does not have detailed knowledge of the storage resources of each node.
More specifically, the devices that can be used for storage (e.g., match `deviceFilter`) is not known until the OSD pod has been started on a given node.

As mentioned previously, it is recommended to make storage changes to the cluster one OSD at a time.
Therefore, the operator and the OSD pods will need to coordinate their efforts in order to adhere to this guidance.
When a cluster update event is received by the operator, it will work on a node by node basis, ensuring all storage updates are completed by the OSD pod for that node before moving to the next.

When an OSD pod starts up and has completed its device discovery, it will need to perform a diff of the desired storage against the actual storage that is currently included in the cluster.
This diff will determine the set of OSD instances that need to be removed or added within the pod.
Fortunately, the OSD pod start up is already idempotent and already handles new storage additions, so the remaining work will be the following:
* Safely removing existing OSDs from the cluster
* Waiting for data migration to complete and all placement groups to become clean
* Signaling to the operator that the pod has completed its storage updates

We should consider an implementation that allows the OSD pod to refresh it's set of OSDs without restarting the entire pod, but since the replication controller's pod template spec needs to be updated by the operator in order to convey this information to the pod, we may need to live with restarting the pod either way.
Remember that this will be done one node at a time to mitigate impact to cluster health.

Also, other types of update operations to the cluster (e.g., software upgrade) should be blocked while a cluster update is ongoing.

#### Cluster CRD Status
The Cluster CRD status will be kept up to date by the operator so the user has some insight into the process being carried out.
While the operator is carrying out an update to the cluster, the Cluster CRD `status` will be set to `updating`.
If there are any errors during the process, the `message` field will be updated with a specific reason for the failure.
We should also update documentation for our users with easy commands to query the status and message fields so they can get more information easily.

#### Operator and OSD pod communication
As mentioned previously, the OSD pods need to communicate to the operator when they are done orchestrating their local OSD instance changes.
To make this effort more resilient and tolerant of operator restarts, this effort should be able to be resumed.
For example, if the operator restarts while an OSD pod is draining OSDs, the operator should **not** start telling other OSD pods to do work.

The OSDs and operator will jointly maintain a config map to track the status of storage update operations within the cluster.
When the operator initially requests an OSD pod to compute its storage diff, it will update a config map with an entry for the OSD containing a status of `computingDiff` and a current timestamp.
When the OSD pod has finished computation and started orchestrating changes, it will update the entry with a status of `orchestrating` and a current timestamp.
Finally, when the pod has finished, it will update the entry with `completed` and a current timestamp again, letting the operator know it is safe to move onto the next node.

If the operator is restarted during this flow, it will look in the config map for any OSD pod that is not in the `completed` state.
If it finds any, then it will wait until they are completed before moving onto another node.
This approach will ensure that only 1 OSD pod is performing changes at a time.
Note that this approach can also be used to ask an OSD pod to compute changes without having to restart the pod needlessly.
If the OSD pods are watching the config map for changes, then they can compute a diff upon request of the operator.

### Storage Update Process
This section covers the general sequence for updating storage resources and outlines important considerations for cluster health.
Before any changes begin, we will temporarily disable scrubbing of placement groups (the process of verifying data integrity of stored objects) to maximize cluster resources that can go to both client I/O and recovery I/O for data migration:
```console
ceph osd set noscrub
ceph osd set nodeep-scrub
```

Some [Ceph documentation](https://access.redhat.com/documentation/en-us/red_hat_ceph_storage/2/html/administration_guide/adding_and_removing_osd_nodes#recommendations) also recommends limiting backfill and recovery work while storage is being added or removed.
The intent is to maximize client I/O while sacrificing throughput of data migration.
I do not believe this is strictly necessary and at this point I would prefer to not limit recovery work in the hopes of finishing data migrations as quickly as possible.
I suspect that most cluster administrators would not be removing storage when the cluster is under heavy load in the first place.
This trade-off can be revisited if we see unacceptable performance impact.

#### Adding Storage
As mentioned previously, we will add one OSD at a time in order to allow the cluster to rebalance itself in a controlled manner and to avoid getting into a situation where there is an unacceptable amount of churn and thrashing.
Adding a new OSD is fairly simple since the OSD pod logic already supports it:
* If the entire node is being added, ensure the node is added to the CRUSH map: `ceph osd crush add-bucket {bucket-name} {type}`
* For each OSD:
  * Register, format, add OSD to the crush map and start the OSD process like normal
  * Wait for all placement groups to reach `active+clean` state, meaning data migration is complete.

#### Removing Storage
Removing storage is a more involved process and it will also be done one OSD at a time to ensure the cluster returns to a clean state.
Of special note for removing storage is that a check should be performed to ensure that the cluster has enough remaining storage to recover (backfill) the entire set of objects from the OSD that is being removed.
If the cluster does not have enough space for this (e.g., it would hit the `full` ratio), then the removal should not proceed.

For each OSD to remove, the following steps should be performed:
* reweight the OSD to 0.0 with `ceph osd crush reweight osd.<id> 0.0`, which will trigger data migration from the OSD.
* wait for all data to finish migrating from the OSD, meaning all placement groups return to the `active+clean` state
* mark the OSD as `out` with `ceph osd out osd.<id>`
* stop the OSD process and remove it from monitoring
* remove the OSD from the CRUSH map: `ceph osd crush remove osd.<id>`
* delete the OSD's auth info: `ceph auth del osd.<id>`
* delete the OSD from the cluster: `ceph osd rm osd.<id>`
* delete the OSD directory from local storage (if using `dataDirHostPath`): `rm -fr /var/lib/rook/<osdID>`

If the entire node is being removed, ensure that the host node is also removed from the CRUSH map:
```console
$ ceph osd crush rm <host-bucket-name>
```

#### Completion
After all storage updates are completed, both additions and removals, then we can once again enable scrubbing:
```console
ceph osd unset noscrub
ceph osd unset nodeep-scrub
```

### Placement Groups
The number of placement groups in the cluster compared to the number of OSDs is a difficult trade-off without knowing the user's intent for future cluster growth.
The general rule of thumb is that you want around 100 PGs per OSD.
With less than that, you have potentially unbalanced distribution of data with certain OSDs storing more than others.
With more PGs than that, you have increased overhead in the cluster because more OSDs need to coordinate with each other, impacting performance and reliability.

It's important to note that **shrinking** placement group count (merging) is still **not supported** in Ceph.
Therefore, you can only increase the number of placement groups (splitting) over time.

If the cluster grows such that we have too few placement groups per OSD, then we can consider increasing the number of PGs in the cluster by incrementing the `pg_num` and `pgp_num` for each storage pool.
Similar to adding new OSDs, this increase of PGs should be done incrementally and in a coordinated fashion to avoid degrading performance significantly in the cluster.

Placement group management will be tracked in further detail in [issue 560](https://github.com/rook/rook/issues/560).

## Scope
The implementation of the design described in this document could be done in a phased approach in order to get critical features out sooner.
One proposal for implementation phases would be:
1. **Simple add storage**: Storage resources can be added to the Cluster CRD and extremely minimal orchestration would be performed to coordinate the storage changes.
Cluster performance impact would not be ideal but may be tolerable for many scenarios, and Rook clusters would then have dynamic storage capabilities.
1. **Simple remove storage**:  Similar to the simple adding of storage, storage resources can be removed from the Cluster CRD with minimal orchestration.
1. **Dynamic storage orchestration**: The more careful orchestration of storage changes would be implemented, with the operator and OSD pods coordinating across the cluster to slowly ramp up/down storage changes with minimal impact to cluster performance.
1. **Non-storage cluster field updates**: All other properties in the cluster CRD supported for updates will be implemented (e.g., `mon`, `resources`, etc.).
1. **Placement Group updates**: Placement group counts will be updated over time as the cluster grows in order to optimize cluster performance.
