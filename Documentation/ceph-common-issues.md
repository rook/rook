---
title: Ceph Common Issues
weight: 10600
indent: true
---

# Ceph Common Issues

Many of these problem cases are hard to summarize down to a short phrase that adequately describes the problem. Each problem will start with a bulleted list of symptoms. Keep in mind that all symptoms may not apply depending on the configuration of Rook. If the majority of the symptoms are seen there is a fair chance you are experiencing that problem.

If after trying the suggestions found on this page and the problem is not resolved, the Rook team is very happy to help you troubleshoot the issues in their Slack channel. Once you have [registered for the Rook Slack](https://slack.rook.io), proceed to the General channel to ask for assistance.

## Table of Contents <!-- omit in toc -->

* [Troubleshooting Techniques](#troubleshooting-techniques)
* [Pod Using Ceph Storage Is Not Running](#pod-using-ceph-storage-is-not-running)
* [Cluster failing to service requests](#cluster-failing-to-service-requests)
* [Monitors are the only pods running](#monitors-are-the-only-pods-running)
* [PVCs stay in pending state](#pvcs-stay-in-pending-state)
* [OSD pods are failing to start](#osd-pods-are-failing-to-start)
* [OSD pods are not created on my devices](#osd-pods-are-not-created-on-my-devices)
* [Node hangs after reboot](#node-hangs-after-reboot)
* [Rook Agent modprobe exec format error](#rook-agent-modprobe-exec-format-error)
* [Rook Agent rbd module missing error](#rook-agent-rbd-module-missing-error)
* [Using multiple shared filesystem (CephFS) is attempted on a kernel version older than 4.7](#using-multiple-shared-filesystem-cephfs-is-attempted-on-a-kernel-version-older-than-47)
* [Activate log to file for a particular Ceph daemon](#activate-log-to-file-for-a-particular-ceph-daemon)
* [Flex storage class versus Ceph CSI storage class](#flex-storage-class-versus-ceph-csi-storage-class)

## Troubleshooting Techniques

There are two main categories of information you will need to investigate issues in the cluster:

1. Kubernetes status and logs documented [here](common-issues.md)
1. Ceph cluster status (see upcoming [Ceph tools](#ceph-tools) section)

### Ceph Tools

After you verify the basic health of the running pods, next you will want to run Ceph tools for status of the storage components. There are two ways to run the Ceph tools, either in the Rook toolbox or inside other Rook pods that are already running.

* Logs on a specific node to find why a PVC is failing to mount:
  * Rook agent errors around the attach/detach: `kubectl logs -n rook-ceph <rook-ceph-agent-pod>`
* See the [log collection topic](ceph-advanced-configuration.md#log-collection) for a script that will help you gather the logs
* Other artifacts:
  * The monitors that are expected to be in quorum: `kubectl -n <cluster-namespace> get configmap rook-ceph-mon-endpoints -o yaml | grep data`

#### Tools in the Rook Toolbox

The [rook-ceph-tools pod](./ceph-toolbox.md) provides a simple environment to run Ceph tools. Once the pod is up and running, connect to the pod to execute Ceph commands to evaluate that current state of the cluster.

```console
kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}') bash
```

#### Ceph Commands

Here are some common commands to troubleshoot a Ceph cluster:

* `ceph status`
* `ceph osd status`
* `ceph osd df`
* `ceph osd utilization`
* `ceph osd pool stats`
* `ceph osd tree`
* `ceph pg stat`

The first two status commands provide the overall cluster health. The normal state for cluster operations is HEALTH_OK, but will still function when the state is in a HEALTH_WARN state. If you are in a WARN state, then the cluster is in a condition that it may enter the HEALTH_ERROR state at which point *all* disk I/O operations are halted. If a HEALTH_WARN state is observed, then one should take action to prevent the cluster from halting when it enters the HEALTH_ERROR state.

There are many Ceph sub-commands to look at and manipulate Ceph objects, well beyond the scope this document. See the [Ceph documentation](https://docs.ceph.com/) for more details of gathering information about the health of the cluster. In addition, there are other helpful hints and some best practices located in the [Advanced Configuration section](advanced-configuration.md). Of particular note, there are scripts for collecting logs and gathering OSD information there.

## Pod Using Ceph Storage Is Not Running

### Symptoms

* The pod that is configured to use Rook storage is stuck in the `ContainerCreating` status
* `kubectl describe pod` for the pod mentions one or more of the following:
  * `PersistentVolumeClaim is not bound`
  * `timeout expired waiting for volumes to attach/mount`
* `kubectl -n rook-ceph get pod` shows the rook-ceph-agent pods in a `CrashLoopBackOff` status

### Possible Solutions Summary

* `rook-ceph-agent` pod is in a `CrashLoopBackOff` status because it cannot deploy its driver on a read-only filesystem: [Flexvolume configuration pre-reqs](./k8s-pre-reqs.md#flexvolume-configuration)
* Persistent Volume and/or Claim are failing to be created and bound: [Volume Creation](#volume-creation)
* `rook-ceph-agent` pod is failing to mount and format the volume: [Rook Agent Mounting](#volume-mounting)

### Investigation Details

If you see some of the symptoms above, it's because the requested Rook storage for your pod is not being created and mounted successfully.
In this walkthrough, we will be looking at the wordpress mysql example pod that is failing to start.

To first confirm there is an issue, you can run commands similar to the following and you should see similar output (note that some of it has been omitted for brevity):

```console
> kubectl get pod
NAME                              READY     STATUS              RESTARTS   AGE
wordpress-mysql-918363043-50pjr   0/1       ContainerCreating   0          1h

> kubectl describe pod wordpress-mysql-918363043-50pjr
...
Events:
  FirstSeen	LastSeen	Count	From			SubObjectPath	Type		Reason			Message
  ---------	--------	-----	----			-------------	--------	------			-------
  1h		1h		3	default-scheduler			Warning		FailedScheduling	PersistentVolumeClaim is not bound: "mysql-pv-claim" (repeated 2 times)
  1h		35s		36	kubelet, 172.17.8.101			Warning		FailedMount		Unable to mount volumes for pod "wordpress-mysql-918363043-50pjr_default(08d14e75-bd99-11e7-bc4c-001c428b9fc8)": timeout expired waiting for volumes to attach/mount for pod "default"/"wordpress-mysql-918363043-50pjr". list of unattached/unmounted volumes=[mysql-persistent-storage]
  1h		35s		36	kubelet, 172.17.8.101			Warning		FailedSync		Error syncing pod
```

To troubleshoot this, let's walk through the volume provisioning steps in order to confirm where the failure is happening.

#### Ceph Agent Deployment

The `rook-ceph-agent` pods are responsible for mapping and mounting the volume from the cluster onto the node that your pod will be running on.
If the `rook-ceph-agent` pod is not running then it cannot perform this function.

Below is an example of the `rook-ceph-agent` pods failing to get to the `Running` status because they are in a `CrashLoopBackOff` status:

```console
> kubectl -n rook-ceph get pod
NAME                                  READY     STATUS             RESTARTS   AGE
rook-ceph-agent-ct5pj                 0/1       CrashLoopBackOff   16         59m
rook-ceph-agent-zb6n9                 0/1       CrashLoopBackOff   16         59m
rook-operator-2203999069-pmhzn        1/1       Running            0          59m
```

If you see this occurring, you can get more details about why the `rook-ceph-agent` pods are continuing to crash with the following command and its sample output:

```console
> kubectl -n rook-ceph get pod -l app=rook-ceph-agent -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[0].lastState.terminated.message}{"\n"}{end}'
rook-ceph-agent-ct5pj	mkdir /usr/libexec/kubernetes: read-only filesystem
rook-ceph-agent-zb6n9	mkdir /usr/libexec/kubernetes: read-only filesystem
```

From the output above, we can see that the agents were not able to bind mount to `/usr/libexec/kubernetes` on the host they are scheduled to run on.
For some environments, this default path is read-only and therefore a better path must be provided to the agents.

First, clean up the agent deployment with:

```console
kubectl -n rook-ceph delete daemonset rook-ceph-agent
```

Once the `rook-ceph-agent` pods are gone, **follow the instructions in the [Flexvolume configuration pre-reqs](./k8s-pre-reqs.md#flexvolume-configuration)** to ensure a good value for `--volume-plugin-dir` has been provided to the Kubelet.
After that has been configured, and the Kubelet has been restarted, start the agent pods up again by restarting `rook-operator`:

```console
kubectl -n rook-ceph delete pod -l app=rook-operator
```

#### Volume Creation

The volume must first be created in the Rook cluster and then bound to a volume claim before it can be mounted to a pod.
Let's confirm that with the following commands and their output:

```console
> kubectl get pv
NAME                                       CAPACITY   ACCESSMODES   RECLAIMPOLICY   STATUS     CLAIM                    STORAGECLASS   REASON    AGE
pvc-9f273fbc-bdbf-11e7-bc4c-001c428b9fc8   20Gi       RWO           Delete          Bound      default/mysql-pv-claim   rook-ceph-block               25m

> kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   STORAGECLASS   AGE
mysql-pv-claim   Bound     pvc-9f273fbc-bdbf-11e7-bc4c-001c428b9fc8   20Gi       RWO           rook-ceph-block     25m
```

Both your volume and its claim should be in the `Bound` status.
If one or neither of them is not in the `Bound` status, then look for details of the issue in the `rook-operator` logs:

```console
kubectl -n rook-ceph logs `kubectl -n rook-ceph -l app=rook-operator get pods -o jsonpath='{.items[*].metadata.name}'`
```

If the volume is failing to be created, there should be details in the `rook-operator` log output, especially those tagged with `op-provisioner`.

One common cause for the `rook-operator` failing to create the volume is when the `clusterNamespace` field of the `StorageClass` doesn't match the **namespace** of the Rook cluster, as described in [#1502](https://github.com/rook/rook/issues/1502).
In that scenario, the `rook-operator` log would show a failure similar to the following:

```console
2018-03-28 18:58:32.041603 I | op-provisioner: creating volume with configuration {pool:replicapool clusterNamespace:rook-ceph fstype:}
2018-03-28 18:58:32.041728 I | exec: Running command: rbd create replicapool/pvc-fd8aba49-32b9-11e8-978e-08002762c796 --size 20480 --cluster=rook --conf=/var/lib/rook/rook-ceph/rook.config --keyring=/var/lib/rook/rook-ceph/client.admin.keyring
E0328 18:58:32.060893       5 controller.go:801] Failed to provision volume for claim "default/mysql-pv-claim" with StorageClass "rook-ceph-block": Failed to create rook block image replicapool/pvc-fd8aba49-32b9-11e8-978e-08002762c796: failed to create image pvc-fd8aba49-32b9-11e8-978e-08002762c796 in pool replicapool of size 21474836480: Failed to complete '': exit status 1. global_init: unable to open config file from search list /var/lib/rook/rook-ceph/rook.config
. output:
```

The solution is to ensure that the [`clusterNamespace`](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/storageclass.yaml#L20) field matches the **namespace** of the Rook cluster when creating the `StorageClass`.

#### Volume Mounting

The final step in preparing Rook storage for your pod is for the `rook-ceph-agent` pod to mount and format it.
If all the preceding sections have been successful or inconclusive, then take a look at the `rook-ceph-agent` pod logs for further clues.
You can determine which `rook-ceph-agent` is running on the same node that your pod is scheduled on by using the `-o wide` output, then you can get the logs for that `rook-ceph-agent` pod similar to the example below:

```console
> kubectl -n rook-ceph get pod -o wide
NAME                                  READY     STATUS    RESTARTS   AGE       IP             NODE
rook-ceph-agent-h6scx                 1/1       Running   0          9m        172.17.8.102   172.17.8.102
rook-ceph-agent-mp7tn                 1/1       Running   0          9m        172.17.8.101   172.17.8.101
rook-operator-2203999069-3tb68        1/1       Running   0          9m        10.32.0.7      172.17.8.101

> kubectl -n rook-ceph logs rook-ceph-agent-h6scx
2017-10-30 23:07:06.984108 I | rook: starting Rook v0.5.0-241.g48ce6de.dirty with arguments '/usr/local/bin/rook agent'
[...]
```

In the `rook-ceph-agent` pod logs, you may see a snippet similar to the following:

```console
Failed to complete rbd: signal: interrupt.
```

In this case, the agent waited for the `rbd` command but it did not finish in a timely manner so the agent gave up and stopped it.
This can happen for multiple reasons, but using `dmesg` will likely give you insight into the root cause.
If `dmesg` shows something similar to below, then it means you have an old kernel that can't talk to the cluster:

```console
libceph: mon2 10.205.92.13:6789 feature set mismatch, my 4a042a42 < server's 2004a042a42, missing 20000000000
```

If `uname -a` shows that you have a kernel version older than `3.15`, you'll need to perform **one** of the following:

* Disable some Ceph features by starting the [rook toolbox](./ceph-toolbox.md) and running `ceph osd crush tunables bobtail`
* Upgrade your kernel to `3.15` or later.

#### Filesystem Mounting

In the `rook-ceph-agent` pod logs, you may see a snippet similar to the following:

```console
2017-11-07 00:04:37.808870 I | rook-flexdriver: WARNING: The node kernel version is 4.4.0-87-generic, which do not support multiple ceph filesystems. The kernel version has to be at least 4.7. If you have multiple ceph filesystems, the result could be inconsistent
```

This will happen in kernels with versions older than 4.7, where the option `mds_namespace` is not supported. This option is used to specify a filesystem namespace.

In this case, if there is only one filesystem in the Rook cluster, there should be no issues and the mount should succeed. If you have more than one filesystem, inconsistent results may arise and the filesystem mounted may not be the one you specified.

If the issue is still not resolved from the steps above, please come chat with us on the **#general** channel of our [Rook Slack](https://slack.rook.io).
We want to help you get your storage working and learn from those lessons to prevent users in the future from seeing the same issue.

## Cluster failing to service requests

### Symptoms

* Execution of the `ceph` command hangs
* PersistentVolumes are not being created
* Large amount of slow requests are blocking
* Large amount of stuck requests are blocking
* One or more MONs are restarting periodically

### Investigation

Create a [rook-ceph-tools pod](./ceph-toolbox.md) to investigate the current state of CEPH. Here is an example of what one might see. In this case the `ceph status` command would just hang so a CTRL-C needed to be sent.

```console
$ kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[0].metadata.name}') bash
root@rook-ceph-tools:/# ceph status
^CCluster connection interrupted or timed out
```

Another indication is when one or more of the MON pods restart frequently. Note the 'mon107' that has only been up for 16 minutes in the following output.

```console
$ kubectl -n rook-ceph get all -o wide --show-all
NAME                                 READY     STATUS    RESTARTS   AGE       IP               NODE
po/rook-ceph-mgr0-2487684371-gzlbq   1/1       Running   0          17h       192.168.224.46   k8-host-0402
po/rook-ceph-mon107-p74rj            1/1       Running   0          16m       192.168.224.28   k8-host-0402
rook-ceph-mon1-56fgm                 1/1       Running   0          2d        192.168.91.135   k8-host-0404
rook-ceph-mon2-rlxcd                 1/1       Running   0          2d        192.168.123.33   k8-host-0403
rook-ceph-osd-bg2vj                  1/1       Running   0          2d        192.168.91.177   k8-host-0404
rook-ceph-osd-mwxdm                  1/1       Running   0          2d        192.168.123.31   k8-host-0403
```

### Solution

What is happening here is that the MON pods are restarting and one or more of the Ceph daemons are not getting configured with the proper cluster information. This is commonly the result of not specifying a value for `dataDirHostPath` in your Cluster CRD.

The `dataDirHostPath` setting specifies a path on the local host for the CEPH daemons to store configuration and data. Setting this to a path like `/var/lib/rook`, reapplying your Cluster CRD and restarting all the CEPH daemons (MON, MGR, OSD, RGW) should solve this problem. After the CEPH daemons have been restarted, it is advisable to restart the [rook-tool pod](./toolbox.md).

## Monitors are the only pods running

### Symptoms

* Rook operator is running
* Either a single mon starts or the mons skip letters, specifically named `a`, `d`, and `f`
* No mgr, osd, or other daemons are created

### Investigation

When the operator is starting a cluster, the operator will start one mon at a time and check that they are healthy before continuing to bring up all three mons.
If the first mon is not detected healthy, the operator will continue to check until it is healthy. If the first mon fails to start, a second and then a third
mon may attempt to start. However, they will never form quorum and the orchestration will be blocked from proceeding.

The likely causes for the mon health not being detected:

* The operator pod does not have network connectivity to the mon pod
* The mon pod is failing to start
* One or more mon pods are in running state, but are not able to form quorum

#### Operator fails to connect to the mon

First look at the logs of the operator to confirm if it is able to connect to the mons.

```console
kubectl -n rook-ceph logs -l app=rook-operator
```

Likely you will see an error similar to the following that the operator is timing out when connecting to the mon. The last command is `ceph mon_status`,
followed by a timeout message five minutes later.

```console
2018-01-21 21:47:32.375833 I | exec: Running command: ceph mon_status --cluster=rook --conf=/var/lib/rook/rook-ceph/rook.config --keyring=/var/lib/rook/rook-ceph/client.admin.keyring --format json --out-file /tmp/442263890
2018-01-21 21:52:35.370533 I | exec: 2018-01-21 21:52:35.071462 7f96a3b82700  0 monclient(hunting): authenticate timed out after 300
2018-01-21 21:52:35.071462 7f96a3b82700  0 monclient(hunting): authenticate timed out after 300
2018-01-21 21:52:35.071524 7f96a3b82700  0 librados: client.admin authentication error (110) Connection timed out
2018-01-21 21:52:35.071524 7f96a3b82700  0 librados: client.admin authentication error (110) Connection timed out
[errno 110] error connecting to the cluster
```

The error would appear to be an authentication error, but it is misleading. The real issue is a timeout.

#### Solution

If you see the timeout in the operator log, verify if the mon pod is running (see the next section).
If the mon pod is running, check the network connectivity between the operator pod and the mon pod.
A common issue is that the CNI is not configured correctly.

#### Failing mon pod

Second we need to verify if the mon pod started successfully.

```console
$ kubectl -n rook-ceph get pod -l app=rook-ceph-mon
NAME                                READY     STATUS               RESTARTS   AGE
rook-ceph-mon-a-69fb9c78cd-58szd    1/1       CrashLoopBackOff     2          47s
```

If the mon pod is failing as in this example, you will need to look at the mon pod status or logs to determine the cause. If the pod is in a crash loop backoff state,
you should see the reason by describing the pod.

```console
# The pod shows a termination status that the keyring does not match the existing keyring
$ kubectl -n rook-ceph describe pod -l mon=rook-ceph-mon0
...
    Last State:    Terminated
      Reason:    Error
      Message:    The keyring does not match the existing keyring in /var/lib/rook/rook-ceph-mon0/data/keyring.
                    You may need to delete the contents of dataDirHostPath on the host from a previous deployment.
...
```

See the solution in the next section regarding cleaning up the `dataDirHostPath` on the nodes.

#### Three mons named a, d, and f

If you see the three mons running with the names `a`, `d`, and `f`, they likely did not form quorum even though they are running.

```console
NAME                               READY   STATUS    RESTARTS   AGE
rook-ceph-mon-a-7d9fd97d9b-cdq7g   1/1     Running   0          10m
rook-ceph-mon-d-77df8454bd-r5jwr   1/1     Running   0          9m2s
rook-ceph-mon-f-58b4f8d9c7-89lgs   1/1     Running   0          7m38s
```

#### Solution

This is a common problem reinitializing the Rook cluster when the local directory used for persistence has **not** been purged.
This directory is the `dataDirHostPath` setting in the cluster CRD and is typically set to `/var/lib/rook`.
To fix the issue you will need to delete all components of Rook and then delete the contents of `/var/lib/rook` (or the directory specified by `dataDirHostPath`) on each of the hosts in the cluster.
Then when the cluster CRD is applied to start a new cluster, the rook-operator should start all the pods as expected.

> **IMPORTANT: Deleting the `dataDirHostPath` folder is destructive to the storage. Only delete the folder if you are trying to permanently purge the Rook cluster.**

See the [Cleanup Guide](ceph-teardown.md) for more details.

## PVCs stay in pending state

### Symptoms

* When you create a PVC based on a rook storage class, it stays pending indefinitely

For the Wordpress example, you might see two PVCs in pending state.

```console
$ kubectl get pvc
NAME             STATUS    VOLUME   CAPACITY   ACCESS MODES   STORAGECLASS      AGE
mysql-pv-claim   Pending                                      rook-ceph-block   8s
wp-pv-claim      Pending                                      rook-ceph-block   16s
```

### Investigation

There are two common causes for the PVCs staying in pending state:

1. There are no OSDs in the cluster
2. The operator is not running or is otherwise not responding to the request to create the block image

#### Confirm if there are OSDs

To confirm if you have OSDs in your cluster, connect to the [Rook Toolbox](ceph-toolbox.md) and run the `ceph status` command.
You should see that you have at least one OSD `up` and `in`. The minimum number of OSDs required depends on the
`replicated.size` setting in the pool created for the storage class. In a "test" cluster, only one OSD is required
(see `storageclass-test.yaml`). In the production storage class example (`storageclass.yaml`), three OSDs would be required.

```console
$ ceph status
  cluster:
    id:     a0452c76-30d9-4c1a-a948-5d8405f19a7c
    health: HEALTH_OK

  services:
    mon: 3 daemons, quorum a,b,c (age 11m)
    mgr: a(active, since 10m)
    osd: 1 osds: 1 up (since 46s), 1 in (since 109m)
```

#### OSD Prepare Logs

If you don't see the expected number of OSDs, let's investigate why they weren't created.
On each node where Rook looks for OSDs to configure, you will see an "osd prepare" pod.

```console
$ kubectl -n rook-ceph get pod -l app=rook-ceph-osd-prepare
NAME                                 ...  READY   STATUS      RESTARTS   AGE
rook-ceph-osd-prepare-minikube-9twvk   0/2     Completed   0          30m
```

See the section on [why OSDs are not getting created](#osd-pods-are-not-created-on-my-devices) to investigate the logs.

#### Operator unresponsiveness

Lastly, if you have OSDs `up` and `in`, the next step is to confirm the operator is responding to the requests.
Look in the Operator pod logs around the time when the PVC was created to confirm if the request is being raised.
If the operator does not show requests to provision the block image, the operator may be stuck on some other operation.
In this case, restart the operator pod to get things going again.

### Solution

If the "osd prepare" logs didn't give you enough clues about why the OSDs were not being created,
please review your [cluster.yaml](eph-cluster-crd.html#storage-selection-settings) configuration.
The common misconfigurations include:

* If `useAllDevices: true`, Rook expects to find local devices attached to the nodes. If no devices are found, no OSDs will be created.
* If `useAllDevices: false`, OSDs will only be created if `directories` or a `deviceFilter` are specified.
* Only local devices attached to the nodes will be configurable by Rook. In other words, the devices must show up under `/dev`.
  * The devices must not have any partitions or filesystems on them. Rook will only configure raw devices. Partitions are not yet supported.

## OSD pods are failing to start

### Symptoms

* OSD pods are failing to start
* You have started a cluster after tearing down another cluster

### Investigation

When an OSD starts, the device or directory will be configured for consumption. If there is an error with the configuration, the pod will crash and you will see the CrashLoopBackoff
status for the pod. Look in the osd pod logs for an indication of the failure.

```console
$ kubectl -n rook-ceph logs rook-ceph-osd-fl8fs
...
```

One common case for failure is that you have re-deployed a test cluster and some state may remain from a previous deployment.
If your cluster is larger than a few nodes, you may get lucky enough that the monitors were able to start and form quorum. However, now the OSDs pods may fail to start due to the
old state. Looking at the OSD pod logs you will see an error about the file already existing.

```console
$ kubectl -n rook-ceph logs rook-ceph-osd-fl8fs
...
2017-10-31 20:13:11.187106 I | mkfs-osd0: 2017-10-31 20:13:11.186992 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) _read_fsid unparsable uuid
2017-10-31 20:13:11.187208 I | mkfs-osd0: 2017-10-31 20:13:11.187026 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) _setup_block_symlink_or_file failed to create block symlink to /dev/disk/by-partuuid/651153ba-2dfc-4231-ba06-94759e5ba273: (17) File exists
2017-10-31 20:13:11.187233 I | mkfs-osd0: 2017-10-31 20:13:11.187038 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) mkfs failed, (17) File exists
2017-10-31 20:13:11.187254 I | mkfs-osd0: 2017-10-31 20:13:11.187042 7f0059d62e00 -1 OSD::mkfs: ObjectStore::mkfs failed with error (17) File exists
2017-10-31 20:13:11.187275 I | mkfs-osd0: 2017-10-31 20:13:11.187121 7f0059d62e00 -1  ** ERROR: error creating empty object store in /var/lib/rook/osd0: (17) File exists
```

### Solution

If the error is from the file that already exists, this is a common problem reinitializing the Rook cluster when the local directory used for persistence has **not** been purged.
This directory is the `dataDirHostPath` setting in the cluster CRD and is typically set to `/var/lib/rook`.
To fix the issue you will need to delete all components of Rook and then delete the contents of `/var/lib/rook` (or the directory specified by `dataDirHostPath`) on each of the hosts in the cluster.
Then when the cluster CRD is applied to start a new cluster, the rook-operator should start all the pods as expected.

## OSD pods are not created on my devices

### Symptoms

* No OSD pods are started in the cluster
* Devices are not configured with OSDs even though specified in the Cluster CRD
* One OSD pod is started on each node instead of multiple pods for each device

### Investigation

First, ensure that you have specified the devices correctly in the CRD.
The [Cluster CRD](ceph-cluster-crd.md#storage-selection-settings) has several ways to specify the devices that are to be consumed by the Rook storage:

* `useAllDevices: true`: Rook will consume all devices it determines to be available
* `deviceFilter`: Consume all devices that match this regular expression
* `devices`: Explicit list of device names on each node to consume

Second, if Rook determines that a device is not available (has existing partitions or a formatted filesystem), Rook will skip consuming the devices.
If Rook is not starting OSDs on the devices you expect, Rook may have skipped it for this reason. To see if a device was skipped, view the OSD preparation log
on the node where the device was skipped. Note that it is completely normal and expected for OSD prepare pod to be in the `completed` state.
After the job is complete, Rook leaves the pod around in case the logs need to be investigated.

```console
# Get the prepare pods in the cluster
$ kubectl -n rook-ceph get pod -l app=rook-ceph-osd-prepare
NAME                                   READY     STATUS      RESTARTS   AGE
rook-ceph-osd-prepare-node1-fvmrp      0/1       Completed   0          18m
rook-ceph-osd-prepare-node2-w9xv9      0/1       Completed   0          22m
rook-ceph-osd-prepare-node3-7rgnv      0/1       Completed   0          22m

# view the logs for the node of interest in the "provision" container
$ kubectl -n rook-ceph logs rook-ceph-osd-prepare-node1-fvmrp provision
[...]
```

Here are some key lines to look for in the log:

```console
# A device will be skipped if Rook sees it has partitions or a filesystem
2019-05-30 19:02:57.353171 W | cephosd: skipping device sda that is in use

# A device is going to be configured
2019-05-30 19:02:57.535598 I | cephosd: device sdc to be configured by ceph-volume

# For each device configured you will see a report printed to the log
2019-05-30 19:02:59.844642 I |   Type            Path                                                    LV Size         % of device
2019-05-30 19:02:59.844651 I | ----------------------------------------------------------------------------------------------------
2019-05-30 19:02:59.844677 I |   [data]          /dev/sdc                                                7.00 GB         100%
```

### Solution

After you have either updated the CRD with the correct settings, or you have cleaned the partitions or filesystem from your devices,
you can trigger the operator to analyze the devices again by restarting the operator. Each time the operator starts, it
will ensure all the desired devices are configured. The operator does automatically deploy OSDs in most scenarios, but an operator restart
will cover any scenarios that the operator doesn't detect automatically.

```console
# Restart the operator to ensure devices are configured. A new pod will automatically be started when the current operator pod is deleted.
$ kubectl -n rook-ceph delete pod -l app=rook-ceph-operator
[...]
```

## Node hangs after reboot

### Symptoms

* After issuing a `reboot` command, node never returned online
* Only a power cycle helps

### Solution

The node needs to be [drained](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/) before reboot. After the successful drain, the node can be rebooted as usual.

Because `kubectl drain` command automatically marks the node as unschedulable (`kubectl cordon` effect), the node needs to be uncordoned once it's back online.

Drain the node:

```console
kubectl drain <node-name> --ignore-daemonsets --delete-local-data
```

Uncordon the node:

```console
kubectl uncordon <node-name>
```

## Rook Agent modprobe exec format error

### Symptoms

* PersistentVolumes from Ceph fail/timeout to mount
* Rook Agent logs contain `modinfo: ERROR: could not get modinfo from 'rbd': Exec format error` lines

### Solution

If it is feasible to upgrade your kernel, you should upgrade to `4.x`, even better is >= `4.7` due to a feature for CephFS added to the kernel.

If you are unable to upgrade the kernel, you need to go to each host that will consume storage and run:

```console
modprobe rbd
```

This command inserts the `rbd` module into the kernel.

To persist this fix, you need to add the `rbd` kernel module to either `/etc/modprobe.d/` or `/etc/modules-load.d/`.
For both paths create a file called `rbd.conf` with the following content:

```console
rbd
```

Now when a host is restarted, the module should be loaded automatically.

## Rook Agent rbd module missing error

### Symptoms

* Rook Agent in `Error` or `CrashLoopBackOff` status when deploying the Rook operator with `kubectl create -f operator.yaml`:

```console
$kubectl -n rook-ceph get pod
NAME                                 READY     STATUS    RESTARTS   AGE
rook-ceph-agent-gfrm5                0/1       Error     0          14s
rook-ceph-operator-5f4866946-vmtff   1/1       Running   0          23s
rook-discover-qhx6c                  1/1       Running   0          14s
```

* Rook Agent logs contain below messages:

```console
2018-08-10 09:09:09.461798 I | exec: Running command: cat /lib/modules/4.15.2/modules.builtin
2018-08-10 09:09:09.473858 I | exec: Running command: modinfo -F parm rbd
2018-08-10 09:09:09.477215 N | ceph-volumeattacher: failed rbd single_major check, assuming it's unsupported: failed to check for rbd module single_major param: Failed to complete 'check kmod param': exit status 1. modinfo: ERROR: Module rbd not found.
2018-08-10 09:09:09.477239 I | exec: Running command: modprobe rbd
2018-08-10 09:09:09.480353 I | modprobe rbd: modprobe: FATAL: Module rbd not found.
2018-08-10 09:09:09.480452 N | ceph-volumeattacher: failed to load kernel module rbd: failed to load kernel module rbd: Failed to complete 'modprobe rbd': exit status 1.
failed to run rook ceph agent. failed to create volume manager: failed to load kernel module rbd: Failed to complete 'modprobe rbd': exit status 1.
```

### Solution

From the log message of Agent, we can see that the `rbd` kernel module is not available in the current system, neither as a builtin nor a loadable external kernel module.

In this case, you have to [re-configure and build](https://www.linuxjournal.com/article/6568) a new kernel to address this issue, there're two options:

* Re-configure your kernel to make sure the `CONFIG_BLK_DEV_RBD=y` in the `.config` file, then build the kernel.
* Re-configure your kernel to make sure the `CONFIG_BLK_DEV_RBD=m` in the `.config` file, then build the kernel.

Rebooting the system to use the new kernel, this issue should be fixed: the Agent will be in normal `running` status if everything was done correctly.

## Using multiple shared filesystem (CephFS) is attempted on a kernel version older than 4.7

### Symptoms

* More than one shared filesystem (CephFS) has been created in the cluster
* A pod attempts to mount any other shared filesystem besides the **first** one that was created
* The pod incorrectly gets the first filesystem mounted instead of the intended filesystem

### Solution

The only solution to this problem is to upgrade your kernel to `4.7` or higher.
This is due to a mount flag added in the kernel version `4.7` which allows to chose the filesystem by name.

For additional info on the kernel version requirement for multiple shared filesystems (CephFS), see [Filesystem - Kernel version requirement](ceph-filesystem.md#kernel-version-requirement).

## Activate log to file for a particular Ceph daemon

They are cases where looking at Kubernetes logs is not enough for diverse reasons, but just to name a few:

* not everyone is familiar for Kubernetes logging and expects to find logs in traditional directories
* logs get eaten (buffer limit from the log engine) and thus not requestable from Kubernetes

So for each daemon, `dataDirHostPath` is used to store logs, if logging is activated.
Rook will bindmount `dataDirHostPath` for every pod.
As of Ceph Nautilus 14.2.1, it is possible to enable logging for a particular daemon on the fly.
Let's say you want to enable logging for `mon.a`, but only for this daemon.
Using the toolbox or from inside the operator run:

```console
ceph config daemon mon.a log_to_file true
```

This will activate logging on the filesystem, you will be able to find logs in `dataDirHostPath/$NAMESPACE/log`, so typically this would mean `/var/lib/rook/rook-ceph/log`.
You don't need to restart the pod, the effect will be immediate.

To disable the logging on file, simply set `log_to_file` to `false`.

For Ceph Luminous/Mimic releases, `mon_cluster_log_file` and `cluster_log_file` can be set to
`/var/log/ceph/XXXX` in the config override ConfigMap to enable logging. See the (Advanced
Documentation)[Documentation/advanced-configuration.md#kubernetes] for information about how to use
the config override ConfigMap.

For Ceph Luminous/Mimic releases, `mon_cluster_log_file` and `cluster_log_file` can be set to `/var/log/ceph/XXXX` in the config override ConfigMap to enable logging. See the [Advanced Documentation](#custom-cephconf-settings) for information about how to use the config override ConfigMap.

## Flex storage class versus Ceph CSI storage class

Since Rook 1.1, Ceph CSI has become stable and moving forward is the ultimate replacement over the Flex driver.
However, not all Flex storage classes are available through Ceph CSI since it's basically catching up on features.
Ceph CSI in its 1.2 version (with Rook 1.1) does not support the Erasure coded pools storage class.

So, if you are looking at using such storage class you should enable the Flex driver by setting `ROOK_ENABLE_FLEX_DRIVER: true` in your `operator.yaml`.
Also, if you are in the need of specific features and wonder if CSI is capable of handling them, you should read [the ceph-csi support matrix](https://github.com/ceph/ceph-csi#support-matrix).
