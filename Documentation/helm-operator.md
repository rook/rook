---
title: Ceph Operator
weight: 10100
indent: true
---

{% include_relative branch.liquid %}

# Ceph Operator Helm Chart

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Ceph clusters on Kubernetes.

## Introduction

This chart bootstraps a [rook-ceph-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

* Kubernetes 1.13+
* Helm 3.x

See the [Helm support matrix](https://helm.sh/docs/topics/version_skew/) for more details.

## Installing

The Ceph Operator helm chart will install the basic components necessary to create a storage platform for your Kubernetes cluster.
1. Install the Helm chart
1. [Create a Rook cluster](quickstart.md#create-a-rook-cluster).

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation. It is recommended that the rook operator be installed into the `rook-ceph` namespace (you will install your clusters into separate namespaces).

Rook currently publishes builds of the Ceph operator to the `release` and `master` channels.

### Release

The release channel is the most recent release of Rook that is considered stable for the community.

```console
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph
```

### Development Build

To deploy from a local build from your development environment:

1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart:

```console
cd deploy/charts/rook-ceph
helm install --create-namespace --namespace rook-ceph rook-ceph .
```

## Uninstalling the Chart

To see the currently installed Rook chart:

```console
helm ls --namespace rook-ceph
```

To uninstall/delete the `rook-ceph` deployment:

```console
helm delete --namespace rook-ceph rook-ceph
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

After uninstalling you may want to clean up the CRDs as described on the [teardown documentation](ceph-teardown.md#removing-the-cluster-crd-finalizer).

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter                           | Description                                                                                                                 | Default                                                   |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `image.repository`                  | Image                                                                                                                       | `rook/ceph`                                               |
| `image.tag`                         | Image tag                                                                                                                   | `master`                                                  |
| `image.pullPolicy`                  | Image pull policy                                                                                                           | `IfNotPresent`                                            |
| `crds.enabled`                      | If true, the helm chart will create the Rook CRDs. Do NOT change to `false` in a running cluster or CRs will be deleted!    | `true`                                                    |
| `rbacEnable`                        | If true, create & use RBAC resources                                                                                        | `true`                                                    |
| `pspEnable`                         | If true, create & use PSP resources                                                                                         | `true`                                                    |
| `resources`                         | Pod resource requests & limits                                                                                              | `{}`                                                      |
| `annotations`                       | Pod annotations                                                                                                             | `{}`                                                      |
| `logLevel`                          | Global log level                                                                                                            | `INFO`                                                    |
| `nodeSelector`                      | Kubernetes `nodeSelector` to add to the Deployment.                                                                         | <none>                                                    |
| `tolerations`                       | List of Kubernetes `tolerations` to add to the Deployment.                                                                  | `[]`                                                      |
| `unreachableNodeTolerationSeconds`  | Delay to use for the node.kubernetes.io/unreachable pod failure toleration to override the Kubernetes default of 5 minutes  | `5s`                                                      |
| `currentNamespaceOnly`              | Whether the operator should watch cluster CRD in its own namespace or not                                                   | `false`                                                   |
| `hostpathRequiresPrivileged`        | Runs Ceph Pods as privileged to be able to write to `hostPath`s in OpenShift with SELinux restrictions.                     | `false`                                                   |
| `discover.priorityClassName`        | The priority class name to add to the discover pods                                                                         | <none>                                                    |
| `discover.toleration`               | Toleration for the discover pods                                                                                            | <none>                                                    |
| `discover.tolerationKey`            | The specific key of the taint to tolerate                                                                                   | <none>                                                    |
| `discover.tolerations`              | Array of tolerations in YAML format which will be added to discover deployment                                              | <none>                                                    |
| `discover.nodeAffinity`             | The node labels for affinity of `discover-agent` (***)                                                                      | <none>                                                    |
| `discover.podLabels`                | Labels to add to the discover pods.                                                                                         | <none>                                                    |
| `csi.enableRbdDriver`               | Enable Ceph CSI RBD driver.                                                                                                 | `true`                                                    |
| `csi.enableCephfsDriver`            | Enable Ceph CSI CephFS driver.                                                                                              | `true`                                                    |
| `csi.enableCephfsSnapshotter`       | Enable Snapshotter in CephFS provisioner pod.                                                                               | `true`                                                    |
| `csi.enableRBDSnapshotter`          | Enable Snapshotter in RBD provisioner pod.                                                                                  | `true`                                                    |
| `csi.pluginPriorityClassName`       | PriorityClassName to be set on csi driver plugin pods.                                                                      | <none>                                                    |
| `csi.provisionerPriorityClassName`  | PriorityClassName to be set on csi driver provisioner pods.                                                                 | <none>                                                    |
| `csi.enableOMAPGenerator`           | EnableOMAP generator deploys omap sidecar in CSI provisioner pod, to enable it set it to true                               | `false`                                                   |
| `csi.rbdFSGroupPolicy`              | Policy for modifying a volume's ownership or permissions when the RBD PVC is being mounted                                  | ReadWriteOnceWithFSType                                   |
| `csi.cephFSFSGroupPolicy`           | Policy for modifying a volume's ownership or permissions when the CephFS PVC is being mounted                               | `None`                                                    |
| `csi.logLevel`                      | Set logging level for csi containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity. | `0`                                                       |
| `csi.provisionerReplicas`           | Set replicas for csi provisioner deployment.                                                                                | `2`                                                       |
| `csi.enableGrpcMetrics`             | Enable Ceph CSI GRPC Metrics.                                                                                               | `false`                                                   |
| `csi.enableCSIHostNetwork`          | Enable Host Networking for Ceph CSI nodeplugins.                                                                            | `false`                                                   |
| `csi.enablePluginSelinuxHostMount`  | Enable Host mount for /etc/selinux directory for Ceph CSI nodeplugins.                                                      | `false`                                                   |
| `csi.provisionerTolerations`        | Array of tolerations in YAML format which will be added to CSI provisioner deployment.                                      | <none>                                                    |
| `csi.provisionerNodeAffinity`       | The node labels for affinity of the CSI provisioner deployment (***)                                                        | <none>                                                    |
| `csi.pluginTolerations`             | Array of tolerations in YAML format which will be added to CephCSI plugin DaemonSet                                         | <none>                                                    |
| `csi.pluginNodeAffinity`            | The node labels for affinity of the CephCSI plugin DaemonSet (***)                                                          | <none>                                                    |
| `csi.rbdProvisionerTolerations`     | Array of tolerations in YAML format which will be added to CephCSI RBD provisioner deployment.                              | <none>                                                    |
| `csi.rbdProvisionerNodeAffinity`    | The node labels for affinity of the CephCSI RBD provisioner deployment (***)                                                | <none>                                                    |
| `csi.rbdPluginTolerations`          | Array of tolerations in YAML format which will be added to CephCSI RBD plugin DaemonSet                                     | <none>                                                    |
| `csi.rbdPluginNodeAffinity`         | The node labels for affinity of the CephCSI RBD plugin DaemonSet (***)                                                      | <none>                                                    |
| `csi.cephFSProvisionerTolerations`  | Array of tolerations in YAML format which will be added to CephCSI CephFS provisioner deployment.                           | <none>                                                    |
| `csi.cephFSProvisionerNodeAffinity` | The node labels for affinity of the CephCSI CephFS provisioner deployment (***)                                             | <none>                                                    |
| `csi.cephFSPluginTolerations`       | Array of tolerations in YAML format which will be added to CephCSI CephFS plugin DaemonSet                                  | <none>                                                    |
| `csi.cephFSPluginNodeAffinity`      | The node labels for affinity of the CephCSI CephFS plugin DaemonSet (***)                                                   | <none>                                                    |
| `csi.csiRBDProvisionerResource`     | CEPH CSI RBD provisioner resource requirement list.                                                                         | <none>                                                    |
| `csi.csiRBDPluginResource`          | CEPH CSI RBD plugin resource requirement list.                                                                              | <none>                                                    |
| `csi.csiCephFSProvisionerResource`  | CEPH CSI CephFS provisioner resource requirement list.                                                                      | <none>                                                    |
| `csi.csiCephFSPluginResource`       | CEPH CSI CephFS plugin resource requirement list.                                                                           | <none>                                                    |
| `csi.cephfsGrpcMetricsPort`         | CSI CephFS driver GRPC metrics port.                                                                                        | `9091`                                                    |
| `csi.cephfsLivenessMetricsPort`     | CSI CephFS driver metrics port.                                                                                             | `9081`                                                    |
| `csi.rbdGrpcMetricsPort`            | Ceph CSI RBD driver GRPC metrics port.                                                                                      | `9090`                                                    |
| `csi.rbdLivenessMetricsPort`        | Ceph CSI RBD driver metrics port.                                                                                           | `8080`                                                    |
| `csi.forceCephFSKernelClient`       | Enable Ceph Kernel clients on kernel < 4.17 which support quotas for Cephfs.                                                | `true`                                                    |
| `csi.kubeletDirPath`                | Kubelet root directory path (if the Kubelet uses a different path for the `--root-dir` flag)                                | `/var/lib/kubelet`                                        |
| `csi.cephcsi.image`                 | Ceph CSI image.                                                                                                             | `quay.io/cephcsi/cephcsi:v3.4.0`                          |
| `csi.rbdPluginUpdateStrategy`       | CSI Rbd plugin daemonset update strategy, supported values are OnDelete and RollingUpdate.                                  | `OnDelete`                                                |
| `csi.cephFSPluginUpdateStrategy`    | CSI CephFS plugin daemonset update strategy, supported values are OnDelete and RollingUpdate.                               | `OnDelete`                                                |
| `csi.registrar.image`               | Kubernetes CSI registrar image.                                                                                             | `k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.3.0` |
| `csi.resizer.image`                 | Kubernetes CSI resizer image.                                                                                               | `k8s.gcr.io/sig-storage/csi-resizer:v1.3.0`               |
| `csi.provisioner.image`             | Kubernetes CSI provisioner image.                                                                                           | `k8s.gcr.io/sig-storage/csi-provisioner:v3.0.0`           |
| `csi.snapshotter.image`             | Kubernetes CSI snapshotter image.                                                                                           | `k8s.gcr.io/sig-storage/csi-snapshotter:v4.2.0`           |
| `csi.attacher.image`                | Kubernetes CSI Attacher image.                                                                                              | `k8s.gcr.io/sig-storage/csi-attacher:v3.3.0`              |
| `csi.cephfsPodLabels`               | Labels to add to the CSI CephFS Pods.                                                                                       | <none>                                                    |
| `csi.rbdPodLabels`                  | Labels to add to the CSI RBD Pods.                                                                                          | <none>                                                    |
| `csi.volumeReplication.enabled`     | Enable Volume Replication.                                                                                                  | `false`                                                   |
| `csi.volumeReplication.image`       | Volume Replication Controller image.                                                                                        | `quay.io/csiaddons/volumereplication-operator:v0.1.0`     |
| `admissionController.tolerations`   | Array of tolerations in YAML format which will be added to admission controller deployment.                                 | <none>                                                    |
| `admissionController.nodeAffinity`  | The node labels for affinity of the admission controller deployment (***)                                                   | <none>                                                    |
| `allowMultipleFilesystems`          | **(experimental in  Octopus (v15))** Allows multiple filesystems to be deployed to a Ceph cluster.                          | `false`                                                   |

&ast; &ast; &ast; `nodeAffinity` and `*NodeAffinity` options should have the format `"role=storage,rook; storage=ceph"` or `storage=;role=rook-example` or `storage=;` (_checks only for presence of key_)

### Command Line

You can pass the settings with helm command line parameters. Specify each parameter using the
`--set key=value[,key=value]` argument to `helm install`.

### Settings File

Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while installing the chart.

```console
helm install --namespace rook-ceph rook-ceph rook-release/rook-ceph -f values.yaml
```

For example settings, see [values.yaml](https://github.com/rook/rook/tree/{{ branchName }}/deploy/charts/rook-ceph/values.yaml)
