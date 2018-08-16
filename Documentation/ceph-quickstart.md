---
title: Ceph Storage
weight: 3
indent: true
---

# Ceph Storage Quickstart

This guide will walk you through the basic setup of a Ceph cluster and enable you to consume block, object, and file storage
from other pods running in your cluster.

## Minimum Version

Kubernetes **v1.7** or higher is supported by Rook.

## Prerequisites

To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

If you are using `dataDirHostPath` to persist rook data on kubernetes hosts, make sure your host has at least 5GB of space available on the specified path.

## TL;DR

If you're feeling lucky, a simple Rook cluster can be created with the following kubectl commands. For the more detailed install, skip to the next section to [deploy the Rook operator](#deploy-the-rook-operator).
```
cd cluster/examples/kubernetes/ceph
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

After the cluster is running, you can create [block, object, or file](#storage) storage to be consumed by other applications in your cluster.

## Deploy the Rook Operator

The first step is to deploy the Rook system components, which include the Rook agent running on each node in your cluster as well as Rook operator pod.

```bash
cd cluster/examples/kubernetes/ceph
kubectl create -f operator.yaml

# verify the rook-ceph-operator, rook-ceph-agent, and rook-discover pods are in the `Running` state before proceeding
kubectl -n rook-ceph-system get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

---
### **Restart Kubelet**
**(K8S 1.7.x only)**

For versions of Kubernetes prior to 1.8, the Kubelet process on all nodes will require a restart after the Rook operator and Rook agents have been deployed. As part of their initial setup, the Rook agents deploy and configure a Flexvolume plugin in order to integrate with Kubernetes' volume controller framework. In Kubernetes v1.8+, the [dynamic Flexvolume plugin discovery](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#dynamic-plugin-discovery) will find and initialize our plugin, but in older versions of Kubernetes a manual restart of the Kubelet will be required.

---

## Create a Rook Cluster

Now that the Rook operator, agent, and discover pods are running, we can create the Rook cluster. For the cluster to survive reboots,
make sure you set the `dataDirHostPath` property. For more settings, see the documentation on [configuring the cluster](ceph-cluster-crd.md).


Save the cluster spec as `cluster.yaml`:

```yaml
#################################################################################
# This example first defines some necessary namespace and RBAC security objects.
# The actual Ceph Cluster CRD example can be found at the bottom of this example.
#################################################################################
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cluster
  namespace: rook-ceph
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: rook-ceph
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph-system
---
# Allow the pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-cluster
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-cluster
  namespace: rook-ceph
---
#################################################################################
# The Ceph Cluster CRD example
#################################################################################
apiVersion: ceph.rook.io/v1beta1
kind: Cluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  dashboard:
    enabled: true
  storage:
    useAllNodes: true
    useAllDevices: false
    config:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
```

Create the cluster:

```bash
kubectl create -f cluster.yaml
```

Use `kubectl` to list pods in the `rook` namespace. You should be able to see the following pods once they are all running.
The number of osd pods will depend on the number of nodes in the cluster and the number of devices and directories configured.

```bash
$ kubectl -n rook-ceph get pod
NAME                                      READY     STATUS      RESTARTS   AGE
rook-ceph-mgr-a-75cc4ccbf4-t8qtx          1/1       Running     0          24m
rook-ceph-mon0-72vx7                      1/1       Running     0          25m
rook-ceph-mon1-rrpm6                      1/1       Running     0          24m
rook-ceph-mon2-zff9r                      1/1       Running     0          24m
rook-ceph-osd-id-0-5fd8cb9747-dvlsb       1/1       Running     0          23m
rook-ceph-osd-id-1-84dc695b48-r5mhf       1/1       Running     0          23m
rook-ceph-osd-id-2-558878cd84-cnp67       1/1       Running     0          23m
rook-ceph-osd-prepare-minikube-wq4f5      0/1       Completed   0          24m
```

# Storage

For a walkthrough of the three types of storage exposed by Rook, see the guides for:
- **[Block](block.md)**: Create block storage to be consumed by a pod
- **[Object](object.md)**: Create an object store that is accessible inside or outside the Kubernetes cluster
- **[Shared File System](filesystem.md)**: Create a file system to be shared across multiple pods

# Ceph Dashboard

Ceph has a dashboard in which you can view the status of your cluster. Please see the [dashboard guide](ceph-dashboard.md) for more details.

# Tools

We have created a toolbox container that contains the full suite of Ceph clients for debugging and troubleshooting your Rook cluster.  Please see the [toolbox readme](toolbox.md) for setup and usage information. Also see our [advanced configuration](advanced-configuration.md) document for helpful maintenance and tuning examples.

# Monitoring

Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).
To learn how to set up monitoring for your Rook cluster, you can follow the steps in the [monitoring guide](./monitoring.md).

# Teardown

When you are done with the test cluster, see [these instructions](ceph-teardown.md) to clean up the cluster.
