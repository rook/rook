---
title: Cleanup
weight: 5
indent: true
---

# Cleaning up a Cluster
If you want to tear down the cluster and bring up a new one, be aware of the following resources that will need to be cleaned up:
- `rook-system` namespace: The Rook operator and agent created by `rook-operator.yaml`
- `rook` namespace: The Rook storage cluster created by `rook-cluster.yaml` (the cluster CRD)
- `/var/lib/rook`: Path on each host in the cluster where configuration is cached by the ceph mons and osds

Note that if you changed the default namespaces or paths in the sample yaml files, you will need to adjust these namespaces and paths throughout these instructions.

If you see issues tearing down the cluster, see the [Troubleshooting](#troubleshooting) section below.

If you are tearing down a cluster frequently for development purposes, it is instead recommended to use an environment such as Minikube that can easily be restarted without worrying about any of these steps.

## Delete the Block and File artifacts
First you will need to clean up the resources created on top of the Rook cluster.

These commands will clean up the resources from the [block](block.md#teardown) and [file](filesystem.md#teardown) walkthroughs (unmount volumes, delete volume claims, etc). If you did not complete those parts of the walkthrough, you can skip these instructions:
```console
kubectl delete -f wordpress.yaml
kubectl delete -f mysql.yaml
kubectl delete -n rook pool replicapool
kubectl delete storageclass rook-block
kubectl delete -n kube-system secret rook-admin
kubectl delete -f kube-registry.yaml
```

## Delete the Cluster CRD
After those block and file resources have been cleaned up, you can then delete your Rook cluster. This is important to delete **before removing the Rook operator and agent or else resources may not be cleaned up properly**.
```console
kubectl delete -n rook cluster rook
```

Verify that the cluster CRD has been deleted before continuing to the next step.
```
kubectl -n rook get cluster
```

## Delete the Operator and Agent
This will begin the process of all cluster resources being cleaned up, after which you can delete the rest of the deployment with the following:
```console
kubectl delete -n rook-system daemonset rook-agent
kubectl delete -f rook-operator.yaml
kubectl delete clusterroles rook-agent
kubectl delete clusterrolebindings rook-agent
```

Optionally remove the rook namespace if it is not in use by any other resources.
```
kubectl delete namespace rook
```

## Delete the data on hosts
IMPORTANT: The final cleanup step requires deleting files on each host in the cluster. All files under the `dataDirHostPath` property specified in the cluster CRD will need to be deleted. Otherwise, inconsistent state will remain when a new cluster is started.

Connect to each machine and delete `/var/lib/rook`, or the path specified by the `dataDirHostPath`.

In the future this step will not be necessary when we build on the K8s local storage feature.

If you modified the demo settings, additional cleanup is up to you for devices, host paths, etc.

## Troubleshooting
If the cleanup instructions are not executed in the order above, or you otherwise have difficulty cleaning up the cluster, here are a few things to try.

The most common issue cleaning up the cluster is that the `rook` namespace or the cluster CRD remain indefinitely in the `terminating` state. A namespace cannot be removed until all of its resources are removed, so look at which resources are pending termination.

Look at the pods:
```
kubectl -n rook get pod
```
If a pod is still terminating, you will need to wait or else attempt to forcefully terminate it (`kubectl delete pod <name>`).

Now look at the cluster CRD:
```
kubectl -n rook get cluster
```
If the cluster CRD still exists even though you have executed the delete command earlier, see the next section on removing the finalizer.

### Removing the Cluster CRD Finalizer
When a Cluster CRD is created, a [finalizer](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/#finalizers) is added automatically by the Rook operator. The finalizer will allow the operator to ensure that before the cluster CRD is deleted, all block and file mounts will be cleaned up. Without proper cleanup, pods consuming the storage will be hung indefinitely until a system reboot.

The operator is responsible for removing the finalizer after the mounts have been cleaned up. If for some reason the operator is not able to remove the finalizer (ie. the operator is not running anymore), you can delete the finalizer manually.

```
kubectl -n rook edit cluster rook
```

This will open a text editor (usually `vi`) to allow you to edit the CRD. Look for the `finalizers` element and delete the following line:
```
  - cluster.rook.io
```

Now save the changes and exit the editor. Within a few seconds you should see that the cluster CRD has been deleted and will no longer block other cleanup such as deleting the `rook` namespace.
