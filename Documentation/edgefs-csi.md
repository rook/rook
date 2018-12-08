---
title: EdgeFS CSI provisioner
weight: 47
indent: true
---

## EdgeFS Rook integrated CSI driver, provisioner and attacher

[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, and attacher for EdgeFS Scale-Out NFS

## Overview

EdgeFS CSI plugins implements interface between CSI enabled Container Orchestrator (CO) and EdgeFS local cluster site. It allows dynamic and static provisioning of EdgeFS NFS exports or iSCSI LUNs, and attaching them to stateful application workloads. With EdgeFS NFS implementation, I/O load can be spread-out across multiple PODs, thus eliminating networking I/O bottlenecks of classic single-node NFS. Current implementation of EdgeFS CSI plugin was tested in Kubernetes environment (requires Kubernetes 1.11+), however the code is Kubernetes version agnostic and should be able to run with any CSI enabled CO.

## Deployment

Ensure that [Rook EdgeFS cluster](edgefs-cluster-crd.md) up and running.

Configure new NFS service (lets say named as "nfs01") via `efscli` and create [Rook EdgeFS NFS resouce](edgefs-nfs-crd.md).

Configure CSI driver options and cluster discovery using [kubernetes secret json file](/cluster/examples/kubernetes/edgefs/csi/secret/cluster-config.json) as an example.

Secret file configuration options example:
```
{
	"k8sEdgefsNamespace": "rook-edgefs",
	"cluster": "cltest",
	"tenant": "test",
	"serviceFilter": "nfs01",
	"username": "admin",
	"password": "admin"
}
```

| Name                | Description           | Default value | Required |
|---------------------|-----------------------|---------------|----------|
| username            | EdgeFS gRPC API server privileged user | "admin" | true |
| password            | EdgeFS gRPC API server password | "admin" | true |
| cluster             | EdgeFS cluster namespace also known as 'region' |  | false |
| tenant              | EdgeFS tenant isolated namespace  |  | false |
| serviceFilter       | List of comma delimeted allowed service names to filter |  "" means all services allowed | false |
| k8sEdgefsNamespace  | Rook EdgeFS cluster namespace | | true |

By using `k8sEdgefsNamespace` parameter, driver is capable of detecting ClusterIPs and Endpoint IPs to provision and attach volumes.

Check configuration options and create kubernetes secret for NexentaEdge CSI plugin
```
cd cluster/examples/kubernetes/edgefs/csi/secret
kubectl create secret generic rook-edgefs-cluster --from-file=./cluster-config.json
```

After secret is created successfully, deploy EdgeFS CSI plugin, provisioner and attacher using the following command
```
cd cluster/examples/kubernetes/edgefs/csi
kubectl apply -f .
```

Note that for Kubernetes versions >= v1.12.1 CSI architecutre introduced kind=CSIDriver CRD. To use earlier version change pwd to 'k8s-prior-12.1' subdirectory.

There should be number of EdgeFS CSI plugin PODs available running as a DaemonSet
```
...
NAMESPACE     NAME                                    READY     STATUS    RESTARTS   AGE
default       csi-attacher-nedgeplugin-0              2/2       Running   0          18s
default       csi-provisioner-nedgeplugin-0           2/2       Running   0          18s
default       edgefs-csi-plugin-7s6wc                 2/2       Running   0          19s
...
```

At this point configuration is all ready and available for consumption by appliations.

## Pre-provisioned volumes (NFS)

This method allows to use already created exports in EdgeFS services. This method keeps exports provisioned after application PODs terminated.
Read more on how to create PersistentVolume specification for pre-provisioned volumes:

[link to Pre-provisioned volumes manifest specification](https://kubernetes-csi.github.io/docs/Usage.html#pre-provisioned-volumes)

To test creation and mount pre-provisioned volume to pod execute example

#### Note:
Make sure that volumeHandle: clus1/ten1/buk1 in nginx.yaml already exist on EdgeFS cluster

Examples:
```
cd cluster/examples/kubernetes/edgefs/csi/examples
kubectl apply -f ./pre-provisioned-nginx.yaml #one pod with pre-provisioned volume
kubectl apply -f ./deployment.yaml            # 10 pods deployment shares one EdgeFS bucket
```

## Dynamically provisioned volumes (NFS)

To setup the system for dynamic provisioning, administrator needs to setup a StorageClass pointing to the CSI driver’s external-provisioner and specifying any parameters required by the driver

[link to dynamically provisioned volumes specification](https://kubernetes-csi.github.io/docs/Usage.html#dynamic-provisioning)

#### Note:
For dynamically provisioned volumes kubernetes will generate volume name automatically
(for example pvc-871068ed-8b5d-11e8-9dae-005056b37cb2)
Additional creation options should be passed as parameters in storage class definition i.e :

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-sc-nedgeplugin
provisioner: edgefs-csi-plugin
parameters:
  tenant: ten1
  encryption: true
```

### Options:

| Name      | Description           | Allowed values            | Default value |
|-----------|-----------------------|---------------------------|---------------|
| cluster   | NexentaEdge cluster namespace if not defined in secret |       |  |
| tenant    | NexentaEdge tenant  namespace if not defined in secret |       |  |
| chunksize | Chunk size for actual volume, in bytes | should be power of two | 1048576 bytes |
| acl       | Volume acl restrictions |                                       | all |
| ec        | Enables ccow erasure coding for volume | true, false, 0, 1 | false |
| ecmode    | Set ccow erasure mode data mode (If 'ec' option enabled) | "4:2:rs" ,"6:2:rs", "9:3:rs" | 6:2:rs |
| encryption | Enables encryption for volume | true, false, 0, 1 | false |

#### Note:
Options are case sensitive and should be in lower case

Example:
```
cd cluster/examples/kubernetes/edgefs/csi/examples
kubectl apply -f ./dynamic-nginx.yaml
```

## Troubleshooting and log collection

For details about other configuration and deployment of EdgeFS CSI plugin, see Wiki pages:

* [Quick Start Guide](https://github.com/Nexenta/edgefs-csi/wiki/EdgeFS-CSI-Quick-Start-Guide)

Please submit an issue at: [Issues](https://github.com/Nexenta/edgefs-csi/issues)

### Tips

In case any problems using EdgeFS CSI driver
1. Check CSI plugin pods state
```
kubectl describe pod edgefs-csi-plugin-xxxxx
```
2. Check provisioned pods state
```
kubectl describe pods nginx
```
3. Check CSI plugin logs
```
kubectl logs csi-attacher-nedgeplugin-0 -c nfs
kubectl logs csi-provisioner-nedgeplugin-0 -c nfs
kubectl logs edgefs-csi-plugin-j8ljf -c nfs
```
