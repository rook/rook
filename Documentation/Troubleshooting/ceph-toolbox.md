---
title: Toolbox
---

The Rook toolbox is a container with common tools used for rook debugging and testing.
The toolbox is based on CentOS, so more tools of your choosing can be easily installed with `yum`.

The toolbox can be run in two modes:

1. [Interactive](#interactive-toolbox): Start a toolbox pod where you can connect and execute Ceph commands from a shell
2. [One-time job](#toolbox-job): Run a script with Ceph commands and collect the results from the job log

!!! hint
    Before running the toolbox you should have a running Rook cluster deployed (see the [Quickstart Guide](../Getting-Started/quickstart.md)).

!!! note
    The toolbox is not necessary if you are using [kubectl plugin](kubectl-plugin.md) to execute Ceph commands.

## Interactive Toolbox

The rook toolbox can run as a deployment in a Kubernetes cluster where you can connect and
run arbitrary Ceph commands.

Launch the rook-ceph-tools pod:

```console
kubectl create -f deploy/examples/toolbox.yaml
```

Wait for the toolbox pod to download its container and get to the `running` state:

```console
kubectl -n rook-ceph rollout status deploy/rook-ceph-tools
```

Once the rook-ceph-tools pod is running, you can connect to it with:

```console
kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- bash
```

All available tools in the toolbox are ready for your troubleshooting needs.

**Example**:

* `ceph status`
* `ceph osd status`
* `ceph df`
* `rados df`

When you are done with the toolbox, you can remove the deployment:

```console
kubectl -n rook-ceph delete deploy/rook-ceph-tools
```

## Toolbox Job

If you want to run Ceph commands as a one-time operation and collect the results later from the
logs, you can run a script as a Kubernetes Job. The toolbox job will run a script that is embedded
in the job spec. The script has the full flexibility of a bash script.

In this example, the `ceph status` command is executed when the job is created.
Create the toolbox job:

```console
kubectl create -f deploy/examples/toolbox-job.yaml
```

After the job completes, see the results of the script:

```console
kubectl -n rook-ceph logs -l job-name=rook-ceph-toolbox-job
```
