# Glossary

## Rook

### CephBlockPool CRD

The [CephBlockPool CRD](../CRDs/Block-Storage/ceph-block-pool-crd.md) is used by Rook to allow creation and customization of storage pools.

### CephBlockPoolRadosNamespace CRD

The [CephBlockPoolRadosNamespace CRD](../CRDs/Block-Storage/ceph-block-pool-rados-namespace-crd.md) is used by Rook to allow creation of Ceph RADOS Namespaces.

### CephClient CRD

CephClient CRD is used by Rook to allow [creation](https://rook.io/docs/rook/latest/CRDs/ceph-client-crd/) and updating clients.

### CephCluster CRD

The [CephCluster CRD](../CRDs/Cluster/ceph-cluster-crd.md) is used by Rook to allow creation and customization of storage clusters through the custom resource definitions (CRDs).

### Ceph CSI

The [Ceph CSI plugins](../Storage-Configuration/Ceph-CSI/ceph-csi-drivers.md) implement an interface between a CSI-enabled Container Orchestrator (CO) and Ceph clusters.

### CephFilesystem CRD

The [CephFilesystem CRD](../CRDs/Shared-Filesystem/ceph-filesystem-crd.md) is used by Rook to allow creation and customization of shared filesystems through the custom resource definitions (CRDs).

### CephFilesystemMirror CRD

The [CephFilesystemMirror CRD](../CRDs/Shared-Filesystem/ceph-fs-mirror-crd.md) is used by Rook to allow creation and updating the Ceph fs-mirror daemon.

### CephFilesystemSubVolumeGroup CRD

CephFilesystemMirror CRD is used by Rook to allow [creation](../CRDs/Shared-Filesystem/ceph-fs-subvolumegroup-crd.md) of Ceph Filesystem SubVolumeGroups.

### CephNFS CRD

CephNFS CRD is used by Rook to allow exporting NFS shares of a CephFilesystem or CephObjectStore through the CephNFS custom resource definition. For further information please refer to the example [here](https://rook.io/docs/rook/latest/CRDs/ceph-nfs-crd/#example).

### CephObjectStore CRD

CephObjectStore CRD is used by Rook to allow [creation](https://rook.io/docs/rook/latest/CRDs/Object-Storage/ceph-object-store-crd/#example) and customization of object stores.

### CephObjectStoreUser CRD

CephObjectStoreUser CRD is used by Rook to allow creation and customization of object store users. For more information and examples refer to this [documentation](../CRDs/Object-Storage/ceph-object-store-user-crd.md).

### CephObjectRealm CRD

CephObjectRealm CRD is used by Rook to allow creation of a realm in a Ceph Object Multisite configuration. For more information and examples refer to this [documentation](../CRDs/Object-Storage/ceph-object-realm-crd.md).

### CephObjectZoneGroup CRD

CephObjectZoneGroup CRD is used by Rook to allow creation of zone groups in a Ceph Object Multisite configuration. For more information and examples refer to this [documentation](../CRDs/Object-Storage/ceph-object-zonegroup-crd.md).

### CephObjectZone CRD

CephObjectZone CRD is used by Rook to allow creation of zones in a ceph cluster for a Ceph Object Multisite configuration. For more information and examples refer to this [documentation](../CRDs/Object-Storage/ceph-object-zone-crd.md).

### CephRBDMirror CRD

CephRBDMirror CRD is used by Rook to allow creation and updating rbd-mirror daemon(s) through the custom resource definitions (CRDs). For more information and examples refer to this [documentation](../CRDs/Block-Storage/ceph-rbd-mirror-crd.md).

### External Storage Cluster

An [external cluster](../CRDs/Cluster/external-cluster/external-cluster.md) is a Ceph configuration that is managed outside of the local K8s cluster.

### Host Storage Cluster

A [host storage cluster](../CRDs/Cluster/host-cluster.md) is where Rook configures Ceph to store data directly on the host devices.

### kubectl Plugin

The [Rook kubectl plugin](../Troubleshooting/kubectl-plugin.md) is a tool to help troubleshoot your Rook cluster.

### Object Bucket Claim (OBC)

An Object Bucket Claim (OBC) is custom resource which requests a bucket (new or existing) from a Ceph object store. For further reference please refer to [OBC Custom Resource](../Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-claim.md).

### Object Bucket (OB)

An Object Bucket (OB) is a custom resource automatically generated when a bucket is provisioned. It is a global resource, typically not visible to non-admin users, and contains information specific to the bucket.

### OpenShift

[OpenShift](https://www.redhat.com/en/technologies/cloud-computing/openshift/container-platform) Container Platform is a distribution of the Kubernetes container platform.

### PVC Storage Cluster

In a [PersistentVolumeClaim-based cluster](../CRDs/Cluster/pvc-cluster.md), the Ceph persistent data is stored on volumes requested from a storage class of your choice.

### Stretch Storage Cluster

A stretched cluster is a deployment model in which two datacenters with low latency are available for storage in the same K8s cluster, rather than three or more. To support this scenario, Rook has integrated support for [stretch clusters](../CRDs/Cluster/stretch-cluster.md).

### Toolbox

The [Rook toolbox](../Troubleshooting/ceph-toolbox.md) is a container with common tools used for rook debugging and testing.

## Ceph

[Ceph](https://docs.ceph.com/en/latest/) is a distributed network storage and file system with distributed metadata management and POSIX semantics. See also the [Ceph Glossary](https://docs.ceph.com/en/latest/glossary/). Here are a few of the important terms to understand:

* [Ceph Monitor](https://docs.ceph.com/en/latest/glossary/#term-Ceph-Monitor) (MON)
* [Ceph Manager](https://docs.ceph.com/en/latest/glossary/#term-Ceph-Manager) (MGR)
* [Ceph Metadata Server](https://docs.ceph.com/en/latest/glossary/#term-MDS) (MDS)
* [Object Storage Device](https://docs.ceph.com/en/latest/glossary/#term-OSD) (OSD)
* [RADOS Block Device](https://docs.ceph.com/en/latest/glossary/#term-Ceph-Block-Device) (RBD)
* [Ceph Object Gateway](https://docs.ceph.com/en/latest/glossary/#term-Ceph-Object-Gateway) (RGW)

## Kubernetes

Kubernetes, also known as K8s, is an open-source system for automating deployment, scaling, and management of containerized applications. For further information see also the [Kubernetes Glossary](https://kubernetes.io/docs/reference/glossary) for more definitions. Here are a few of the important terms to understand:

* [Affinity](https://kubernetes.io/docs/reference/glossary/?all=true#term-affinity)
* [Container Storage Interface (CSI)](https://kubernetes.io/docs/reference/glossary/?all=true#term-csi) for Kubernetes
* [CustomResourceDefinition (CRDs)](https://kubernetes.io/docs/reference/glossary/?all=true#term-CustomResourceDefinition)
* [DaemonSet](https://kubernetes.io/docs/reference/glossary/?all=true#term-daemonset)
* [Deployment](https://kubernetes.io/docs/reference/glossary/?all=true#term-deployment)
* [Finalizer](https://kubernetes.io/docs/reference/glossary/?all=true#term-finalizer)
* [Node affinity](https://kubernetes.io/docs/tasks/configure-pod-container/assign-pods-nodes-using-node-affinity/)
* [Node Selector](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector)
* [PersistentVolume (PV)](https://kubernetes.io/docs/reference/glossary/?all=true#term-persistent-volume)
* [PersistentVolumeClaim (PVC)](https://kubernetes.io/docs/reference/glossary/?all=true#term-persistent-volume-claim)
* [Selector](https://kubernetes.io/docs/reference/glossary/?all=true#term-selector)
* [Storage Class](https://kubernetes.io/docs/reference/glossary/?all=true#term-storageclass)
* [Taint](https://kubernetes.io/docs/reference/glossary/?all=true#term-taint)
* [Toleration](https://kubernetes.io/docs/reference/glossary/?all=true#term-toleration)
* [Volume](https://kubernetes.io/docs/reference/glossary/?all=true#term-volume)
