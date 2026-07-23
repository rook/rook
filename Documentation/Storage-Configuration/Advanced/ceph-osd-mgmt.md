---
title: Ceph OSD Management
---

Ceph Object Storage Daemons (OSDs) are the heart and soul of the Ceph storage platform.
Each OSD manages a local device and together they provide the distributed storage. Rook will automate creation and management of OSDs to hide the complexity
based on the desired state in the CephCluster CR as much as possible. This guide will walk through some of the scenarios
to configure OSDs where more configuration may be required.

## OSD Health

The [rook-ceph-tools pod](../../Troubleshooting/ceph-toolbox.md) provides a simple environment to run Ceph tools. The `ceph` commands
mentioned in this document should be run from the toolbox.

Once the is created, connect to the pod to execute the `ceph` commands to analyze the health of the cluster,
in particular the OSDs and placement groups (PGs). Some common commands to analyze OSDs include:

```console
ceph status
ceph osd tree
ceph osd status
ceph osd df
ceph osd utilization
```

```console
kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}') -- bash
```

## Add an OSD

The [QuickStart Guide](../../Getting-Started/quickstart.md) will provide the basic steps to create a cluster and start some OSDs. For more details on the OSD
settings also see the [Cluster CRD](../../CRDs/Cluster/ceph-cluster-crd.md) documentation. If you are not seeing OSDs created, see the [Ceph Troubleshooting Guide](../../Troubleshooting/ceph-common-issues.md).

To add more OSDs, Rook will automatically watch for new nodes and devices being added to your cluster.
If they match the filters or other settings in the `storage` section of the cluster CR, the operator
will create new OSDs.

## Add an OSD on a PVC

