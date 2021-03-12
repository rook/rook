---
title: Ceph Storage
weight: 300
indent: true
---

{% include_relative branch.liquid %}


# Ceph Storage Quickstart

This guide will walk you through the basic setup of a Ceph cluster and enable you to consume block, object, and file storage
from other pods running in your cluster.

## Minimum Version

Kubernetes **v1.11** or higher is supported by Rook.

**Important** If you are using K8s 1.15 or older, you will need to create a different version of the Rook CRDs. Create the `crds.yaml` found in the [pre-k8s-1.16](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/pre-k8s-1.16) subfolder of the example manifests.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

In order to configure the Ceph storage cluster, at least one of these local storage options are required:
- Raw devices (no partitions or formatted filesystems)
- Raw partitions (no formatted filesystem)
- PVs available from a storage class in `block` mode

You can confirm whether your partitions or devices are formatted filesystems with the following command.

```console
lsblk -f
```
>```
>NAME                  FSTYPE      LABEL UUID                                   MOUNTPOINT
>vda
>└─vda1                LVM2_member       >eSO50t-GkUV-YKTH-WsGq-hNJY-eKNf-3i07IB
>  ├─ubuntu--vg-root   ext4              c2366f76-6e21-4f10-a8f3-6776212e2fe4   /
>  └─ubuntu--vg-swap_1 swap              9492a3dc-ad75-47cd-9596-678e8cf17ff9   [SWAP]
>vdb
>```

If the `FSTYPE` field is not empty, there is a filesystem on top of the corresponding device. In this case, you can use vdb for Ceph and can't use vda and its partitions.

## TL;DR

If you're feeling lucky, a simple Rook cluster can be created with the following kubectl commands and [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph). For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).

```console
$ git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd rook/cluster/examples/kubernetes/ceph
kubectl create -f crds.yaml -f common.yaml -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

### Cluster Environments

The Rook documentation is focused around starting Rook in a production environment. Examples are also
provided to relax some settings for test environments. When creating the cluster later in this guide, consider these example cluster manifests:
- [cluster.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster.yaml): Cluster settings for a production cluster running on bare metal. Requires at least three worker nodes.
- [cluster-on-pvc.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster-on-pvc.yaml): Cluster settings for a production cluster running in a dynamic cloud environment.
- [cluster-test.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/cluster-test.yaml): Cluster settings for a test environment such as minikube.

See the [Ceph examples](ceph-examples.md) for more details.

## Deploy the Rook Operator

The first step is to deploy the Rook operator. Check that you are using the [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph) that correspond to your release of Rook. For more options, see the [examples documentation](ceph-examples.md).

```console
cd cluster/examples/kubernetes/ceph
kubectl create -f crds.yaml -f common.yaml -f operator.yaml

# verify the rook-ceph-operator is in the `Running` state before proceeding
kubectl -n rook-ceph get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

Before you start the operator in production, there are some settings that you may want to consider:
1. If you are using kubernetes v1.15 or older you need to create CRDs found here `/cluster/examples/kubernetes/ceph/pre-k8s-1.16/crd.yaml`.
   The apiextension v1beta1 version of CustomResourceDefinition was deprecated in Kubernetes v1.16.
2. Consider if you want to enable certain Rook features that are disabled by default. See the [operator.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/operator.yaml) for these and other advanced settings.
   1. Device discovery: Rook will watch for new devices to configure if the `ROOK_ENABLE_DISCOVERY_DAEMON` setting is enabled, commonly used in bare metal clusters.
   2. Flex driver: The flex driver is deprecated in favor of the CSI driver, but can still be enabled with the `ROOK_ENABLE_FLEX_DRIVER` setting.
   3. Node affinity and tolerations: The CSI driver by default will run on any node in the cluster. To configure the CSI driver affinity, several settings are available.

