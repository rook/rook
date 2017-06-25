rook-operator
=============

Installs [rook-operator](https://github.com/rook/rook) to create/configure/manage Rook clusters atop Kubernetes.

TL;DR
-----

```console
$ helm repo add rook http://charts.rook.io
$ helm install rook/rook-operator
```

Alternatively, to deploy from a local checkout of the rook codebase (until the rook chart repo is deployed):

```console
$ cd demo/helm/rook-operator
$ helm install --name rook-operator --namespace rook .
```

Introduction
------------

This chart bootstraps a [rook-operator](https://github.com/rook/rook) deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

Prerequisites
-------------

- Kubernetes 1.6+ with Beta APIs & ThirdPartyResources enabled

### RBAC

If role-based access control (RBAC) is enabled in your cluster, you may need to give Tiller (the server-side component of Helm) additional permissions. **If RBAC is not enabled, be sure to set `rbacEnable` to `false` when installing the chart.**

1. Create a ServiceAccount for Tiller in the `kube-system` namespace
  ```console
  $ kubectl -n kube-system create sa tiller
  ```

2. Create a ClusterRoleBinding for Tiller

  ```console
  $ kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller
  ```

3. Patch Tiller's Deployment to use the new ServiceAccount

  ```console
  $ kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'
  ```

Installing the Chart
--------------------

To install the chart with the release name `rook-operator`:

```console
$ helm install --name rook-operator --namespace rook-operator rook/rook-operator
```

The command deploys the rook-operator on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

Uninstalling the Chart
----------------------

To uninstall/delete the `rook-operator` deployment:

```console
$ helm delete --purge rook-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

Configuration
-------------

The following tables lists the configurable parameters of the rook-operator chart and their default values.

| Parameter          | Description                          | Default              |
|--------------------|--------------------------------------|----------------------|
| `image.repository` | Image                                | `rook/rook`          |
| `image.tag`        | Image tag                            | `master-latest`      |
| `image.pullPolicy` | Image pull policy                    | `IfNotPresent`       |
| `rbacEnable`       | If true, create & use RBAC resources | `true`               |
| `resources`        | Pod resource requests & limits       | `{}`                 |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example to disable RBAC,

```console
$ helm install --name rook-operator rook/rook-operator --set rbacEnable=false
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm install --name rook-operator rook/rook-operator -f values.yaml
```

> **Tip**: You can use the default [values.yaml](values.yaml)
