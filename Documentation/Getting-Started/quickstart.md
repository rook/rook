---
title: Quickstart
---

Welcome to Rook! We hope you have a great experience installing the Rook **cloud-native storage orchestrator** platform to enable highly available, durable Ceph storage in Kubernetes clusters.

Don't hesitate to ask questions in our [Slack channel](https://rook-io.slack.com). Sign up for the Rook Slack [here](https://slack.rook.io).

This guide will walk through the basic setup of a Ceph cluster and enable K8s applications to consume block, object, and file storage.

**Always use a virtual machine when testing Rook. Never use a host system where local devices may mistakenly be consumed.**

## Kubernetes Version

Kubernetes versions **v1.26** through **v1.31** are supported.

## CPU Architecture

Architectures released are `amd64 / x86_64` and `arm64`.

## Prerequisites

To check if a Kubernetes cluster is ready for `Rook`, see the [prerequisites](Prerequisites/prerequisites.md).

To configure the Ceph storage cluster, at least one of these local storage options are required:

* Raw devices (no partitions or formatted filesystem)
* Raw partitions (no formatted filesystem)
* LVM Logical Volumes (no formatted filesystem)
* Encrypted devices (no formatted filesystem)
* Multipath devices (no formatted filesystem)
* Persistent Volumes available from a storage class in `block` mode

## TL;DR

