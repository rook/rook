---
title: Ceph Operator
weight: 10100
indent: true
---

# Ceph Operator Helm Chart

Installs [rook](https://github.com/rook/rook) to create, configure, and manage Ceph clusters on Kubernetes.

## Introduction

This chart bootstraps a [rook-ceph-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

* Kubernetes 1.11+

### RBAC

If role-based access control (RBAC) is enabled in your cluster, you may need to give Tiller (the server-side component of Helm) additional permissions. **If RBAC is not enabled, be sure to set `rbacEnable` to `false` when installing the chart.**

```console
# Create a ServiceAccount for Tiller in the `kube-system` namespace
kubectl --namespace kube-system create sa tiller

# Create a ClusterRoleBinding for Tiller
kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller

# Patch Tiller's Deployment to use the new ServiceAccount
kubectl --namespace kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
```

## Installing

The Ceph Operator helm chart will install the basic components necessary to create a storage platform for your Kubernetes cluster.
After the helm chart is installed, you will need to [create a Rook cluster](ceph-quickstart.md#create-a-rook-cluster).

The `helm install` command deploys rook on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation. It is recommended that the rook operator be installed into the `rook-ceph` namespace (you will install your clusters into separate namespaces).

Rook currently publishes builds of the Ceph operator to the `release` and `master` channels.

### Release

The release channel is the most recent release of Rook that is considered stable for the community.

```console
helm repo add rook-release https://charts.rook.io/release
```

For Helm `v3.x`:

```console
helm install --namespace rook-ceph rook-ceph rook-release/rook-ceph
```

For Helm `v2.x` the `--name` flag needs to be specified:

```console
helm install --namespace rook-ceph --name rook-ceph rook-release/rook-ceph
```

### Master

The master channel includes the latest commits, with all automated tests green. Historically it has been very stable, though it is only recommended for testing.
The critical point to consider is that upgrades are not supported to or from master builds.

To install the helm chart from master, you will need to pass the specific version returned by the `search` command.

```console
helm repo add rook-master https://charts.rook.io/master

# For Helm v3.x
helm search repo rook-ceph --versions
helm install --namespace rook-ceph rook-ceph rook-master/rook-ceph --version <version>

# For Helm v2.x
helm search rook-ceph
helm install --namespace rook-ceph --name rook-ceph rook-master/rook-ceph --version <version>
```

For example to install version `v1.3.0.860.g80ff2bb`:

```console
helm install --namespace rook-ceph rook-ceph rook-master/rook-ceph --version v1.3.0.860.g80ff2bb
```

For Helm `v2.x` the `--name` flag must be specified instead of just `rook-ceph`, e.g., `--name rook-ceph`.

### Development Build

To deploy from a local build from your development environment:

1. Build the Rook docker image: `make`
1. Copy the image to your K8s cluster, such as with the `docker save` then the `docker load` commands
1. Install the helm chart:

```console
cd cluster/charts/rook-ceph
helm install --namespace rook-ceph rook-ceph .
```
For Helm `v2.x` the `--name` flag must be specified instead of just `rook-ceph`, e.g., `--name rook-ceph`.

## Uninstalling the Chart

To uninstall/delete the `rook-ceph` deployment:

```console
helm delete --purge rook-ceph
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter                          | Description                                                                                                                 | Default                                                   |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `image.repository`                 | Image                                                                                                                       | `rook/ceph`                                               |
| `image.tag`                        | Image tag                                                                                                                   | `master`                                                  |
| `image.pullPolicy`                 | Image pull policy                                                                                                           | `IfNotPresent`                                            |
| `rbacEnable`                       | If true, create & use RBAC resources                                                                                        | `true`                                                    |
| `pspEnable`                        | If true, create & use PSP resources                                                                                         | `true`                                                    |
| `resources`                        | Pod resource requests & limits                                                                                              | `{}`                                                      |
| `annotations`                      | Pod annotations                                                                                                             | `{}`                                                      |
| `logLevel`                         | Global log level                                                                                                            | `INFO`                                                    |
| `nodeSelector`                     | Kubernetes `nodeSelector` to add to the Deployment.                                                                         | <none>                                                    |
| `tolerations`                      | List of Kubernetes `tolerations` to add to the Deployment.                                                                  | `[]`                                                      |
| `unreachableNodeTolerationSeconds` | Delay to use for the node.kubernetes.io/unreachable pod failure toleration to override the Kubernetes default of 5 minutes  | `5s`                                                      |
| `currentNamespaceOnly`             | Whether the operator should watch cluster CRD in its own namespace or not                                                   | `false`                                                   |
| `hostpathRequiresPrivileged`       | Runs Ceph Pods as privileged to be able to write to `hostPath`s in OpenShift with SELinux restrictions.                     | `false`                                                   |
| `mon.healthCheckInterval`          | The frequency for the operator to check the mon health                                                                      | `45s`                                                     |
| `mon.monOutTimeout`                | The time to wait before failing over an unhealthy mon                                                                       | `600s`                                                    |
| `discover.priorityClassName`       | The priority class name to add to the discover pods                                                                         | <none>                                                    |
| `discover.toleration`              | Toleration for the discover pods                                                                                            | <none>                                                    |
| `discover.tolerationKey`           | The specific key of the taint to tolerate                                                                                   | <none>                                                    |
| `discover.tolerations`             | Array of tolerations in YAML format which will be added to discover deployment                                              | <none>                                                    |
| `discover.nodeAffinity`            | The node labels for affinity of `discover-agent` (***)                                                                      | <none>                                                    |
| `csi.enableRbdDriver`              | Enable Ceph CSI RBD driver.                                                                                                 | `true`                                                    |
| `csi.enableCephfsDriver`           | Enable Ceph CSI CephFS driver.                                                                                              | `true`                                                    |
| `csi.pluginPriorityClassName`      | PriorityClassName to be set on csi driver plugin pods.                                                                      | <none>                                                    |
| `csi.provisionerPriorityClassName` | PriorityClassName to be set on csi driver provisioner pods.                                                                 | <none>                                                    |
| `csi.logLevel`                     | Set logging level for csi containers. Supported values from 0 to 5. 0 for general useful logs, 5 for trace level verbosity. | `0`                                                       |
| `csi.enableGrpcMetrics`            | Enable Ceph CSI GRPC Metrics.                                                                                               | `true`                                                    |
| `csi.provisionerTolerations`       | Array of tolerations in YAML format which will be added to CSI provisioner deployment.                                      | <none>                                                    |
| `csi.provisionerNodeAffinity`      | The node labels for affinity of the CSI provisioner deployment (***)                                                        | <none>                                                    |
| `csi.pluginTolerations`            | Array of tolerations in YAML format which will be added to Ceph CSI plugin DaemonSet                                        | <none>                                                    |
| `csi.pluginNodeAffinity`           | The node labels for affinity of the Ceph CSI plugin DaemonSet (***)                                                         | <none>                                                    |
| `csi.csiRBDProvisionerResource`    | CEPH CSI RBD provisioner resource requirement list.                                                                         | <none>                                                    |
| `csi.csiRBDPluginResource`         | CEPH CSI RBD plugin resource requirement list.                                                                              | <none>                                                    |
| `csi.csiCephFSProvisionerResource` | CEPH CSI CephFS provisioner resource requirement list.                                                                      | <none>                                                    |
| `csi.csiCephFSPluginResource`      | CEPH CSI CephFS plugin resource requirement list.                                                                           | <none>                                                    |
| `csi.cephfsGrpcMetricsPort`        | CSI CephFS driver GRPC metrics port.                                                                                        | `9091`                                                    |
| `csi.cephfsLivenessMetricsPort`    | CSI CephFS driver metrics port.                                                                                             | `9081`                                                    |
| `csi.rbdGrpcMetricsPort`           | Ceph CSI RBD driver GRPC metrics port.                                                                                      | `9090`                                                    |
| `csi.rbdLivenessMetricsPort`       | Ceph CSI RBD driver metrics port.                                                                                           | `8080`                                                    |
| `csi.forceCephFSKernelClient`      | Enable Ceph Kernel clients on kernel < 4.17 which support quotas for Cephfs.                                                | `true`                                                    |
| `csi.kubeletDirPath`               | Kubelet root directory path (if the Kubelet uses a different path for the `--root-dir` flag)                                | `/var/lib/kubelet`                                        |
| `csi.cephcsi.image`                | Ceph CSI image.                                                                                                             | `quay.io/cephcsi/cephcsi:v3.1.1`                          |
| `csi.rbdPluginUpdateStrategy`      | CSI Rbd plugin daemonset update strategy, supported values are OnDelete and RollingUpdate.                                  | `OnDelete`                                                |
| `csi.cephFSPluginUpdateStrategy`   | CSI CephFS plugin daemonset update strategy, supported values are OnDelete and RollingUpdate.                               | `OnDelete`                                                |
| `csi.registrar.image`              | Kubernetes CSI registrar image.                                                                                             | `k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.0.1` |
| `csi.resizer.image`                | Kubernetes CSI resizer image.                                                                                               | `k8s.gcr.io/sig-storage/csi-resizer:v1.0.0`               |
| `csi.provisioner.image`            | Kubernetes CSI provisioner image.                                                                                           | `k8s.gcr.io/sig-storage/csi-provisioner:v2.0.0`           |
| `csi.snapshotter.image`            | Kubernetes CSI snapshotter image.                                                                                           | `k8s.gcr.io/sig-storage/csi-snapshotter:v3.0.0`           |
| `csi.attacher.image`               | Kubernetes CSI Attacher image.                                                                                              | `k8s.gcr.io/sig-storage/csi-attacher:v3.0.0`              |
| `agent.flexVolumeDirPath`          | Path where the Rook agent discovers the flex volume plugins (*)                                                             | `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/`    |
| `agent.libModulesDirPath`          | Path where the Rook agent should look for kernel modules (*)                                                                | `/lib/modules`                                            |
| `agent.mounts`                     | Additional paths to be mounted in the agent container (**)                                                                  | <none>                                                    |
| `agent.mountSecurityMode`          | Mount Security Mode for the agent.                                                                                          | `Any`                                                     |
| `agent.priorityClassName`          | The priority class name to add to the agent pods                                                                            | <none>                                                    |
| `agent.toleration`                 | Toleration for the agent pods                                                                                               | <none>                                                    |
| `agent.tolerationKey`              | The specific key of the taint to tolerate                                                                                   | <none>                                                    |
| `agent.tolerations`                | Array of tolerations in YAML format which will be added to agent deployment                                                 | <none>                                                    |
| `agent.nodeAffinity`               | The node labels for affinity of `rook-agent` (***)                                                                          | <none>                                                    |
| `admissionController.tolerations`  | Array of tolerations in YAML format which will be added to admission controller deployment.                                 | <none>                                                    |
| `admissionController.nodeAffinity` | The node labels for affinity of the admission controller deployment (***)                                                   | <none>                                                    |

&ast; For information on what to set `agent.flexVolumeDirPath` to, please refer to the [Rook flexvolume documentation](flexvolume.md)

&ast; &ast; `agent.mounts` should have this format `mountname1=/host/path:/container/path,mountname2=/host/path2:/container/path2`

&ast; &ast; &ast; `nodeAffinity` and `*NodeAffinity` options should have the format `"role=storage,rook; storage=ceph"` or `storage=;role=rook-example` or `storage=;` (_checks only for presence of key_)

### Command Line

You can pass the settings with helm command line parameters. Specify each parameter using the
`--set key=value[,key=value]` argument to `helm install`. For example, the following command will install rook where RBAC is not enabled.

For Helm `v2.x` the `--name` flag must be specified instead of just `rook-ceph`, e.g., `--name rook-ceph`.

```console
helm install --namespace rook-ceph rook-ceph rook-release/rook-ceph --set rbacEnable=false
```

### Settings File

Alternatively, a yaml file that specifies the values for the above parameters (`values.yaml`) can be provided while installing the chart.

For Helm `v2.x` the `--name` flag must be specified instead of just `rook-ceph`, e.g., `--name rook-ceph`.

```console
helm install --namespace rook-ceph rook-ceph rook-release/rook-ceph -f values.yaml
```

Here are the sample settings to get you started.

```yaml
image:
  prefix: rook
  repository: rook/ceph
  tag: master
  pullPolicy: IfNotPresent

resources:
  limits:
    cpu: 100m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 256Mi

rbacEnable: true
pspEnable: true
```
