---
title: Ceph CSI Drivers
---

There are three CSI drivers integrated with Rook that are used in different scenarios:

* RBD: This block storage driver is optimized for RWO pod access where only one pod may access the
    storage. [More information](../Block-Storage-RBD/block-storage.md).
* CephFS: This file storage driver allows for RWX with one or more pods accessing the same storage.
    [More information](../Shared-Filesystem-CephFS/filesystem-storage.md).
* NFS (experimental): This file storage driver allows creating NFS exports that can be mounted on
    pods, or directly via an NFS client from inside or outside the
    Kubernetes cluster. [More information](../NFS/nfs-csi-driver.md)

The Ceph Filesystem (CephFS) and RADOS Block Device (RBD) drivers are enabled automatically by
the Rook operator. The NFS driver is disabled by default. All drivers will be started in the same
namespace as the operator when the first CephCluster CR is created.

## Supported Versions

The two most recent Ceph CSI version are supported with Rook. Refer to ceph csi [releases](https://github.com/ceph/ceph-csi/releases)
for more information.

## Static Provisioning

The RBD and CephFS drivers support the creation of static PVs and static PVCs from
an existing RBD image or CephFS volume/subvolume. Refer
to the [static PVC](https://github.com/ceph/ceph-csi/blob/devel/docs/static-pvc.md) documentation for more information.

## Configure CSI Drivers in non-default namespace

If you've deployed the Rook operator in a namespace other than `rook-ceph`,
change the prefix in the provisioner to match the namespace you used. For
example, if the Rook operator is running in the namespace `my-namespace` the
provisioner value should be `my-namespace.rbd.csi.ceph.com`. The same provisioner
name must be set in both the storageclass and snapshotclass.

To find the provisioner name in the example storageclasses and
volumesnapshotclass, search for: `# csi-provisioner-name`

### Configure custom Driver name prefix for CSI Drivers

To use a custom prefix for the CSI drivers instead of the namespace prefix, set
the `CSI_DRIVER_NAME_PREFIX` environment variable in the operator configmap.
For instance, to use the prefix `my-prefix` for the CSI drivers, set
the following in the operator configmap:

```console
kubectl patch cm rook-ceph-operator-config -n rook-ceph -p $'data:\n "CSI_DRIVER_NAME_PREFIX": "my-prefix"'
```

Once the configmap is updated, the CSI drivers will be deployed with the
`my-prefix` prefix. The same prefix must be set in both the storageclass and
snapshotclass. For example, to use the prefix `my-prefix` for the
CSI drivers, update the provisioner in the storageclass:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block-sc
provisioner: my-prefix.rbd.csi.ceph.com
...
```

The same prefix must be set in the volumesnapshotclass as well:

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: rook-ceph-block-vsc
driver: my-prefix.rbd.csi.ceph.com
...
```

When the prefix is set, the driver names will be:

* RBD: `my-prefix.rbd.csi.ceph.com`
* CephFS: `my-prefix.cephfs.csi.ceph.com`
* NFS: `my-prefix.nfs.csi.ceph.com`

!!! note
    Please be careful when setting the `CSI_DRIVER_NAME_PREFIX`
    environment variable. It should be done only in fresh deployments because
    changing the prefix in an existing cluster will result in unexpected behavior.

To find the provisioner name in the example storageclasses and
volumesnapshotclass, search for: `# csi-provisioner-name`

## Liveness Sidecar

All CSI pods are deployed with a sidecar container that provides a Prometheus
metric for tracking whether the CSI plugin is alive and running.

Check the [monitoring documentation](../Monitoring/ceph-monitoring.md) to see how to integrate CSI
liveness and GRPC metrics into Ceph monitoring.

## Dynamically Expand Volume

### Prerequisites

To expand the PVC the controlling StorageClass must have `allowVolumeExpansion`
set to `true`. `csi.storage.k8s.io/controller-expand-secret-name` and
`csi.storage.k8s.io/controller-expand-secret-namespace` values set in the
storageclass. Now expand the PVC by editing the PVC's
`pvc.spec.resource.requests.storage` to a higher values than the current size.
Once the PVC is expanded on the back end and the new size is reflected on
the application mountpoint, the status capacity `pvc.status.capacity.storage` of
the PVC will be updated to the new size.

## RBD Mirroring

To support RBD Mirroring,
the [CSI-Addons sidecar](https://github.com/csi-addons/kubernetes-csi-addons#readme)
will be started in the RBD provisioner pod. CSI-Addons support the `VolumeReplication`
operation. The volume replication controller provides common and reusable APIs for
storage disaster recovery. It is based on the [csi-addons](https://github.com/csi-addons/spec) specification.
It follows the controller pattern and provides extended APIs for storage disaster recovery.
The extended APIs are provided via Custom Resource Definitions (CRDs).

### Enable CSIAddons Sidecar

To enable the CSIAddons sidecar and deploy the controller, follow the
steps [below](#csi-addons-controller)

## Ephemeral volume support

The generic ephemeral volume feature adds support for specifying PVCs in the
`volumes` field to create a Volume as part of the pod spec.
This feature requires the `GenericEphemeralVolume` feature gate to be enabled.

For example:

```yaml
kind: Pod
apiVersion: v1
...
  volumes:
    - name: mypvc
      ephemeral:
        volumeClaimTemplate:
          spec:
            accessModes: ["ReadWriteOnce"]
            storageClassName: "rook-ceph-block"
            resources:
              requests:
                storage: 1Gi
```

A volume claim template is defined inside the pod spec, and defines a volume to be
provisioned and used by the pod within its lifecycle. Volumes are provisioned
when a pod is spawned and destroyed when the pod is deleted.

Refer to the [ephemeral-doc](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes) for more info.
See example manifests for an [RBD ephemeral volume](https://github.com/rook/rook/tree/master/deploy/examples/csi/rbd/pod-ephemeral.yaml)
and a [CephFS ephemeral volume](https://github.com/rook/rook/tree/master/deploy/examples/csi/cephfs/pod-ephemeral.yaml).

## CSI-Addons Controller

The CSI-Addons Controller handles requests from users. Users create a CR
that the controller inspects and forwards to one or more CSI-Addons sidecars for execution.

### Deploying the controller

Deploy the controller by running the following commands:

```console
kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/v0.14.0/crds.yaml
kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/v0.14.0/rbac.yaml
kubectl create -f https://github.com/csi-addons/kubernetes-csi-addons/releases/download/v0.14.0/setup-controller.yaml
```

This creates the required CRDs and configures permissions.

### Enable the CSI-Addons Sidecar

To use the features provided by the CSI-Addons, the `csi-addons`
containers need to be deployed in the RBD provisioner and nodeplugin pods,
which are not enabled by default.

Execute the following to enable the CSI-Addons sidecars:

* Update the `rook-ceph-operator-config` configmap and patch the
    following configuration:

    ```console
    kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_CSIADDONS": "true"'
    ```

* After enabling `CSI_ENABLE_CSIADDONS` in the configmap, a new sidecar container named `csi-addons`
    will start automatically in the RBD CSI provisioner and nodeplugin pods.

### CSI-Addons Operations

CSI-Addons supports the following operations:

* Reclaim Space
    * [Creating a ReclaimSpaceJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/reclaimspace.md#reclaimspacejob)
    * [Creating a ReclaimSpaceCronJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/reclaimspace.md#reclaimspacecronjob)
    * [Annotating PersistentVolumeClaims](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/reclaimspace.md#annotating-perstentvolumeclaims)
    * [Annotating Namespace](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/reclaimspace.md#annotating-namespace)
    * [Annotating StorageClass](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/reclaimspace.md#annotating-storageclass)
* Network Fencing
    * [Creating a NetworkFence](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/networkfence.md)
* Volume Replication
    * [Creating VolumeReplicationClass](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/volumereplicationclass.md)
    * [Creating VolumeReplication CR](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/volumereplication.md)
* Key Rotation Job for PV encryption
    * [Creating EncryptionKeyRotationJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/encryptionkeyrotation.md#encryptionkeyrotationjob)
    * [Creating EncryptionKeyRotationCronJob](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/encryptionkeyrotation.md#encryptionkeyrotationcronjob)
    * [Annotating PersistentVolumeClaims](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/encryptionkeyrotation.md#annotating-persistentvolumeclaims)
    * [Annotating Namespace](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/encryptionkeyrotation.md#annotating-namespace)
    * [Annotating StorageClass](https://github.com/csi-addons/kubernetes-csi-addons/blob/v0.14.0/docs/encryptionkeyrotation.md#annotating-storageclass)

## Enable RBD and CephFS Encryption Support

Ceph-CSI supports encrypting PersistentVolumeClaims (PVCs) for both RBD and CephFS.
This can be achieved using LUKS for RBD and fscrypt for CephFS. More details on encrypting RBD PVCs can be found
[here](https://github.com/ceph/ceph-csi/blob/v3.16.0/docs/deploy-rbd.md#encryption-for-rbd-volumes),
which includes a full list of supported encryption configurations.
More details on encrypting CephFS PVCs can be found [here](https://github.com/ceph/ceph-csi/blob/v3.14.2/docs/deploy-cephfs.md#cephfs-volume-encryption).
A sample KMS configmap can be found [here](https://github.com/ceph/ceph-csi/blob/v3.16.0/examples/kms/vault/kms-config.yaml).

!!! note
    Not all KMS are compatible with fscrypt. Generally, KMS that either store secrets to use directly (like Vault)
    or allow access to the plain password (like Kubernetes Secrets) are compatible.

!!! note
    Rook also supports OSD-level encryption (see `encryptedDevice` option [here](../../CRDs/Cluster/ceph-cluster-crd.md#osd-configuration-settings)).

Using both RBD PVC encryption and OSD encryption at the same time will lead to double encryption and may reduce read/write performance.

Existing Ceph clusters can also enable Ceph-CSI PVC encryption support and multiple kinds of encryption
KMS can be used on the same Ceph cluster using different storageclasses.

The following steps demonstrate the common process for enabling encryption support for both RBD and CephFS:

* Create the `rook-ceph-csi-kms-config` configmap with required encryption configuration in
the same namespace where the Rook operator is deployed. An example is shown below:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-ceph-csi-kms-config
  namespace: rook-ceph
data:
  config.json: |-
    {
      "user-secret-metadata": {
        "encryptionKMSType": "metadata",
        "secretName": "storage-encryption-secret"
      }
    }
```

* Update the `rook-ceph-operator-config` configmap and patch the
    following configurations

```console
kubectl patch cm rook-ceph-operator-config -nrook-ceph -p $'data:\n "CSI_ENABLE_ENCRYPTION": "true"'
```

* Create the resources (secrets, configmaps etc) required by the encryption type.
In this case, create `storage-encryption-secret` secret in the namespace of the PVC as follows:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-encryption-secret
  namespace: rook-ceph
stringData:
  encryptionPassphrase: test-encryption
```

* Create a new [RBD storageclass](https://github.com/rook/rook/blob/master/deploy/examples/csi/rbd/storageclass.yaml) or
[CephFS storageclass](https://github.com/rook/rook/blob/master/deploy/examples/csi/cephfs/storageclass.yaml) with additional parameters
`encrypted: "true"` and `encryptionKMSID: "<key used in configmap>"`. An example is shown below:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block-encrypted
parameters:
  # additional parameters required for encryption
  encrypted: "true"
  encryptionKMSID: "user-secret-metadata"
# ...
```

* PVCs created using the new storageclass will be encrypted.

!!! note
    CephFS encryption requires fscrypt support in Linux kernel, kernel version 6.6 or higher.

## Enable Read affinity for RBD and CephFS volumes

Ceph CSI supports mapping RBD volumes with KRBD options and mounting
CephFS volumes with ceph mount options to allow
serving reads from an OSD closest to the client, according to
OSD locations defined in the CRUSH map and topology labels on nodes.

Refer to the [krbd-options document](https://docs.ceph.com/en/latest/man/8/rbd/#kernel-rbd-krbd-options)
for more details.

Execute the following step to enable read affinity for a specific ceph cluster:

* Patch the ceph cluster CR to enable read affinity:

```console
kubectl patch cephclusters.ceph.rook.io <cluster-name> -n rook-ceph -p '{"spec":{"csi":{"readAffinity":{"enabled": true}}}}'
```

```yaml
  csi:
    readAffinity:
      enabled: true
```

* Add topology labels to the Kubernetes nodes. The same labels may be used as mentioned in the
[OSD topology](../../CRDs/Cluster/ceph-cluster-crd.md#osd-topology) topic.

Ceph CSI will extract the CRUSH location from the topology labels found on the node
and pass it though krbd options during mapping RBD volumes.

!!! note
    This requires Linux kernel version 5.8 or higher.
