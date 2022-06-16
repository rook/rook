---
title: Ceph Common Issues
---

Many of these problem cases are hard to summarize down to a short phrase that adequately describes the problem. Each problem will start with a bulleted list of symptoms. Keep in mind that all symptoms may not apply depending on the configuration of Rook. If the majority of the symptoms are seen there is a fair chance you are experiencing that problem.

If after trying the suggestions found on this page and the problem is not resolved, the Rook team is very happy to help you troubleshoot the issues in their Slack channel. Once you have [registered for the Rook Slack](https://slack.rook.io), proceed to the `#ceph` channel to ask for assistance.

See also the [CSI Troubleshooting Guide](../Troubleshooting/ceph-csi-common-issues.md).

## Troubleshooting Techniques

There are two main categories of information you will need to investigate issues in the cluster:

1. Kubernetes status and logs documented [here](common-issues.md)
1. Ceph cluster status (see upcoming [Ceph tools](#ceph-tools) section)

### Ceph Tools

After you verify the basic health of the running pods, next you will want to run Ceph tools for status of the storage components. There are two ways to run the Ceph tools, either in the Rook toolbox or inside other Rook pods that are already running.

* Logs on a specific node to find why a PVC is failing to mount
* See the [log collection topic](../Storage-Configuration/Advanced/ceph-configuration.md#log-collection) for a script that will help you gather the logs
* Other artifacts:
  * The monitors that are expected to be in quorum: `kubectl -n <cluster-namespace> get configmap rook-ceph-mon-endpoints -o yaml | grep data`

#### Tools in the Rook Toolbox

The [rook-ceph-tools pod](ceph-toolbox.md) provides a simple environment to run Ceph tools. Once the pod is up and running, connect to the pod to execute Ceph commands to evaluate that current state of the cluster.

```console
kubectl -n rook-ceph exec -it $(kubectl -n rook-ceph get pod -l "app=rook-ceph-tools" -o jsonpath='{.items[*].metadata.name}') bash
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

There are many Ceph sub-commands to look at and manipulate Ceph objects, well beyond the scope this document. See the [Ceph documentation](https://docs.ceph.com/) for more details of gathering information about the health of the cluster. In addition, there are other helpful hints and some best practices located in the [Advanced Configuration section](../Storage-Configuration/Advanced/ceph-configuration.md). Of particular note, there are scripts for collecting logs and gathering OSD information there.

## Cluster failing to service requests

### Symptoms

* Execution of the `ceph` command hangs
* PersistentVolumes are not being created
* Large amount of slow requests are blocking
* Large amount of stuck requests are blocking
* One or more MONs are restarting periodically

### Investigation

Create a [rook-ceph-tools pod](ceph-toolbox.md) to investigate the current state of Ceph. Here is an example of what one might see. In this case the `ceph status` command would just hang so a CTRL-C needed to be sent.

```console
kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- ceph status

ceph status
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

The `dataDirHostPath` setting specifies a path on the local host for the Ceph daemons to store configuration and data. Setting this to a path like `/var/lib/rook`, reapplying your Cluster CRD and restarting all the Ceph daemons (MON, MGR, OSD, RGW) should solve this problem. After the Ceph daemons have been restarted, it is advisable to restart the [rook-tools pod](ceph-toolbox.md).

## Monitors are the only pods running

### Symptoms

* Rook operator is running
* Either a single mon starts or the mons start very slowly (at least several minutes apart)
* The crash-collector pods are crashing
* No mgr, osd, or other daemons are created except the CSI driver

### Investigation

When the operator is starting a cluster, the operator will start one mon at a time and check that they are healthy before continuing to bring up all three mons.
If the first mon is not detected healthy, the operator will continue to check until it is healthy. If the first mon fails to start, a second and then a third
mon may attempt to start. However, they will never form quorum and the orchestration will be blocked from proceeding.

The crash-collector pods will be blocked from starting until the mons have formed quorum the first time.

There are several common causes for the mons failing to form quorum:

* The operator pod does not have network connectivity to the mon pod(s). The network may be configured incorrectly.
* One or more mon pods are in running state, but the operator log shows they are not able to form quorum
* A mon is using configuration from a previous installation. See the [cleanup guide](../Storage-Configuration/ceph-teardown.md#delete-the-data-on-hosts)
  for cleaning the previous cluster.
* A firewall may be blocking the ports required for the Ceph mons to form quorum. Ensure ports 6789 and 3300 are enabled.
  See the [Ceph networking guide](https://docs.ceph.com/en/latest/rados/configuration/network-config-ref/) for more details.
* There may be MTU mismatch between different networking components. Some networks may be more
  susceptible to mismatch than others. If Kubernetes CNI or hosts enable jumbo frames (MTU 9000),
  Ceph will use large packets to maximize network bandwidth. If other parts of the networking chain
  don't support jumbo frames, this could result in lost or rejected packets unexpectedly.

#### Operator fails to connect to the mon

First look at the logs of the operator to confirm if it is able to connect to the mons.

```console
kubectl -n rook-ceph logs -l app=rook-ceph-operator
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

To verify the network connectivity:

* Get the endpoint for a mon
* Curl the mon from the operator pod

For example, this command will curl the first mon from the operator:

```console
$ kubectl -n rook-ceph exec deploy/rook-ceph-operator -- curl $(kubectl -n rook-ceph get svc -l app=rook-ceph-mon -o jsonpath='{.items[0].spec.clusterIP}'):3300 2>/dev/null
ceph v2
```

If "ceph v2" is printed to the console, the connection was successful. If the command does not respond or
otherwise fails, the network connection cannot be established.

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

#### Solution

This is a common problem reinitializing the Rook cluster when the local directory used for persistence has **not** been purged.
This directory is the `dataDirHostPath` setting in the cluster CRD and is typically set to `/var/lib/rook`.
To fix the issue you will need to delete all components of Rook and then delete the contents of `/var/lib/rook` (or the directory specified by `dataDirHostPath`) on each of the hosts in the cluster.
Then when the cluster CRD is applied to start a new cluster, the rook-operator should start all the pods as expected.

!!! caution
    **Deleting the `dataDirHostPath` folder is destructive to the storage. Only delete the folder if you are trying to permanently purge the Rook cluster.**

See the [Cleanup Guide](../Storage-Configuration/ceph-teardown.md) for more details.

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
2. The CSI provisioner pod is not running or is not responding to the request to provision the storage

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

#### CSI Driver

The CSI driver may not be responding to the requests. Look in the logs of the CSI provisioner pod to see if there are any errors
during the provisioning.

There are two provisioner pods:

```console
kubectl -n rook-ceph get pod -l app=csi-rbdplugin-provisioner
```

Get the logs of each of the pods. One of them should be the "leader" and be responding to requests.

```console
kubectl -n rook-ceph logs csi-cephfsplugin-provisioner-d77bb49c6-q9hwq csi-provisioner
```

See also the [CSI Troubleshooting Guide](../Troubleshooting/ceph-csi-common-issues.md).

#### Operator unresponsiveness

Lastly, if you have OSDs `up` and `in`, the next step is to confirm the operator is responding to the requests.
Look in the Operator pod logs around the time when the PVC was created to confirm if the request is being raised.
If the operator does not show requests to provision the block image, the operator may be stuck on some other operation.
In this case, restart the operator pod to get things going again.

### Solution

If the "osd prepare" logs didn't give you enough clues about why the OSDs were not being created,
please review your [cluster.yaml](../CRDs/Cluster/ceph-cluster-crd.md#storage-selection-settings) configuration.
The common misconfigurations include:

* If `useAllDevices: true`, Rook expects to find local devices attached to the nodes. If no devices are found, no OSDs will be created.
* If `useAllDevices: false`, OSDs will only be created if `deviceFilter` is specified.
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
The [Cluster CRD](../CRDs/Cluster/ceph-cluster-crd.md#storage-selection-settings) has several ways to specify the devices that are to be consumed by the Rook storage:

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
```

```console
# view the logs for the node of interest in the "provision" container
$ kubectl -n rook-ceph logs rook-ceph-osd-prepare-node1-fvmrp provision
[...]
```

Here are some key lines to look for in the log:

```console
# A device will be skipped if Rook sees it has partitions or a filesystem
2019-05-30 19:02:57.353171 W | cephosd: skipping device sda that is in use
2019-05-30 19:02:57.452168 W | skipping device "sdb5": ["Used by ceph-disk"]

# Other messages about a disk being unusable by ceph include:
Insufficient space (<5GB) on vgs
Insufficient space (<5GB)
LVM detected
Has BlueStore device label
locked
read-only

# A device is going to be configured
2019-05-30 19:02:57.535598 I | cephosd: device sdc to be configured by ceph-volume

# For each device configured you will see a report printed to the log
2019-05-30 19:02:59.844642 I |   Type            Path                                                    LV Size         % of device
2019-05-30 19:02:59.844651 I | ----------------------------------------------------------------------------------------------------
2019-05-30 19:02:59.844677 I |   [data]          /dev/sdc                                                7.00 GB         100%
```

### Solution

Either update the CR with the correct settings, or clean the partitions or filesystem from your devices.
To clean devices from a previous install see the [cleanup guide](../Storage-Configuration/ceph-teardown.md#zapping-devices).

After the settings are updated or the devices are cleaned, trigger the operator to analyze the devices again by restarting the operator.
Each time the operator starts, it will ensure all the desired devices are configured. The operator does automatically
deploy OSDs in most scenarios, but an operator restart will cover any scenarios that the operator doesn't detect automatically.

```console
# Restart the operator to ensure devices are configured. A new pod will automatically be started when the current operator pod is deleted.
$ kubectl -n rook-ceph delete pod -l app=rook-ceph-operator
[...]
```

## Node hangs after reboot

This issue is fixed in Rook v1.3 or later.

### Symptoms

* After issuing a `reboot` command, node never returned online
* Only a power cycle helps

### Investigation

On a node running a pod with a Ceph persistent volume

```console
mount | grep rbd
# _netdev mount option is absent, also occurs for cephfs
# OS is not aware PV is mounted over network
/dev/rbdx on ... (rw,relatime, ..., noquota)
```

When the reboot command is issued, network interfaces are terminated before disks
are unmounted. This results in the node hanging as repeated attempts to unmount
Ceph persistent volumes fail with the following error:

```console
libceph: connect [monitor-ip]:6789 error -101
```

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

## Using multiple shared filesystem (CephFS) is attempted on a kernel version older than 4.7

### Symptoms

* More than one shared filesystem (CephFS) has been created in the cluster
* A pod attempts to mount any other shared filesystem besides the **first** one that was created
* The pod incorrectly gets the first filesystem mounted instead of the intended filesystem

### Solution

The only solution to this problem is to upgrade your kernel to `4.7` or higher.
This is due to a mount flag added in the kernel version `4.7` which allows to chose the filesystem by name.

For additional info on the kernel version requirement for multiple shared filesystems (CephFS), see [Filesystem - Kernel version requirement](../Storage-Configuration/Shared-Filesystem-CephFS/filesystem-storage.md#kernel-version-requirement).

## Set debug log level for all Ceph daemons

You can set a given log level and apply it to all the Ceph daemons at the same time.
For this, make sure the toolbox pod is running, then determine the level you want (between 0 and 20).
You can find the list of all subsystems and their default values in [Ceph logging and debug official guide](https://docs.ceph.com/en/latest/rados/troubleshooting/log-and-debug/#ceph-subsystems). Be careful when increasing the level as it will produce very verbose logs.

Assuming you want a log level of 1, you will run:

```console
$ kubectl -n rook-ceph exec deploy/rook-ceph-tools -- set-ceph-debug-level 1
ceph config set global debug_context 1
ceph config set global debug_lockdep 1
[...]
```

Once you are done debugging, you can revert all the debug flag to their default value by running the following:

```console
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- set-ceph-debug-level default
```

## Activate log to file for a particular Ceph daemon

They are cases where looking at Kubernetes logs is not enough for diverse reasons, but just to name a few:

* not everyone is familiar for Kubernetes logging and expects to find logs in traditional directories
* logs get eaten (buffer limit from the log engine) and thus not requestable from Kubernetes

So for each daemon, `dataDirHostPath` is used to store logs, if logging is activated.
Rook will bindmount `dataDirHostPath` for every pod.
Let's say you want to enable logging for `mon.a`, but only for this daemon.
Using the toolbox or from inside the operator run:

```console
ceph config set mon.a log_to_file true
```

This will activate logging on the filesystem, you will be able to find logs in `dataDirHostPath/$NAMESPACE/log`, so typically this would mean `/var/lib/rook/rook-ceph/log`.
You don't need to restart the pod, the effect will be immediate.

To disable the logging on file, simply set `log_to_file` to `false`.

## A worker node using RBD devices hangs up

### Symptoms

* There is no progress on I/O from/to one of RBD devices (`/dev/rbd*` or `/dev/nbd*`).
* After that, the whole worker node hangs up.

### Investigation

This happens when the following conditions are satisfied.

* The problematic RBD device and the corresponding OSDs are co-located.
* There is an XFS filesystem on top of this device.

In addition, when this problem happens, you can see the following messages in `dmesg`.

```console
$ dmesg
...
[51717.039319] INFO: task kworker/2:1:5938 blocked for more than 120 seconds.
[51717.039361]       Not tainted 4.15.0-72-generic #81-Ubuntu
[51717.039388] "echo 0 > /proc/sys/kernel/hung_task_timeout_secs" disables this message.
...
```

It's so-called `hung_task` problem and means that there is a deadlock in the kernel. For more detail, please refer to [the corresponding issue comment](https://github.com/rook/rook/issues/3132#issuecomment-580508760).

### Solution

This problem will be solve by the following two fixes.

* Linux kernel: A minor feature that is introduced by [this commit](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=8d19f1c8e1937baf74e1962aae9f90fa3aeab463). It will be included in Linux v5.6.
* Ceph: A fix that uses the above-mentioned kernel's feature. The Ceph community will probably discuss this fix after releasing Linux v5.6.

You can bypass this problem by using ext4 or any other filesystems rather than XFS. Filesystem type can be specified with `csi.storage.k8s.io/fstype` in StorageClass resource.

## Too few PGs per OSD warning is shown

### Symptoms

* `ceph status` shows "too few PGs per OSD" warning as follows.

```console
$ ceph status
  cluster:
    id:     fd06d7c3-5c5c-45ca-bdea-1cf26b783065
    health: HEALTH_WARN
            too few PGs per OSD (16 < min 30)
[...]
```

### Solution

The meaning of this warning is written in [the document](https://docs.ceph.com/docs/master/rados/operations/health-checks#too-few-pgs).
However, in many cases it is benign. For more information, please see [the blog entry](http://ceph.com/community/new-luminous-pg-overdose-protection/).
Please refer to [Configuring Pools](../Storage-Configuration/Advanced/ceph-configuration.md#configuring-pools) if you want to know the proper `pg_num` of pools and change these values.

## LVM metadata can be corrupted with OSD on LV-backed PVC

### Symptoms

There is a critical flaw in OSD on LV-backed PVC. LVM metadata can be corrupted if both the host and OSD container modify it simultaneously. For example, the administrator might modify it on the host, while the OSD initialization process in a container could modify it too. In addition, if `lvmetad` is running, the possibility of occurrence gets higher. In this case, the change of LVM metadata in OSD container is not reflected to LVM metadata cache in host for a while.

If you still decide to configure an OSD on LVM, please keep the following in mind to reduce the probability of this issue.

### Solution

* Disable `lvmetad.`
* Avoid configuration of LVs from the host. In addition, don't touch the VGs and physical volumes that back these LVs.
* Avoid incrementing the `count` field of `storageClassDeviceSets` and create a new LV that backs an OSD simultaneously.

You can know whether the above-mentioned tag exists with the command: `sudo lvs -o lv_name,lv_tags`. If the `lv_tag` field is empty in an LV corresponding to the OSD lv_tags, this OSD encountered the problem. In this case, please [retire this OSD](../Storage-Configuration/Advanced/ceph-osd-mgmt.md#remove-an-osd) or replace with other new OSD before restarting.

This problem doesn't happen in newly created LV-backed PVCs because OSD container doesn't modify LVM metadata anymore. The existing lvm mode OSDs work continuously even thought upgrade your Rook. However, using the raw mode OSDs is recommended because of the above-mentioned problem. You can replace the existing OSDs with raw mode OSDs by retiring them and adding new OSDs one by one. See the documents [Remove an OSD](../Storage-Configuration/Advanced/ceph-osd-mgmt.md#remove-an-osd) and [Add an OSD on a PVC](../Storage-Configuration/Advanced/ceph-osd-mgmt.md#add-an-osd-on-a-pvc).

## OSD prepare job fails due to low aio-max-nr setting

If the Kernel is configured with a low [aio-max-nr setting](https://www.kernel.org/doc/Documentation/sysctl/fs.txt), the OSD prepare job might fail with the following error:

```text
exec: stderr: 2020-09-17T00:30:12.145+0000 7f0c17632f40 -1 bdev(0x56212de88700 /var/lib/ceph/osd/ceph-0//block) _aio_start io_setup(2) failed with EAGAIN; try increasing /proc/sys/fs/aio-max-nr
```

To overcome this, you need to increase the value of `fs.aio-max-nr` of your sysctl configuration (typically `/etc/sysctl.conf`).
You can do this with your favorite configuration management system.

Alternatively, you can have a [DaemonSet](https://github.com/rook/rook/issues/6279#issuecomment-694390514) to apply the configuration for you on all your nodes.

## Unexpected partitions created

### Symptoms

**Users running Rook versions v1.6.0-v1.6.7 may observe unwanted OSDs on partitions that appear
unexpectedly and seemingly randomly, which can corrupt existing OSDs.**

Unexpected partitions are created on host disks that are used by Ceph OSDs. This happens more often
on SSDs than HDDs and usually only on disks that are 875GB or larger. Many tools like `lsblk`,
`blkid`, `udevadm`, and `parted` will not show a partition table type for the partition. Newer
versions of `blkid` are generally able to recognize the type as "atari".

The underlying issue causing this is Atari partition (sometimes identified as AHDI) support in the
Linux kernel. Atari partitions have very relaxed specifications compared to other partition types,
and it is relatively easy for random data written to a disk to appear as an Atari partition to the
Linux kernel. Ceph's Bluestore OSDs have an anecdotally high probability of writing data on to disks
that can appear to the kernel as an Atari partition.

Below is an example of `lsblk` output from a node where phantom Atari partitions are present. Note
that `sdX1` is never present for the phantom partitions, and `sdX2` is 48G on all disks. `sdX3`
is a variable size and may not always be present. It is possible for `sdX4` to appear, though it is
an anecdotally rare event.

```console
# lsblk
NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINT
sdb      8:16   0     3T  0 disk
├─sdb2   8:18   0    48G  0 part
└─sdb3   8:19   0   6.1M  0 part
sdc      8:32   0     3T  0 disk
├─sdc2   8:34   0    48G  0 part
└─sdc3   8:35   0   6.2M  0 part
sdd      8:48   0     3T  0 disk
├─sdd2   8:50   0    48G  0 part
└─sdd3   8:51   0   6.3M  0 part
```

You can see [GitHub rook/rook - Issue 7940 unexpected partition on disks >= 1TB (atari partitions)](https://github.com/rook/rook/issues/7940) for more detailed information and discussion.

### Solution

#### Recover from corruption (v1.6.0-v1.6.7)

If you are using Rook v1.6, you must first update to v1.6.8 or higher to avoid further incidents of
OSD corruption caused by these Atari partitions.

An old workaround suggested using `deviceFilter: ^sd[a-z]+$`, but this still results in unexpected
partitions. Rook will merely stop creating new OSDs on the partitions. It does not fix a related
issue that `ceph-volume` that is unaware of the Atari partition problem. Users who used this
workaround are still at risk for OSD failures in the future.

To resolve the issue, immediately update to v1.6.8 or higher. After the update, no corruption should
occur on OSDs created in the future. Next, to get back to a healthy Ceph cluster state, focus on one
corrupted disk at a time and [remove all OSDs on each corrupted disk](../Storage-Configuration/Advanced/ceph-osd-mgmt.md#remove-an-osd)
one disk at a time.

As an example, you may have `/dev/sdb` with two unexpected partitions (`/dev/sdb2` and `/dev/sdb3`)
as well as a second corrupted disk `/dev/sde` with one unexpected partition (`/dev/sde2`).

1. First, remove the OSDs associated with `/dev/sdb`, `/dev/sdb2`, and `/dev/sdb3`. There might be
   only one, or up to 3 OSDs depending on how your system was affected. Again see the
   [OSD management doc](../Storage-Configuration/Advanced/ceph-osd-mgmt.md#remove-an-osd).
2. Use `dd` to wipe the first sectors of the partitions followed by the disk itself. E.g.,
    * `dd if=/dev/zero of=/dev/sdb2 bs=1M`
    * `dd if=/dev/zero of=/dev/sdb3 bs=1M`
    * `dd if=/dev/zero of=/dev/sdb bs=1M`
3. Then wipe clean `/dev/sdb` to prepare it for a new OSD.
   See [the teardown document](../Storage-Configuration/ceph-teardown.md#zapping-devices) for details.
4. After this, scale up the Rook operator to deploy a new OSD to `/dev/sdb`. This will allow Ceph to
   use `/dev/sdb` for data recovery and replication while the next OSDs are removed.
5. Now Repeat steps 1-4 for `/dev/sde` and `/dev/sde2`, and continue for any other corrupted disks.

If your Rook cluster does not have any critical data stored in it, it may be simpler to
uninstall Rook completely and redeploy with v1.6.8 or higher.

## Operator environment variables are ignored

### Symptoms

Configuration settings passed as environment variables do not take effect as expected. For example,
the discover daemonset is not created, even though `ROOK_ENABLE_DISCOVERY_DAEMON="true"` is set.

### Investigation

Inspect the `rook-ceph-operator-config` ConfigMap for conflicting settings. The ConfigMap takes
precedence over the environment. The ConfigMap [must exist](../Storage-Configuration/Advanced/ceph-configuration.md#configuration-using-environment-variables),
even if all actual configuration is supplied through the environment.

Look for lines with the `op-k8sutil` prefix in the operator logs.
These lines detail the final values, and source, of the different configuration variables.

Verify that both of the following messages are present in the operator logs:

```log
rook-ceph-operator-config-controller successfully started
rook-ceph-operator-config-controller done reconciling
```

### Solution

If it does not exist, create an empty ConfigMap:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-ceph-operator-config
  namespace: rook-ceph # namespace:operator
data: {}
```

If the ConfigMap exists, remove any keys that you wish to configure through the environment.
