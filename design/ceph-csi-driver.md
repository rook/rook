# Ceph CSI Driver Support
**Targeted for v0.9**

## Background

Container Storage Interface (CSI) is a set of gRPC specifications for container orchestrators to manage storage drivers. CSI spec abstracts
common storage features such as create/delete volumes, publish/unpublish volumes, stage/unstage volumes, and more. It is currently at 0.3.0 release.

Kubernetes started to CSI driver alpha support in [1.9](https://kubernetes.io/blog/2018/01/introducing-container-storage-interface/), beta support in [1.10](https://kubernetes.io/blog/2018/04/10/container-storage-interface-beta/).

It is projected that CSI will be the only supported persistent storage driver
in the near feature. In-tree drivers such as Ceph RBD and CephFS will be replaced with their respective CSI drivers.

## Ceph CSI Drivers Status

There have been active Ceph CSI drivers developments since Kubernetes 1.9. 
Both Ceph RBD and CephFS drivers can be found at [ceph/ceph-csi](https://github.com/ceph/ceph-csi). Currently the drivers are up to CSI v0.3.0 spec.

* RBD driver. Currently, rbd CSI driver supports both krbd and rbd-nbd. There is a consideration to support other forms of TCMU based drivers.
* CephFS driver. Both Kernel CephFS and Ceph FUSE are supported. When `ceph-fuse` is installed on the CSI plugin container, it can be used to mount CephFS shares.

There is also upstream Kubernetes work to [include these drivers for e2e tests](https://github.com/kubernetes/kubernetes/pull/67088).

## Kubernetes CSI Driver Deployment

Starting Kubernetes CSI driver brings up an [external-provisioner](https://github.com/kubernetes-csi/external-provisioner), an [external-attacher](https://github.com/kubernetes-csi/external-attacher), DaemonSet that runs the driver on the nodes, and optionally an [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter).

For example, deploying a CephFS CSI driver consists of the following steps:
1. Creating a [RBAC for external provisioner](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-provisioner-rbac.yaml) and the [provisioner itself](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml).
2. Creating a [RBAC for external attacher](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-attacher-rbac.yaml) and [attacher itself](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-attacher-rbac.yaml).
3. Creating [RBAC for CSI driver](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-nodeplugin-rbac.yaml) and [driver DaemonSet](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-cephfsplugin.yaml)
4. Creating Storage Classes for CSI provisioners.

## Integration Plan

### How can Rook improve CSI drivers reliability?

Rook can ensure resources used by CSI drivers and their associated Storage Classes are created, protected, and updated. Specifically, when a CSI based Storage Class is created, the referenced Pools and Filesystems should exist by the time a PVC uses this Storage Class. Otherwise Rook should resolve missing resources to avoid PVC creation failure. Rook should prevent Pools or Filesystems that are still being used by PVCs from accidental removal. Similarly, when the resources, especially mon addresses, are updated, Rook should try to update the Storage Class as well. 

Rook Ceph Agent supports Ceph mon failover by storing Ceph cluster instead of Ceph mon addresses in Storage Classes. This gives the Agent the ability to retrieve most current mon addresses at mount time. RBD CSI driver also allows mon address stored in other Kubernetes API objects (currently in a Secret) than Storage Class. However, Rook Operator must be informed to update mon addresses in this object during mon failover/addition/removal. Either a new CRD object or a Storage Class label has to be created to help Operator be aware that a Ceph cluster is used by a CSI based Storage Class. When Operator updates mon addresses, the Secret refereneced by the Storage Class must be updated as well to pick up the latest mon addresses. 

### Coexist with flex driver
Rook's local Ceph Node Agent is modeled after flexvolume drivers. There are some overlapping with Kubernetes CSI model:

- VolumeAttachment API objects are currently in Kubernetes storage API group. The VolumeAttachment watchers and finalizers can be replaced with CSI attacher.
- There is external CSI volume provisioner that calls out the drivers' create/delete volume. Rook's own provisioner needs to be integrated/replaced with the external provisioner
- The flexvolume drivers can be replaced by CSI drivers when running on Kubernetes 1.10+.


### Work with out-of-band CSI drivers deployment

If CSI drivers are already successfully deployed, Rook should help monitor the Storage Classes used by the CSI drivers, ensuring mon addresses, Pools, Filesystems referenced by the Storage Classes up-to-date.


### Rook initiated CSI drivers deployment

If Rook is instructed to deploy CSI drivers and other external controllers, Rook Operator should create all the driver and daemon deployments as well.

The probe based CSI driver discovery feature is currently in alpha in Kubernetes 1.11. If the feature is not turned on, CSI driver registratar will not be able to annotate the node, the external attacher will thus fail to attach the volume. The feature is expected to be in beta in Kubernetes 1.12. Before then, Rook Operator has to annotate the node if necessary. 
