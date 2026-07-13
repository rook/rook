---
title: RBD QoS
---

RBD Quality of Service (QoS) allows administrators to set IOPS and bandwidth limits on
RBD block volumes. This prevents noisy-neighbor issues in multi-tenant clusters by ensuring
that no single workload can saturate the storage cluster's I/O capacity.

QoS is configured through a `VolumeAttributesClass` resource, which allows dynamic
modification of QoS limits on existing volumes without recreation using the
`ControllerModifyVolume` CSI operation.

## Supported Mounters

### krbd (cgroup v2)

The default krbd (kernel RBD) mounter enforces QoS via the Linux cgroup v2 `io.max`
controller at the container level.

!!! note
    Kubernetes >= v1.34 is required (VolumeAttributesClass is GA since v1.34).
    cgroup v2 must be enabled on the nodes (default on most modern Linux distributions)
    and Linux kernel >= 5.8 is required.

## QoS Parameters Reference

### krbd (cgroup v2) Parameters

The following parameters can be set on a `VolumeAttributesClass` resource. All parameters
are optional — set only the limits you need.

| Parameter | Description | Example |
|-----------|-------------|---------|
| `maxReadIops` | Maximum read IOPS limit | `"1000"` |
| `maxWriteIops` | Maximum write IOPS limit | `"2000"` |
| `maxReadBps` | Maximum read bandwidth (bytes/sec) | `"104857600"` (100 MiB/s) |
| `maxWriteBps` | Maximum write bandwidth (bytes/sec) | `"209715200"` (200 MiB/s) |

## Fresh Cluster Setup

For new Rook Ceph deployments, follow these steps to enable QoS.

1. Ensure the StorageClass has the required secret references for modify and
    publish operations. The default
    [storageclass.yaml](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/storageclass.yaml)
    already includes these:

    ```yaml
    parameters:
      # ... existing parameters ...
      # Required for VolumeAttributesClass (dynamic QoS modification)
      csi.storage.k8s.io/controller-modify-secret-name: rook-csi-rbd-provisioner
      csi.storage.k8s.io/controller-modify-secret-namespace: rook-ceph # namespace:cluster
      # Required for krbd QoS (cgroup v2)
      csi.storage.k8s.io/node-publish-secret-name: rook-csi-rbd-node
      csi.storage.k8s.io/node-publish-secret-namespace: rook-ceph # namespace:cluster
    ```

2. Create the VolumeAttributesClass:

    ```console
    kubectl create -f deploy/examples/csi/rbd/volumeattributesclass-cgroup.yaml
    ```

    See the [volumeattributesclass-cgroup.yaml](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/volumeattributesclass-cgroup.yaml)
    for the full example with all available parameters.

3. Create a PVC with the VolumeAttributesClass:

    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: rbd-pvc-qos
    spec:
      accessModes:
        - ReadWriteOnce
      volumeAttributesClassName: rook-ceph-rbd-cgroup-qos
      resources:
        requests:
          storage: 10Gi
      storageClassName: rook-ceph-block
    ```

4. To change QoS on an existing volume, update the PVC's `volumeAttributesClassName`:

    ```console
    kubectl patch pvc rbd-pvc-qos -p '{"spec":{"volumeAttributesClassName":"rook-ceph-rbd-cgroup-qos"}}'
    ```

    !!! note
        A single pod can have multiple volumes with different QoS limits by using different VolumeAttributesClasses for each PVC.

## Upgrading Existing Clusters

VolumeAttributesClass allows applying QoS to existing volumes without migration.
Follow these steps to enable QoS on upgraded clusters.

1. **Update the existing StorageClass** to add the required secret references. Since StorageClass parameters are immutable, you must delete and recreate it:

    !!! warning
        Deleting a StorageClass does not affect existing PVs or PVCs. However, new PVC creation
        will fail until the StorageClass is recreated.

    ```console
    kubectl get storageclass rook-ceph-block -o yaml > storageclass-backup.yaml
    kubectl delete storageclass rook-ceph-block
    ```

    Edit the storageclass-backup.yaml to add the modify-secret and publish-secret parameters as described
    in [Step 1 of the Fresh Cluster Setup](#fresh-cluster-setup), then recreate:

    ```console
    kubectl create -f storageclass-backup.yaml
    ```

2. **Create the VolumeAttributesClass**:

    ```console
    kubectl create -f deploy/examples/csi/rbd/volumeattributesclass-cgroup.yaml
    ```

3. **Apply QoS to existing PVCs** by patching them with the VolumeAttributesClass:

    ```console
    kubectl patch pvc <pvc-name> -p '{"spec":{"volumeAttributesClassName":"rook-ceph-rbd-cgroup-qos"}}'
    ```

4. **Verify QoS is applied** by checking the PVC status:

    ```console
    kubectl get pvc <pvc-name> -o jsonpath='{.status.currentVolumeAttributesClassName}'
    ```

    The output should match the VolumeAttributesClass name you applied.

## Troubleshooting

### QoS Limits Not Being Enforced

1. **Verify VolumeAttributesClass**: Check that the VolumeAttributesClass exists and the PVC references it:

    ```console
    kubectl get volumeattributesclass
    kubectl get pvc <pvc-name> -o jsonpath='{.spec.volumeAttributesClassName}'
    ```

2. **Check CSI driver logs**: Look for QoS-related errors in the CSI node plugin:

    ```console
    kubectl logs -n rook-ceph -l app=csi-rbdplugin -c csi-rbdplugin --tail=100 | grep -i qos
    ```

### cgroup v2 QoS Not Working

1. **Verify cgroup v2**: Check that nodes use cgroup v2:

    ```console
    kubectl debug node/<node-name> -it --image=busybox -- cat /host/sys/fs/cgroup/cgroup.controllers
    ```

    If the file exists and lists `io`, cgroup v2 is active with the `io` controller enabled.

2. **Check Kubernetes version**: VolumeAttributesClass requires Kubernetes >= v1.34. Verify:

    ```console
    kubectl version --short
    ```
