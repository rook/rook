# Ceph CSI Driver Support
**Targeted for v0.9**

## Background

Container Storage Interface (CSI) is a set of specifications for container
orchestration frameworks to manage storage. The CSI spec abstracts common
storage features such as create/delete volumes, publish/unpublish volumes,
stage/unstage volumes, and more. It is currently at the 1.0 release.

Kubernetes started to support CSI with alpha support in
[1.9](https://kubernetes.io/blog/2018/01/introducing-container-storage-interface/),
beta support in
[1.10](https://kubernetes.io/blog/2018/04/10/container-storage-interface-beta/),
and CSI 1.0 in [Kubernetes
1.13](https://kubernetes.io/blog/2018/12/03/kubernetes-1-13-release-announcement/).

It is projected that CSI will be the only supported persistent storage driver
in the near feature. In-tree drivers such as Ceph RBD and CephFS will be replaced with their respective CSI drivers.

## Ceph CSI Drivers Status

There have been active Ceph CSI drivers developments since Kubernetes 1.9.
Both Ceph RBD and CephFS drivers can be found at
[ceph/ceph-csi](https://github.com/ceph/ceph-csi). Currently ceph-csi
supports both the CSI v0.3.0 spec and CSI v1.0 spec.

* RBD driver. Currently, rbd CSI driver supports both krbd and rbd-nbd. There is a consideration to support other forms of TCMU based drivers.
* CephFS driver. Both Kernel CephFS and Ceph FUSE are supported. When `ceph-fuse` is installed on the CSI plugin container, it can be used to mount CephFS shares.

There is also upstream Kubernetes work to [include these drivers for e2e tests](https://github.com/kubernetes/kubernetes/pull/67088).

## Kubernetes CSI Driver Deployment

Starting Kubernetes CSI driver brings up an [external-provisioner](https://github.com/kubernetes-csi/external-provisioner), an [external-attacher](https://github.com/kubernetes-csi/external-attacher), DaemonSet that runs the driver on the nodes, and optionally an [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter).

For example, deploying a CephFS CSI driver consists of the following steps:
1. Creating a [RBAC for external provisioner](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-provisioner-rbac.yaml) and the [provisioner itself](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml).
2. Creating [RBAC for CSI driver](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-nodeplugin-rbac.yaml) and [driver DaemonSet](https://github.com/ceph/ceph-csi/blob/master/deploy/cephfs/kubernetes/csi-cephfsplugin.yaml)
3. Creating Storage Classes for CSI provisioners.

## Integration Plan

The aim is to support CSI 1.0 in Rook as a beta with Rook release 1.0. In Rook
1.1 CSI support will be considered stable.

### How can Rook improve CSI drivers reliability?

Rook can ensure resources used by CSI drivers and their associated Storage Classes are created, protected, and updated. Specifically, when a CSI based Storage Class is created, the referenced Pools and Filesystems should exist by the time a PVC uses this Storage Class. Otherwise Rook should resolve missing resources to avoid PVC creation failure. Rook should prevent Pools or Filesystems that are still being used by PVCs from accidental removal. Similarly, when the resources, especially mon addresses, are updated, Rook should try to update the Storage Class as well.

Rook Ceph Agent supports Ceph mon failover by storing Ceph cluster instead of Ceph mon addresses in Storage Classes. This gives the Agent the ability to retrieve most current mon addresses at mount time. RBD CSI driver also allows mon address stored in other Kubernetes API objects (currently in a Secret) than Storage Class. However, Rook Operator must be informed to update mon addresses in this object during mon failover/addition/removal. Either a new CRD object or a Storage Class label has to be created to help Operator be aware that a Ceph cluster is used by a CSI based Storage Class. When Operator updates mon addresses, the Secret referenced by the Storage Class must be updated as well to pick up the latest mon addresses.

### Coexist with flex driver

No changes to the current method of deploying Storage Classes with the flex
driver should be required. Eventually, the flex driver approach will be
deprecated and CSI will become the default method of working with Storage
Classes. At or around the time of flex driver deprecation Rook should provide a
method to upgrade/convert existing flex driver based provisioning with CSI
based provisioning.

The behavior of the CSI integration must be complementary to the approach
currently taken by the flex volume driver. When CSI is managed through Rook it
should work with the existing Rook CRDs and aim to minimize the required
configuration parameters.


### Work with out-of-band CSI drivers deployment

Rook generally has more information about the state of the cluster than the
static settings in the Storage Class and is more up-to-date than the system's
administrators. When so configured, Rook could additionally manage the
configuration of CSI not directly managed by Rook.


### Rook initiated CSI drivers deployment

With the addition of CSI 1.0 support in Kubernetes 1.13 Rook should become a
fully-featured method of deploying Ceph-CSI aiming to minimize extra steps
needed to use CSI targeting the Rook managed Ceph cluster(s). Initially, this
would be an opt-in feature that requires Kubernetes 1.13. Supporting CSI
versions earlier than 1.0 will be a non-goal.

Opting in to CSI should be simple and require very few changes from the
currently documented approach for deploying Rook. Configuring Rook to use CSI
should require changing only the default mechanism for interacting with the
storage classes. The standard deployment should include the needed RBAC files
for managing storage with Ceph-CSI. Rook should package and/or source all other
needed configuration files/templates. All other configuration must be defaulted
to reasonable values and only require changing if the user requires it.

The following are plans to converge the user experience when choosing to use
CSI rather than the flex volume method:

#### Ceph-CSI Requirements

To manage CSI with Rook the following requirements are placed on the
ceph-csi project:
* Configuration parameters currently set in the Storage Class will be
  configurable via secrets:
  * Mons (done)
  * Admin Id
  * User Id
* The key "blockpool" will serve as an alias to "pool"

To additionally minimize the required parameters in a Storage Class it may
require changes to create CSI instance secrets; secrets that are associated
with CSI outside of the storage class (see [ceph-csi
PR#244](https://github.com/ceph/ceph-csi/pull/224)).  If this change is made
nearly no parameters will be directly required in the storage class.

#### Rook Requirements

To manage CSI with Rook the following requirements are place on Rook:
* Rook deployments must include all the needed RBAC rules to set up CSI
* Rook deploys all additional CSI components required to provision
  and mount a volume
* Rook must be able to dynamically update the secrets used to configure
  Ceph-CSI, including but not limited to the mons list.
* Users should not be required to deploy Rook differently when using
  CSI versus flex except minimal steps to opt in to CSI
* When provisioning Ceph-CSI Rook must uniquely identify the
  driver/provisioner name so that multiple CSI drivers or multiple Rook
  instances within a (Kubernetes) cluster will not collide


### Future points of integration

While not immediately required this section outlines a few improvements
that could be made to improve the Rook and CSI integration:

#### Extend CephBlockPool and CephFilesystem CRDs to provision Storage Classes

Extend CephBlockPool and CephFilesystem CRDs to automatically provision Storage
Classes when so configured. Instead of requiring an administrator to create a
CRD and a Storage Class, add metadata to the CRD such that Rook will
automatically create storage classes based on that additional metadata.

#### Select Flex Provisioning or CSI based on CephCluster CRD

Currently the code requires changing numerous parameters to enable CSI.  This
document aims to change that to a single parameter. In the future it may be
desirable to make this more of a "runtime" parameter that could be managed in
the cluster CRD.