If you wish to deploy into a namespace other than the default `rook-ceph`, see the
[Ceph advanced configuration section](ceph-advanced-configuration.md#using-alternate-namespaces) on the topic.

## Create a Rook Ceph Cluster

Now that the Rook operator is running we can create the Ceph cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property that is valid for your hosts. For more settings, see the documentation on [configuring the cluster](ceph-cluster-crd.md).

Create the cluster:

```console
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook-ceph` namespace. You should be able to see the following pods once they are all running.
The number of osd pods will depend on the number of nodes in the cluster and the number of devices configured.
If you did not modify the `cluster.yaml` above, it is expected that one OSD will be created per node.
The CSI, `rook-ceph-agent` (flex driver), and `rook-discover` pods are also optional depending on your settings.

> If the `rook-ceph-mon`, `rook-ceph-mgr`, or `rook-ceph-osd` pods are not created, please refer to the
> [Ceph common issues](ceph-common-issues.md) for more details and potential solutions.

```console
kubectl -n rook-ceph get pod
```

>```
>NAME                                                 READY   STATUS      RESTARTS   AGE
>csi-cephfsplugin-provisioner-d77bb49c6-n5tgs         5/5     Running     0          140s
>csi-cephfsplugin-provisioner-d77bb49c6-v9rvn         5/5     Running     0          140s
>csi-cephfsplugin-rthrp                               3/3     Running     0          140s
>csi-rbdplugin-hbsm7                                  3/3     Running     0          140s
>csi-rbdplugin-provisioner-5b5cd64fd-nvk6c            6/6     Running     0          140s
>csi-rbdplugin-provisioner-5b5cd64fd-q7bxl            6/6     Running     0          140s
>rook-ceph-crashcollector-minikube-5b57b7c5d4-hfldl   1/1     Running     0          105s
>rook-ceph-mgr-a-64cd7cdf54-j8b5p                     1/1     Running     0          77s
>rook-ceph-mon-a-694bb7987d-fp9w7                     1/1     Running     0          105s
>rook-ceph-mon-b-856fdd5cb9-5h2qk                     1/1     Running     0          94s
>rook-ceph-mon-c-57545897fc-j576h                     1/1     Running     0          85s
>rook-ceph-operator-85f5b946bd-s8grz                  1/1     Running     0          92m
>rook-ceph-osd-0-6bb747b6c5-lnvb6                     1/1     Running     0          23s
>rook-ceph-osd-1-7f67f9646d-44p7v                     1/1     Running     0          24s
>rook-ceph-osd-2-6cd4b776ff-v4d68                     1/1     Running     0          25s
>rook-ceph-osd-prepare-node1-vx2rz                    0/2     Completed   0          60s
>rook-ceph-osd-prepare-node2-ab3fd                    0/2     Completed   0          60s
>rook-ceph-osd-prepare-node3-w4xyz                    0/2     Completed   0          60s
>```

To verify that the cluster is in a healthy state, connect to the [Rook toolbox](ceph-toolbox.md) and run the
`ceph status` command.

* All mons should be in quorum
* A mgr should be active
* At least one OSD should be active
* If the health is not `HEALTH_OK`, the warnings or errors should be investigated

```console
ceph status
```
>```
>  cluster:
>    id:     a0452c76-30d9-4c1a-a948-5d8405f19a7c
>    health: HEALTH_OK
>
>  services:
>    mon: 3 daemons, quorum a,b,c (age 3m)
>    mgr: a(active, since 2m)
>    osd: 3 osds: 3 up (since 1m), 3 in (since 1m)
>...
>```

If the cluster is not healthy, please refer to the [Ceph common issues](ceph-common-issues.md) for more details and potential solutions.

## Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:

* **[Block](ceph-block.md)**: Create block storage to be consumed by a pod
* **[Object](ceph-object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster
* **[Shared Filesystem](ceph-filesystem.md)**: Create a filesystem to be shared across multiple pods

## Ceph Dashboard

Ceph has a dashboard in which you can view the status of your cluster. Please see the [dashboard guide](ceph-dashboard.md) for more details.

## Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](ceph-toolbox.md) for setup and usage information. Also see our [advanced configuration](ceph-advanced-configuration.md) document for helpful maintenance and tuning examples.

## Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./ceph-monitoring.md).

## Teardown

When you are done with the test cluster, see [these instructions](ceph-teardown.md) to clean up the cluster.