A simple Rook cluster is created for Kubernetes with the following `kubectl` commands and [example manifests](https://github.com/rook/rook/blob/master/deploy/examples).

```console
$ git clone --single-branch --branch master https://github.com/rook/rook.git
cd rook/deploy/examples
kubectl create -f crds.yaml -f common.yaml -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, applications can consume [block, object, or file](#storage) storage.

## Deploy the Rook Operator

The first step is to deploy the Rook operator.

!!! important
    The [Rook Helm Chart](../Helm-Charts/operator-chart.md) is available to deploy the operator instead of creating the below manifests.

!!! note
    Check that the [example yaml files](https://github.com/rook/rook/blob/master/deploy/examples) are from a tagged release of Rook.

!!! note
    These steps are for a standard production Rook deployment in Kubernetes. For Openshift, testing, or more options, see the [example configurations documentation](example-configurations.md).

```console
cd deploy/examples
kubectl create -f crds.yaml -f common.yaml -f operator.yaml

# verify the rook-ceph-operator is in the `Running` state before proceeding
kubectl -n rook-ceph get pod
```

Before starting the operator in production, consider these settings:

1. Some Rook features are disabled by default. See the [operator.yaml](https://github.com/rook/rook/blob/master/deploy/examples/operator.yaml) for these and other advanced settings.
    1. Device discovery: Rook will watch for new devices to configure if the `ROOK_ENABLE_DISCOVERY_DAEMON` setting is enabled, commonly used in bare metal clusters.
    2. Node affinity and tolerations: The CSI driver by default will run on any node in the cluster. To restrict the CSI driver affinity, several settings are available.
2. If deploying Rook into a namespace other than the default `rook-ceph`, see the topic on
[using an alternative namespace](../Storage-Configuration/Advanced/ceph-configuration.md#using-alternate-namespaces).

## Cluster Environments

The Rook documentation is focused around starting Rook in a variety of environments. While creating the cluster in this guide, consider these example cluster manifests:

* [cluster.yaml](https://github.com/rook/rook/blob/master/deploy/examples/cluster.yaml): Cluster settings for a production cluster running on bare metal. Requires at least three worker nodes.
* [cluster-on-pvc.yaml](https://github.com/rook/rook/blob/master/deploy/examples/cluster-on-pvc.yaml): Cluster settings for a production cluster running in a dynamic cloud environment.
* [cluster-test.yaml](https://github.com/rook/rook/blob/master/deploy/examples/cluster-test.yaml): Cluster settings for a test environment such as minikube.

See the [Ceph example configurations](example-configurations.md) for more details.

## Create a Ceph Cluster

Now that the Rook operator is running we can create the Ceph cluster.

!!! important
    The [Rook Cluster Helm Chart](../Helm-Charts/ceph-cluster-chart.md) is available to deploy the operator instead of creating the below manifests.

!!! important
    For the cluster to survive reboots, set the `dataDirHostPath` property that is valid for the hosts. For more settings, see the documentation on [configuring the cluster](../CRDs/Cluster/ceph-cluster-crd.md).

Create the cluster:

```console
kubectl create -f cluster.yaml
```

Verify the cluster is running by viewing the pods in the `rook-ceph` namespace.

The number of osd pods will depend on the number of nodes in the cluster and the number of devices configured.
For the default `cluster.yaml` above, one OSD will be created for each available device found on each node.

!!! hint
    If the `rook-ceph-mon`, `rook-ceph-mgr`, or `rook-ceph-osd` pods are not created, please refer to the
    [Ceph common issues](../Troubleshooting/ceph-common-issues.md) for more details and potential solutions.

```console
$ kubectl -n rook-ceph get pod
NAME                                                 READY   STATUS      RESTARTS   AGE
csi-cephfsplugin-provisioner-d77bb49c6-n5tgs         5/5     Running     0          140s
csi-cephfsplugin-provisioner-d77bb49c6-v9rvn         5/5     Running     0          140s
csi-cephfsplugin-rthrp                               3/3     Running     0          140s
csi-rbdplugin-hbsm7                                  3/3     Running     0          140s
csi-rbdplugin-provisioner-5b5cd64fd-nvk6c            6/6     Running     0          140s
csi-rbdplugin-provisioner-5b5cd64fd-q7bxl            6/6     Running     0          140s
rook-ceph-crashcollector-minikube-5b57b7c5d4-hfldl   1/1     Running     0          105s
rook-ceph-mgr-a-64cd7cdf54-j8b5p                     2/2     Running     0          77s
rook-ceph-mgr-b-657d54fc89-2xxw7                     2/2     Running     0          56s
rook-ceph-mon-a-694bb7987d-fp9w7                     1/1     Running     0          105s
rook-ceph-mon-b-856fdd5cb9-5h2qk                     1/1     Running     0          94s
rook-ceph-mon-c-57545897fc-j576h                     1/1     Running     0          85s
rook-ceph-operator-85f5b946bd-s8grz                  1/1     Running     0          92m
rook-ceph-osd-0-6bb747b6c5-lnvb6                     1/1     Running     0          23s
rook-ceph-osd-1-7f67f9646d-44p7v                     1/1     Running     0          24s
rook-ceph-osd-2-6cd4b776ff-v4d68                     1/1     Running     0          25s
rook-ceph-osd-prepare-node1-vx2rz                    0/2     Completed   0          60s
rook-ceph-osd-prepare-node2-ab3fd                    0/2     Completed   0          60s
rook-ceph-osd-prepare-node3-w4xyz                    0/2     Completed   0          60s
```

To verify that the cluster is in a healthy state, connect to the [Rook toolbox](../Troubleshooting/ceph-toolbox.md) and run the
`ceph status` command.

* All mons should be in quorum
* A mgr should be active
* At least three OSDs should be `up` and `in`
* If the health is not `HEALTH_OK`, the warnings or errors should be investigated

```console
$ ceph status
  cluster:
    id:     a0452c76-30d9-4c1a-a948-5d8405f19a7c
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum a,b,c (age 3m)
    mgr:a(active, since 2m), standbys: b
    osd: 3 osds: 3 up (since 1m), 3 in (since 1m)
[]...]
```

!!! hint
    If the cluster is not healthy, please refer to the [Ceph common issues](../Troubleshooting/ceph-common-issues.md) for potential solutions.

## Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:

* **[Block](../Storage-Configuration/Block-Storage-RBD/block-storage.md)**: Create block storage to be consumed by a pod (RWO)
* **[Shared Filesystem](../Storage-Configuration/Shared-Filesystem-CephFS/filesystem-storage.md)**: Create a filesystem to be shared across multiple pods (RWX)
* **[Object](../Storage-Configuration/Object-Storage-RGW/object-storage.md)**: Create an object store that is accessible with an S3 endpoint inside or outside the Kubernetes cluster

## Ceph Dashboard

Ceph has a dashboard to view the status of the cluster. See the [dashboard guide](../Storage-Configuration/Monitoring/ceph-dashboard.md).

## Tools

Create a toolbox pod for full access to a ceph admin client for debugging and troubleshooting the Rook cluster. See the [toolbox documentation](../Troubleshooting/ceph-toolbox.md) for setup and usage information.

The [Rook kubectl plugin](https://github.com/rook/kubectl-rook-ceph) provides commands to view status and troubleshoot issues.

See the [advanced configuration](../Storage-Configuration/Advanced/ceph-configuration.md) document for helpful maintenance and tuning examples.

## Monitoring

Each Rook cluster has built-in metrics collectors/exporters for monitoring with Prometheus.
To configure monitoring, see the [monitoring guide](../Storage-Configuration/Monitoring/ceph-monitoring.md).

## Telemetry

The Rook maintainers would like to receive telemetry reports for Rook clusters.
The data is anonymous and does not include any identifying information.
Enable the telemetry reporting feature with the following command in the toolbox:

```console
ceph telemetry on
```

For more details on what is reported and how your privacy is protected,
see the [Ceph Telemetry Documentation](https://docs.ceph.com/en/latest/mgr/telemetry/).

## Teardown

When finished with the test cluster, see [the cleanup guide](../Storage-Configuration/ceph-teardown.md).
