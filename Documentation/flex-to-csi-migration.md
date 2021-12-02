---
title: Flex Migration
weight: 11900
indent: true
---

# Flex to CSI Migration

In Rook v1.8, the Flex driver has been deprecated. Before updating to v1.8, any Flex volumes created in previous versions of Rook will need to be converted to Ceph-CSI volumes.

The tool [persistent-volume-migrator](https://github.com/ceph/persistent-volume-migrator) will help automate migration of Flex rbd volumes to Ceph-CSI volumes.

> **Note** Migration of CephFS FlexVolumes is not supported for now.

## Migration Preparation

1. Rook v1.7.9 is required. If you have a previous version of Rook running, follow the [upgrade guide](https://rook.io/docs/rook/v1.7/ceph-upgrade.html) to upgrade from previous releases until on v1.7.9.
2. Enable the CSI driver if not already enabled. See the [operator settings](https://github.com/rook/rook/blob/release-1.7/cluster/examples/kubernetes/ceph/operator.yaml#L29-L32) such as `ROOK_CSI_ENABLE_RBD`.
3. Confirm the Rook-Ceph cluster is healthy (`ceph status` shows `health: OK`)
4. Create the CSI storage class to which you want to migrate
5. Create the migrator pod
   1) `kubectl create -f cluster/examples/kubernetes/ceph/flex-migrator.yaml`

**NOTE**: The Migration procedure will come with a downtime as we need to scale down the applications using the volumes before migration.

## Migrate a PVC

1. Stop the application pods that are consuming the flex volume(s) that need to be converted. For example, scale the application deployment down to 0: `kubectl scale --replicas=0 deploy/<name>`
2. Connect to migration pod
   1. `migration_pod=$(kubectl -n rook-ceph get pod -l app=rook-ceph-migrator -o jsonpath='{.items[*].metadata.name}')`

   2. `kubectl -n rook-ceph exec -it "$migration_pod" -- sh`
3. Run the tool to migrate the PVC. See the [section below](#migration-tool-options) for more details on the options and the sample output.
   1. `pv-migrator --pvc=<pvc-name> --pvc-ns=<pvc-namespace> --destination-sc=<storageclass-name-to-migrate-in>`
4. Start the application pods which were stopped in step 1. For example, scale the application deployment back up: `kubectl scale --replicas=1 deploy/<name>`

## Migration Tool Options

These are the options for converting a single PVC. For more options, see the [tool documentation](https://github.com/ceph/persistent-volume-migrator), for example to convert all PVCs automatically that belong to the same storage class.

1. `--pvc`: **required**: name of the pvc to migrate
2. `--pvc-ns`: **required**: namespace in which the target PVC is present.
3. `--destination-sc`: **required**: name of the ceph-csi storage class in which you want to migrate.
4. `--rook-ns`: **optional** namespace where the rook operator is running. **default: rook-ceph**.
5. `--ceph-cluster-ns`: **optional** namespace where the ceph cluster is running. **default: rook-ceph**.

```console
pv-migrator --pvc=rbd-pvc --pvc-ns=default --destination-sc=csi-rook-ceph-block
```

```console
I1125 07:56:22.247311      63 log.go:34] Create Kubernetes Client
I1125 07:56:22.259115      63 log.go:34] List all the PVC from the source storageclass
I1125 07:56:22.261205      63 log.go:34] 1 PVCs found with source StorageClass
I1125 07:56:22.261221      63 log.go:34] Start Migration of PVCs to CSI
I1125 07:56:22.261226      63 log.go:34] migrating PVC "rbd-pvc" from namespace "default"
I1125 07:56:22.261229      63 log.go:34] Fetch PV information from PVC rbd-pvc
I1125 07:56:22.266734      63 log.go:34] PV found "pvc-30a01887-8821-4baf-835c-16a7e55ba7f0"
---
I1125 07:56:26.483172      63 log.go:34] successfully renamed volume csi-vol-2f8de58f-4dc5-11ec-a130-0242ac110005 -> pvc-30a01887-8821-4baf-835c-16a7e55ba7f0
I1125 07:56:26.483194      63 log.go:34] Delete old PV object: pvc-30a01887-8821-4baf-835c-16a7e55ba7f0
I1125 07:56:26.504578      63 log.go:34] waiting for PV pvc-30a01887-8821-4baf-835c-16a7e55ba7f0 in state &PersistentVolumeStatus{Phase:Bound,Message:,Reason:,} to be deleted (0 seconds elapsed)
I1125 07:56:26.702921      63 log.go:34] deleted persistent volume pvc-30a01887-8821-4baf-835c-16a7e55ba7f0
I1125 07:56:26.702944      63 log.go:34] successfully migrated pvc rbd-pvc
I1125 07:56:26.703335      63 log.go:34] Successfully migrated all the PVCs to CSI
```

After running above command you should see something similar to this output.

## Cleanup Guide

Delete the `flex-migration.yaml` when done using migration tool.

1. `kubectl delete -f cluster/examples/kubernetes/ceph/flex-migrator.yaml`
