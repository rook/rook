---
title: CSI driver
weight: 4700
indent: true
---
{% include_relative branch.liquid %}

# EdgeFS Rook integrated CSI driver, provisioner, attacher and snapshotter

[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, attacher and snapshotter for EdgeFS Scale-Out NFS/ISCSI services

## Overview

EdgeFS CSI plugins implement an interface between CSI enabled Container Orchestrator (CO) and EdgeFS local cluster site. It allows dynamic and static provisioning of EdgeFS NFS exports and ISCSI LUNs, and attaching them to application workloads. With EdgeFS NFS/ISCSI implementation, I/O load can be spread-out across multiple PODs, thus eliminating I/O bottlenecks of classing single-node NFS/ISCSI and providing highly available persistent volumes.  Current implementation of EdgeFS CSI plugins was tested in Kubernetes environment (requires Kubernetes 1.13+)

## Prerequisites

* Ensure your kubernetes cluster version is 1.13+
* Kubernetes cluster must allow privileged pods, this flag must be set for the API server and the kubelet
([instructions](https://github.com/kubernetes-csi/docs/blob/735f1ef4adfcb157afce47c64d750b71012c8151/book/src/Setup.md#enable-privileged-pods)):

```console
  --allow-privileged=true
```

* Required the API server and the kubelet feature gates
  ([instructions](https://github.com/kubernetes-csi/docs/blob/735f1ef4adfcb157afce47c64d750b71012c8151/book/src/Setup.md#enabling-features)):

```console
  --feature-gates=VolumeSnapshotDataSource=true,CSIDriverRegistry=true
```

* Mount propagation must be enabled, the Docker daemon for the cluster must allow shared mounts
  ([instructions](https://github.com/kubernetes-csi/docs/blob/735f1ef4adfcb157afce47c64d750b71012c8151/book/src/Setup.md#enabling-mount-propagation))
* Kubernetes CSI drivers require `CSIDriver` and `CSINodeInfo` resource types
  [to be defined on the cluster](https://github.com/kubernetes-csi/docs/blob/460a49286fe164a78fde3114e893c48b572a36c8/book/src/Setup.md#csidriver-custom-resource-alpha).
  Check if they are already defined:
  
  ```console
  kubectl get customresourcedefinition.apiextensions.k8s.io/csidrivers.csi.storage.k8s.io
  kubectl get customresourcedefinition.apiextensions.k8s.io/csinodeinfos.csi.storage.k8s.io
  ```

  If the cluster doesn't have "csidrivers" and "csinodeinfos" resource types, create them:

  ```console
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/release-1.13/pkg/crd/manifests/csidriver.yaml
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/release-1.13/pkg/crd/manifests/csinodeinfo.yaml
  ```

* Depends on preferred CSI driver type, following utilities must be installed on each Kubernetes node (For Debian/Ubuntu based systems):

  ```console
  # for NFS
  apt install -y nfs-common rpcbind
  # for ISCSI
  apt install -y open-iscsi
  ```

## EdgeFS CSI drivers configuration

For each driver type (NFS/ISCSI) we have already prepared configuration files examples, there are:

* [EdgeFS CSI NFS driver config](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/edgefs/csi/nfs/edgefs-nfs-csi-driver-config.yaml)
* [EdgeFS CSI ISCSI driver config](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/edgefs/csi/iscsi/edgefs-iscsi-csi-driver-config.yaml)

Secret file configuration options example:

```console
# EdgeFS k8s cluster options
k8sEdgefsNamespaces: ["rook-edgefs"]          # edgefs cluster namespace
k8sEdgefsMgmtPrefix: rook-edgefs-mgr     # edgefs cluster management prefix

# EdgeFS csi operations options
cluster: cltest           # substitution edgefs cluster name for csi operations
tenant: test              # substitution edgefs tenant name for csi operations
#serviceFilter: "nfs01"   # comma delimited list of allowed service names for filtering

# EdgeFS GRPC security options
username: admin           # edgefs k8s cluster grpc service username
password: admin           # edgefs k8s cluster grpc service password
```

Options for NFS and ISCSI configuration files

| `Name`                  | `Description`                                                         | `Default value`                 | `Required` | `Type`     |
| ----------------------- | --------------------------------------------------------------------- | ------------------------------- | ---------- | ---------- |
| `k8sEdgefsNamespaces`   | Array of Kubernetes cluster's namespaces for EdgeFS service discovery | `rook-edgefs`                   | true       | both       |
| `k8sEdgefsMgmtPrefix`   | Rook EdgeFS cluster mgmt service prefix                               | `rook-edgefs-mgr`               | true       | both       |
| `username`              | EdgeFS gRPC API server privileged user                                | `admin`                         | true       | both       |
| `password`              | EdgeFS gRPC API server password                                       | `admin`                         | true       | both       |
| `cluster`               | EdgeFS cluster namespace also known as 'region'                       |                                 | false      | both       |
| `tenant`                | EdgeFS tenant isolated namespace                                      |                                 | false      | both       |
| `bucket`                | EdgeFS tenant bucket to use as a default                              |                                 | false      | ISCSI only |
| `serviceFilter`         | Comma delimited list of allowed service names for filtering           | `""` means all services allowed | false      | both       |
| `serviceBalancerPolicy` | Service selection policy [`minexportspolicy`, `randomservicepolicy`]  | `minexportspolicy`              | false      | both       |
| `chunksize`             | Chunk size for actual volume, in bytes                                | `16384`, should be power of two | false      | both       |
| `blocksize`             | Block size for actual volume, in bytes                                | `4096`, should be power of two  | false      | iSCSI only |
| `fsType`                | New volume's filesystem type                                          | `ext4`, `ext3`, `xfs`           | ext4       | ISCSI only |
| `forceVolumeDeletion`   | Automatically deletes EdgeFS volume after usage                       | `false`                         | false      | both       |

By using `k8sEdgefsNamespaces` and `k8sEdgefsMgmtPrefix` parameters, driver is capable of detecting ClusterIPs and Endpoint IPs to provision and attach volumes.

## Apply EdgeFS CSI NFS driver configuration

Check configuration options and create kubernetes secret for Edgefs CSI NFS plugin

```console
git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd cluster/examples/kubernetes/edgefs/csi/nfs
kubectl create secret generic edgefs-nfs-csi-driver-config --from-file=./edgefs-nfs-csi-driver-config.yaml
```

## Deploy EdgeFS CSI NFS driver

After secret is created successfully, deploy EdgeFS CSI plugin, provisioner and attacher using the following command

```console
cd cluster/examples/kubernetes/edgefs/csi/nfs
kubectl apply -f edgefs-nfs-csi-driver.yaml
```

There should be number of EdgeFS CSI plugin PODs available running as a DaemonSet:

```console
...
NAMESPACE     NAME                           READY   STATUS    RESTARTS   AGE
default       edgefs-nfs-csi-controller-0    4/4     Running   0          33s
default       edgefs-nfs-csi-node-9st9n      2/2     Running   0          33s
default       edgefs-nfs-csi-node-js7jp      2/2     Running   0          33s
default       edgefs-nfs-csi-node-lhjgr      2/2     Running   0          33s
...
```

At this point configuration is all ready and available for consumption by applications.

## Pre-provisioned volumes (NFS)

This method allows to use already created exports in EdgeFS services. This method keeps exports provisioned after application PODs terminated.
Read more on how to create PersistentVolume specification for pre-provisioned volumes:

[Link to Pre-provisioned volumes manifest specification](https://kubernetes-csi.github.io/docs/Usage.html#pre-provisioned-volumes)

To test creation and mount pre-provisioned volume to pod execute example

> **NOTE**: Make sure that `volumeHandle: segment:service@cluster/tenant/bucket` in nginx.yaml already exist on EdgeFS cluster and served via any Edgefs NFS service. Any volumeHandle's parameters may be omitted and will be substituted via CSI configuration file parameters.

Examples:

```console
cd cluster/examples/kubernetes/edgefs/csi/nfs/examples
kubectl apply -f ./preprovisioned-edgefs-volume-nginx.yaml
```

## Dynamically provisioned volumes (NFS)

To setup the system for dynamic provisioning, administrator needs to setup a StorageClass pointing to the CSI driver’s external-provisioner and specifying any parameters required by the driver

[Link to dynamically provisioned volumes specification](https://kubernetes-csi.github.io/docs/Usage.html#dynamic-provisioning)

### Note

For dynamically provisioned volumes kubernetes will generate volume name automatically
(for example pvc-871068ed-8b5d-11e8-9dae-005056b37cb2)
Additional creation options should be passed as parameters in StorageClass definition i.e :

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: edgefs-nfs-csi-storageclass
provisioner: io.edgefs.csi.nfs
parameters:
  segment: rook-edgefs
  service: nfs01
  tenant: ten1
  encryption: true
```

### Parameters

> **NOTE**: Parameters and their options are case sensitive and should be in lower case.

| `Name`       | `Description`                                            | `Allowed values`                                  | `Default value` |
| ------------ | -------------------------------------------------------- | ------------------------------------------------- | --------------- |
| `segment`    | Edgefs cluster namespace for current StorageClass or PV. |                                                   | rook-edgefs     |
| `service`    | Edgefs cluster service if not defined in secret          |                                                   |                 |
| `cluster`    | Edgefs cluster namespace if not defined in secret        |                                                   |                 |
| `tenant`     | Edgefs tenant  namespace if not defined in secret        |                                                   |                 |
| `chunksize`  | Chunk size for actual volume, in bytes                   | should be power of two                            | 16384 bytes     |
| `blocksize`  | Block size for actual volume, in bytes                   | should be power of two                            | 4096 bytes      |
| `acl`        | Volume acl restrictions                                  |                                                   | all             |
| `ec`         | Enables ccow erasure coding for volume                   | `true`, `false`, `0`, `1`                         | `false`         |
| `ecmode`     | Set ccow erasure mode data mode (If 'ec' option enabled) | `3:1:xor`, `2:2:rs`, `4:2:rs`, `6:2:rs`, `9:3:rs` | `6:2:rs`        |
| `encryption` | Enables encryption for volume                            | `true`, `false`, `0`, `1`                         | `false`         |

Example:

```console
cd cluster/examples/kubernetes/edgefs/csi/nfs/examples
kubectl apply -f ./dynamic-nginx.yaml
```

## Apply Edgefs CSI ISCSI driver configuration

Check configuration options and create kubernetes secret for Edgefs CSI ISCSI plugin

```console
cd cluster/examples/kubernetes/edgefs/csi/iscsi
kubectl create secret generic edgefs-iscsi-csi-driver-config --from-file=./edgefs-iscsi-csi-driver-config.yaml
```

## Deploy Edgefs CSI ISCSI driver

After secret is created successfully, deploy EdgeFS CSI plugin, provisioner, attacher and snapshotter using the following command

```console
cd cluster/examples/kubernetes/edgefs/csi/iscsi
kubectl apply -f edgefs-iscsi-csi-driver.yaml
```

There should be number of EdgeFS CSI ISCSI plugin PODs available running as a DaemonSet:

```console
...
NAMESPACE     NAME                             READY   STATUS    RESTARTS   AGE
default       edgefs-iscsi-csi-controller-0    4/4     Running   0          12s
default       edgefs-iscsi-csi-node-26464      2/2     Running   0          12s
default       edgefs-iscsi-csi-node-p5r58      2/2     Running   0          12s
default       edgefs-iscsi-csi-node-ptn2m      2/2     Running   0          12s
...
```

At this point configuration is all ready and available for consumption by applications.

## Pre-provisioned volumes (ISCSI)

This method allows to use already created exports in EdgeFS ISCSI services. This method keeps exports provisioned after application PODs terminated.
Read more on how to create PersistentVolume specification for pre-provisioned volumes:

[Link to Pre-provisioned volumes manifest specification](https://kubernetes-csi.github.io/docs/Usage.html#pre-provisioned-volumes)

To test creation and mount pre-provisioned volume to pod execute example:

> **NOTE**: Make sure that `volumeHandle: segment:service@cluster/tenant/bucket/lun` in nginx.yaml already exist on EdgeFS cluster and served via any Edgefs ISCSI service. Any volumeHandle's parameters may be omitted and will be substituted via CSI configuration file parameters.

Example:

```console
cd cluster/examples/kubernetes/edgefs/csi/iscsi/examples
kubectl apply -f ./preprovisioned-edgefs-volume-nginx.yaml
```

## Dynamically provisioned volumes (ISCSI)

For dynamic volume provisioning, the administrator needs to set up a _StorageClass_ pointing to the driver.
In this case Kubernetes generates volume name automatically (for example `pvc-ns-cfc67950-fe3c-11e8-a3ca-005056b857f8`).
Default driver configuration may be overwritten in `parameters` section:

[Link to dynamically provisioned volumes specification](https://kubernetes-csi.github.io/docs/Usage.html#dynamic-provisioning)

### Note for dynamically provisioned volumes

For dynamically provisioned volumes, Kubernetes will generate volume names automatically
(for example pvc-871068ed-8b5d-11e8-9dae-005056b37cb2).
To pass additional creation parameters, you can add them as parameters to your StorageClass definition.

Example:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: edgefs-iscsi-csi-storageclass
provisioner: io.edgefs.csi.nfs
parameters:
  segment: rook-edgefs
  service: iscsi01
  cluster: cltest
  tenant: test
  bucket: bk1
  encryption: true
```

### Parameters

> **NOTE**: Parameters and their options are case sensitive and should be in lower case.

| `Name`       | `Description`                                             | `Allowed values`                       | `Default value` |
| ------------ | --------------------------------------------------------- | -------------------------------------- | --------------- |
| `segment`    | Edgefs cluster namespace for specific StorageClass or PV. |                                        | `rook-edgefs`   |
| `service`    | Edgefs cluster service if not defined in secret           |                                        |                 |
| `cluster`    | Edgefs cluster namespace if not defined in secret         |                                        |                 |
| `tenant`     | Edgefs tenant  namespace if not defined in secret         |                                        |                 |
| `bucket`     | Edgefs bucket namespace if not defined in secret          |                                        |                 |
| `chunksize`  | Chunk size for actual volume, in bytes                    | should be power of two                 | `16384`         |
| `blocksize`  | Blocksize size for actual volume, in bytes                | should be power of two                 | `4096`          |
| `fsType`     | New volume's filesystem type                              | `ext4`, `ext3`, `xfs`                  | `ext4`          |
| `acl`        | Volume acl restrictions                                   |                                        | `all`           |
| `ec`         | Enables ccow erasure coding for volume                    | `true`, `false`, `0`, `1`              | `false`         |
| `ecmode`     | Set ccow erasure mode data mode (If 'ec' option enabled)  | `4:2:rs` ,`6:2:rs`, `4:2:rs`, `9:3:rs` | `4:2:rs`        |
| `encryption` | Enables encryption for volume                             | `true`, `false`, `0`, `1`              | `false`         |

Example:

```console
cd cluster/examples/kubernetes/edgefs/csi/nfs/examples
kubectl apply -f ./dynamic-nginx.yaml
```

## EdgeFS CSI ISCSI driver snapshots and clones

### Getting information about existing snapshots

```console
# snapshot classes
kubectl get volumesnapshotclasses.snapshot.storage.k8s.io

# snapshot list
kubectl get volumesnapshots.snapshot.storage.k8s.io

# volumesnapshotcontents
kubectl get volumesnapshotcontents.snapshot.storage.k8s.io
```

### To create volume's clone from existing snapshot you should:

* Create snapshotter StorageClass [Example yaml](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/edgefs/csi/iscsi/examples/snapshots/snapshot-class.yaml)
* Have an existing PVC based on EdgeFS ISCSI LUN
* Take snapshot from volume [Example yaml](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/edgefs/csi/iscsi/examples/snapshots/create-snapshot.yaml)
* Clone volume from existing snapshot [Example yaml](https://github.com/rook/rook/tree/master/cluster/examples/kubernetes/edgefs/csi/iscsi/examples/snapshots/nginx-snapshot-clone-volume.yaml)

## Troubleshooting and log collection

For details about other configuration and deployment of NFS, ISCSI and EdgeFS CSI plugin, see Wiki pages:

* [Quick Start Guide](https://github.com/Nexenta/edgefs-csi/wiki/EdgeFS-CSI-Quick-Start-Guide)

Please submit an issue at: [Issues](https://github.com/Nexenta/edgefs-csi/issues)

## Troubleshooting

* Show installed drivers:

```console
kubectl get csidrivers.csi.storage.k8s.io
kubectl describe csidrivers.csi.storage.k8s.io
```

* Error:

```console
MountVolume.MountDevice failed for volume "pvc-ns-<...>" :
driver name io.edgefs.csi.iscsi not found in the list of registered CSI drivers
```

  Make sure _kubelet_ is configured with `--root-dir=/var/lib/kubelet`, otherwise update paths in the driver yaml file
  ([all requirements](https://github.com/kubernetes-csi/docs/blob/387dce893e59c1fcf3f4192cbea254440b6f0f07/book/src/Setup.md#enabling-features)).

* "VolumeSnapshotDataSource" feature gate is disabled:

```console
vim /var/lib/kubelet/config.yaml
# ...
# featureGates:
#   VolumeSnapshotDataSource: true
# ...
vim /etc/kubernetes/manifests/kube-apiserver.yaml
# ...
#     - --feature-gates=VolumeSnapshotDataSource=true
# ...
```

* Driver logs (for ISCSI driver, to get NFS driver logs substitute iscsi to nfs)

```console
kubectl logs -f edgefs-iscsi-csi-controller-0 driver
kubectl logs -f $(kubectl get pods | awk '/edgefs-iscsi-csi-node-/ {print $1;exit}') driver
# combine all pods:
kubectl get pods | awk '/edgefs-iscsi-csi-node-/ {system("kubectl logs " $1 " driver &")}'
```

* Show termination message in case driver failed to run:

<!-- {% raw %} -->
```console
kubectl get pod edgefs-iscsi-csi-controller-0 -o go-template="{{range .status.containerStatuses}}{{.lastState.terminated.message}}{{end}}"
```
<!-- {% endraw %} -->
