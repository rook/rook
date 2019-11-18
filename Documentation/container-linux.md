---
title: Container Linux
weight: 11500
indent: true
---
# Using the Container Linux Update Operator with Rook

When you are using Container Linux (CoreOS) and have the update engine enabled, it could be that a node reboots quickly after another not leaving enough time for the Rook cluster to rebuild. The [Container Linux Update Operator](https://github.com/coreos/container-linux-update-operator) is the solution for this, you can block your nodes to reboot until the Ceph cluster is healthy.

## Prerequisites

* An operational Container Linux Kubernetes cluster (Successfully tested with 1.8.4)
* A working rook cluster
* The update-engine.service systemd unit on each machine should be unmasked, enabled and started in systemd
* The locksmithd.service systemd unit on each machine should be masked and stopped in systemd

## Start the update operator

Proper reading of the README on the [Container Linux Update Operator](https://github.com/coreos/container-linux-update-operator) is necessary. Clone the repo and go in the `examples` directory.

Look for the file named `update-operator.yaml` and update the `command` part of the container from:

```yaml
command:
- "/bin/update-operator"
```

to:

```yaml
command:
- "/bin/update-operator"
- "--before-reboot-annotations"
- "ceph-before-reboot-check"
- "--after-reboot-annotations"
- "ceph-after-reboot-check"
```

You can also add the `-v 6` argument for more extensive logging.

Now create the update-operator by invoking following commands:

```console
kubectl create -f namespace.yaml
kubectl create -f cluster-role.yaml
kubectl create -f cluster-role-binding.yaml
kubectl create -f update-operator.yaml
kubectl create -f update-agent.yaml
```

These files create a new namespace `reboot-coordinator`, configured to listen for the node annotation `ceph-reboot-check`. Now you can create both files in the `cluster/examples/coreos` folder, here's a short description of what each file does:

* `rbac.yaml`: This file contains the necessary RBAC settings.
* `ceph-after-reboot-script.yaml`: This file creates a `ConfigMap` containing a bash script which will be mounted in the `rook-toolbox` image as executable file.
* `ceph-before-reboot-script.yaml`: This file creates a `ConfigMap` containing a bash script which will be mounted in the `rook-toolbox` image as executable file.
* `before-reboot-daemonset.yaml`: This file creates a `DaemonSet` which waits for a node being labeled `before-reboot=true`, runs and checks the Ceph status. If all is correct, it annotates the node with `ceph-before-reboot-check=true`.
* `after-reboot-daemonset.yaml`: This file creates a `DaemonSet` which waits for a node being labeled `after-reboot=true`, runs and unsets the `noout` option for the ceph OSDs. If all is correct, it annotates the node with `ceph-after-reboot-check=true`.

The node annotation `ceph-no-noout=true` can be used to avoid `ceph-before-reboot-check` from setting the OSD `noout` flag. This annotation should only be used when deleting a node from a cluster, this way the cluster starts rebalancing immediately, not waiting for the node to come back up.

```console
kubectl create -f rbac.yaml
kubectl create -f ceph-after-reboot-script.yaml
kubectl create -f ceph-before-reboot-script.yaml
kubectl create -f before-reboot-daemonset.yaml
kubectl create -f after-reboot-daemonset.yaml
```

## Destroy the update operator

To destroy all elements created in this file, run:

```console
kubectl delete -f before-reboot-daemonset.yaml
kubectl delete -f after-reboot-daemonset.yaml
kubectl delete -f ceph-after-reboot-script.yaml
kubectl delete -f ceph-before-reboot-script.yaml
kubectl delete -f rbac.yaml
```

Then you may safely delete the update operator itself:
From the directory of the Container Linux Update Operator you cloned earlier, go again into the `examples` folder and run following commands:

```console
kubectl delete -f update-agent.yaml
kubectl delete -f update-operator.yaml
kubectl delete -f cluster-role-binding.yaml
kubectl delete -f cluster-role.yaml
kubectl delete -f namespace.yaml
```