In more dynamic environments where storage can be dynamically provisioned with a raw block storage provider, the OSDs can be backed
by PVCs. See the `storageClassDeviceSets` documentation in the [Cluster CRD](../../CRDs/Cluster/ceph-cluster-crd.md#storage-class-device-sets) topic.

To add more OSDs, you can either increase the `count` of the OSDs in an existing device set or you can
add more device sets to the cluster CR. The operator will then automatically create new OSDs according
to the updated cluster CR.

## Remove an OSD

To remove an OSD due to a failed disk or other re-configuration, consider the following to ensure the health of the data
through the removal process:

* Confirm you will have enough space on your cluster after removing your OSDs to properly handle the deletion
* Confirm the remaining OSDs and their placement groups (PGs) are healthy in order to handle the rebalancing of the data
* Do not remove too many OSDs at once
* Wait for rebalancing between removing multiple OSDs

If all the PGs are `active+clean` and there are no warnings about being low on space, this means the data is fully replicated
and it is safe to proceed. If an OSD is failing, the PGs will not be perfectly clean and you will need to proceed anyway.

### Host-based cluster

Update your CephCluster CR. Depending on your CR settings, you may need to remove the device from the list or update the device filter.
If you are using `useAllDevices: true`, no change to the CR is necessary.

!!! important
    **On host-based clusters, you may need to stop the Rook Operator while performing OSD
    removal steps in order to prevent Rook from detecting the old OSD and trying to re-create it before the disk is wiped or removed.**

To stop the Rook Operator, run:

```console
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=0
```

You must perform steps below to (1) purge the OSD and either (2.a) delete the underlying data or
(2.b)replace the disk before starting the Rook Operator again.

Once you have done that, you can start the Rook operator again with:

```console
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=1
```

### PVC-based cluster

To reduce the storage in your cluster or remove a failed OSD on a PVC:

1. Shrink the number of OSDs in the `storageClassDeviceSets` in the CephCluster CR. If you have multiple device sets,
    you may need to change the index of `0` in this example path.
    * `kubectl -n rook-ceph patch CephCluster rook-ceph --type=json -p '[{"op": "replace", "path": "/spec/storage/storageClassDeviceSets/0/count", "value":<desired number>}]'`
    * Reduce the `count` of the OSDs to the desired number. Rook will not take any action to automatically remove the extra OSD(s).
2. Identify the PVC that belongs to the OSD that is failed or otherwise being removed.
    * `kubectl -n rook-ceph get pvc -l ceph.rook.io/DeviceSet=<deviceSet>`
3. Identify the OSD you desire to remove.
    * The OSD assigned to the PVC can be found in the labels on the PVC
    * `kubectl -n rook-ceph get pod -l ceph.rook.io/pvc=<orphaned-pvc> -o yaml | grep ceph-osd-id`
    * For example, this might return: `ceph-osd-id: "0"`
    * Remember the OSD ID for purging the OSD below

If you later increase the count in the device set, note that the operator will create PVCs with the highest index
that is not currently in use by existing OSD PVCs.

### Confirm the OSD is down

If you want to remove an unhealthy OSD, the osd pod may be in an error state such as `CrashLoopBackoff` or the `ceph` commands
in the toolbox may show which OSD is `down`. If you want to remove a healthy OSD, you should run the following commands:

```console
$ kubectl -n rook-ceph scale deployment rook-ceph-osd-<ID> --replicas=0
# Inside the toolbox
$ ceph osd down osd.<ID>
```

### Purge the OSD with kubectl

!!! note
    The `rook-ceph` kubectl plugin must be [installed](https://github.com/rook/kubectl-rook-ceph#install)

```bash
kubectl rook-ceph rook purge-osd 0 --force

# 2022-09-14 08:58:28.888431 I | rookcmd: starting Rook v1.10.0-alpha.0.164.gcb73f728c with arguments 'rook ceph osd remove --osd-ids=0 --force-osd-removal=true'
# 2022-09-14 08:58:28.889217 I | rookcmd: flag values: --force-osd-removal=true, --help=false, --log-level=INFO, --operator-image=, --osd-ids=0, --preserve-pvc=false, --service-account=
# 2022-09-14 08:58:28.889582 I | op-mon: parsing mon endpoints: b=10.106.118.240:6789
# 2022-09-14 08:58:28.898898 I | cephclient: writing config file /var/lib/rook/rook-ceph/rook-ceph.config
# 2022-09-14 08:58:28.899567 I | cephclient: generated admin config in /var/lib/rook/rook-ceph
# 2022-09-14 08:58:29.421345 I | cephosd: validating status of osd.0
---
```

### Purge the OSD with a Job

OSD removal can be automated with the example found in the [rook-ceph-purge-osd job](https://github.com/rook/rook/blob/master/deploy/examples/osd-purge.yaml).
In the osd-purge.yaml, change the `<OSD-IDs>` to the ID(s) of the OSDs you want to remove.

1. Run the job: `kubectl create -f osd-purge.yaml`
2. When the job is completed, review the logs to ensure success: `kubectl -n rook-ceph logs -l app=rook-ceph-purge-osd`
3. When finished, you can delete the job: `kubectl delete -f osd-purge.yaml`

If you want to remove OSDs by hand, continue with the following sections. However, we recommend you use the above-mentioned steps to avoid operation errors.

### Purge the OSD manually

If the OSD purge job fails or you need fine-grained control of the removal, here are the individual commands that can be run from the toolbox.

1. Detach the OSD PVC from Rook
    * `kubectl -n rook-ceph label pvc <orphaned-pvc> ceph.rook.io/DeviceSetPVCId-`
2. Mark the OSD as `out` if not already marked as such by Ceph. This signals Ceph to start moving (backfilling) the data that was on that OSD to another OSD.
    * `ceph osd out osd.<ID>` (for example if the OSD ID is 23 this would be `ceph osd out osd.23`)
3. Wait for the data to finish backfilling to other OSDs.
    * `ceph status` will indicate the backfilling is done when all of the PGs are `active+clean`. If desired, it's safe to remove the disk after that.
4. Remove the OSD from the Ceph cluster
    * `ceph osd purge <ID> --yes-i-really-mean-it`
5. Verify the OSD is removed from the node in the CRUSH map
    * `ceph osd tree`

The operator can automatically remove OSD deployments that are considered "safe-to-destroy" by Ceph.
After the steps above, the OSD will be considered safe to remove since the data has all been moved
to other OSDs. But this will only be done automatically by the operator if you have this setting in the cluster CR:

```yaml
removeOSDsIfOutAndSafeToRemove: true
```

Otherwise, you will need to delete the deployment directly:

```console
kubectl delete deployment -n rook-ceph rook-ceph-osd-<ID>
```

In PVC-based cluster, remove the orphaned PVC, if necessary.

### Delete the underlying data

If you want to clean the device where the OSD was running, see in the instructions to
wipe a disk on the [Cleaning up a Cluster](../ceph-teardown.md#delete-the-data-on-hosts) topic.

## Replace an OSD

This procedure replaces a failed disk while the OSD keeps its original ID and its place in the CRUSH map, so Ceph backfills only the data that was on the failed disk rather than rebalancing the whole cluster. The replacement is triggered by an annotation on the OSD deployment.

!!! note
    This applies to **host-based clusters** only. To replace a disk backing a PVC-based OSD, follow [Remove an OSD](#remove-an-osd) instead and let the operator re-create it.

### How it works

The replacement runs in five stages, alternating between what you do and what Rook does:

1. You annotate the OSD deployment with `osd.rook.io/replace` to trigger the replacement.
2. Rook marks the OSD `out` and waits until Ceph reports it safe to destroy.
3. Rook destroys the OSD and annotates the deployment `osd.rook.io/replace-ready-for-swap`.
4. You physically swap the failed disk.
5. Rook detects the new disk and provisions the replacement OSD with the same OSD ID.

### Before you begin

**Cluster settings.** The feature needs the OSD health check enabled and automatic OSD removal disabled in the CephCluster CR. Both are the defaults, so check them only if you have changed them:

- `spec.healthCheck.daemonHealth.osd.disabled: false`
- `spec.removeOSDsIfOutAndSafeToRemove: false`

**Operator discovery daemon (recommended).** After you swap the disk, Rook provisions the replacement on its next reconcile, and the operator's discovery daemon triggers that reconcile automatically once the new disk appears. This is a Rook **operator** setting and is **disabled by default**; enable it by setting `enableDiscoveryDaemon: true` in the [operator Helm chart](../../Helm-Charts/operator-chart.md). If you leave it disabled, trigger a reconcile yourself after the swap, for example by restarting the operator pod.

**Device selection.** After the swap, Rook provisions the new disk only if it matches the device selection in your CephCluster CR (see [Storage Selection Settings](../../CRDs/Cluster/ceph-cluster-crd.md#storage-selection-settings)):

- **Matched automatically:** `useAllDevices: true`, a `deviceFilter` that matches by kernel name, or a `/dev/disk/by-path/...` reference when the new disk is in the same physical slot.
- **Needs a CR update:** a `/dev/disk/by-id/...` or `/dev/disk/by-uuid/...` reference, an explicit device name, or a `by-path` reference when the disk moves to a different slot. These identify the old disk, so edit the CephCluster CR to point at the new one.

**Supported layouts:** all types of OSDs for host-based clusters, including shared-metadata OSDs.

!!! warning
    This procedure replaces a single **data** disk. It does not replace a **metadata device** shared by several OSDs. If the disk that failed is the shared metadata device, this procedure does not apply.

### Step 1: Trigger the replacement

**In the following example, OSD with ID 5 is being replaced. Make sure to replace `5` with your OSD ID.**

First, annotate the OSD's deployment for replacement. The ID in the annotation value must match the OSD ID: this guards against destroying the wrong OSD.

```console
kubectl -n rook-ceph annotate deployment rook-ceph-osd-5 \
  osd.rook.io/replace=yes-really-replace-osd-5
```

The OSD does not have to have failed. You can replace a healthy OSD the same way, for example to move it to new hardware; Rook drains it before destroying it, so no data is at risk of loss.

### Step 2: Wait until the disk is ready to swap

Rook validates the request, then drains and destroys the OSD. When the OSD is destroyed and the disk is safe to pull, Rook adds the `osd.rook.io/replace-ready-for-swap` annotation. Watch for it:

```console
kubectl -n rook-ceph get deployment rook-ceph-osd-5 \
  -o jsonpath='{.metadata.annotations.osd\.rook\.io/replace-ready-for-swap}{"\n"}'
# prints "true" once the disk is safe to pull
```

While the OSD drains, track how much data it still holds by running `ceph osd df` from the [toolbox](../../Troubleshooting/ceph-toolbox.md); the OSD is safe to destroy once its usage approaches zero:

```console
ceph osd df osd.5
```

If the annotation does not appear, either the request failed validation and the replacement never started, or the OSD is still draining (a large OSD can take hours or days, and there is no timeout). Check the operator's warning and error logs for a rejected request:

```console
kubectl -n rook-ceph logs deploy/rook-ceph-operator | grep -i "replacement"
```

### Step 3: Swap the disk

!!! important
    Pull the disk only **after** the `osd.rook.io/replace-ready-for-swap` annotation appears. Until the swap, the failed disk still carries its Ceph signature, which is both what reserves the OSD ID for the new disk and how Rook detects the swap. Swapping before the annotation appears causes the new disk to be provisioned as a brand-new OSD with a **different** ID.

Physically remove the failed disk and insert the replacement. You can do this at any time after the annotation appears, minutes or days later. The new disk must go into the **same host**. It does not have to be the same physical slot unless you select devices by a stable path (see [Before you begin](#before-you-begin)).

### Step 4: Verify the replacement completed

Rook detects the new disk on its next reconcile and brings the OSD back up with its original ID `5`, reusing the surviving DB volume for a shared-metadata OSD. If the discovery daemon is disabled, Rook will not notice the new disk on its own. Instead trigger a reconcile by restarting the operator pod, so you do not wait indefinitely.

Confirm the deployment is back and running:

```console
kubectl -n rook-ceph get deployment rook-ceph-osd-5
# READY should be 1/1
```

The replacement is complete once OSD `5` is `up` and `in`. Ceph then backfills the data onto the new disk; monitor recovery from the [Ceph dashboard](../Monitoring/ceph-dashboard.md) or the toolbox until all PGs are `active+clean`.

## OSD Migration

Ceph does not support changing certain settings on existing OSDs. To support changing these settings on an OSD, the OSD must be destroyed and re-created with the new settings. Rook will automate this by migrating only one OSD at a time. The operator waits for the data to rebalance (PGs to become `active+clean`) before migrating the next OSD. This ensures that there is no data loss. Refer to the [OSD migration](https://github.com/rook/rook/blob/master/design/ceph/osd-migration.md) design doc for more information. 

The following scenarios are supported for OSD migration:

- Enable or disable OSD encryption for existing PVC-based OSDs by changing the `encrypted` setting under the `storageClassDeviceSets`

For example:

```yaml
storage:
    migration:
        confirmation: "yes-really-migrate-osds" 
    storageClassDeviceSets:
        - name: set1
          count: 3
          encrypted: true  # change to true or false based on whether encryption needs to enable or disabled.
```

Details about the migration status can be found under the cephCluster `status.storage.osd.migrationStatus.pending` field which shows the total number of OSDs that are pending migration.

!!! note
    Performance of the cluster might be impacted during data rebalancing while OSDs are being migrated.
