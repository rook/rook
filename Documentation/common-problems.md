---
title: Common Issues
weight: 78
indent: true
---

# Common Problems

Many of these problem cases are hard to summarize down to a short phrase that adequately describes the problem. Each problem will start with a bulleted list of symptoms. Keep in mind that all symptoms may not apply depending upon the configuration of the Rook. If the majority of the symptoms are seen there is a fair chance you are experiencing that problem.

If after trying the suggestions found on this page and the problem is not resolved, the Rook team is very happy to help you troubleshoot the issues in their Slack channel. Once you have [registered for the Rook Slack](https://rook-slackin.herokuapp.com/), proceed to the General channel to ask for assistance.

## Table of Contents
- [Pod using Rook storage is not running](#pod-using-rook-storage-is-not-running)
- [Cluster failing to service requests](#cluster-failing-to-service-requests)
- [Only a single monitor pod starts after redeploy](#only-a-single-monitor-pod-starts-after-redeploy)

# Troubleshooting Techniques
One of the first things that should be done is to start the [rook-tools pod](./toolbox.md) as described in the Toolbox section. Once the pod is up and running one can `kubectl exec` into the pod to execute Ceph commands to evaluate that current state of the cluster. Here is a list of commands that can help one get an understanding of the current state.

* rookctl status
* ceph status
* ceph osd status
* ceph osd df
* ceph osd utilization
* ceph osd pool stats
* ceph osd tree
* ceph pg stat

Of particular note, the first two status commands provide the overall cluster health. The normal state for cluster operations is HEALTH_OK, but will still function when the state is in a HEALTH_WARN state. If you are in a WARN state, then the cluster is in a condition that it may enter the HEALTH_ERROR state at which point *all* disk I/O operations are halted. If a HEALTH_WARN state is observed, then one should take action to prevent the cluster from halting when it enters the HEALTH_ERROR state.

There is literally a ton of Ceph sub-commands to look at and manipulate Ceph objects. Well beyond the scope of a few troubleshooting techniques and there are other sites and documentation sets that deal more with assisting one with troubleshooting a Ceph environment. In addition, there are other helpful hints and some best practices concerning a Ceph environment located in the [Advanced Configuration section](advanced-configuration.md). Of particular note there are scripts for collecting logs and gathering OSD information there.


# Pod Using Rook Storage Is Not Running

## Symptoms
* The pod that is configured to use Rook storage is stuck in the `ContainerCreating` status
* `kubectl describe pod` for the pod mentions one or more of the following:
  * `PersistentVolumeClaim is not bound`
  * `timeout expired waiting for volumes to attach/mount`
* `kubectl -n rook-system get pod` shows the rook-agent pods in a `CrashLoopBackOff` status

## Possible Solutions Summary
* `rook-agent` pod is in a `CrashLoopBackOff` status because it cannot deploy its driver on a read-only filesystem: [Flexvolume configuration pre-reqs](./k8s-pre-reqs.md#flexvolume-configuration)
* Persistent Volume and/or Claim are failing to be created and bound: [Volume Creation](#volume-creation)
* `rook-agent` pod is failing to mount and format the volume: [Rook Agent Mounting](#volume-mounting)
* You are using Kubernetes 1.7.x or earlier and the Kubelet has not been restarted after `rook-agent` is in the `Running` status: [Restart Kubelet](#kubelet-restart)
* You are using Kubernetes 1.6.x and the attach-detach controller has not been disabled: [Disable attach-detach controller](./quickstart.md#disable-attacher-detacher-controller)

## Investigation Details
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

### Rook Agent Deployment
The `rook-agent` pods are responsible for mapping and mounting the volume from the cluster onto the node that your pod will be running on.
If the `rook-agent` pod is not running then it cannot perform this function.
Below is an example of the `rook-agent` pods failing to get to the `Running` status because they are in a `CrashLoopBackOff` status:
```console
> kubectl -n rook-system get pod
NAME                             READY     STATUS             RESTARTS   AGE
rook-agent-ct5pj                 0/1       CrashLoopBackOff   16         59m
rook-agent-zb6n9                 0/1       CrashLoopBackOff   16         59m
rook-operator-2203999069-pmhzn   1/1       Running            0          59m
```
If you see this occurring, you can get more details about why the `rook-agent` pods are continuing to crash with the following command and its sample output:
```console
> kubectl -n rook-system get pod -l app=rook-agent -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[0].lastState.terminated.message}{"\n"}{end}'
rook-agent-ct5pj	mkdir /usr/libexec/kubernetes: read-only file system
rook-agent-zb6n9	mkdir /usr/libexec/kubernetes: read-only file system
```
From the output above, we can see that the agents were not able to bind mount to `/usr/libexec/kubernetes` on the host they are scheduled to run on.
For some environments, this default path is read-only and therefore a better path must be provided to the agents.

First, clean up the agent deployment with:
```console
kubectl -n rook-system delete daemonset rook-agent
```
Once the `rook-agent` pods are gone, **follow the instructions in the [Flexvolume configuration pre-reqs](./k8s-pre-reqs.md#flexvolume-configuration)** to ensure a good value for `--volume-plugin-dir` has been provided to the Kubelet.
After that has been configured, and the Kubelet has been restarted, start the agent pods up again by restarting `rook-operator`:
```console
kubectl -n rook-system delete pod -l app=rook-operator
```

### Kubelet Restart
#### **Kubernetes 1.7.x and earlier only**
If the `rook-agent` pods are all in the `Running` state then another thing to confirm is that **if you are running on Kubernetes 1.7.x or earlier**, the Kubelet must be restarted after the `rook-agent` pods are running.

A symptom of this can be found in the Kubelet's log/journal, with the following error saying `no volume plugin matched`:
```console
Oct 30 22:23:03 core-02 kubelet-wrapper[31926]: E1030 22:23:03.524159   31926 desired_state_of_world_populator.go:285] Failed to add volume "mysql-persistent-storage" (specName: "pvc-9f273fbc") for pod "9f2ff89a-bdbf" to desiredStateOfWorld. err=failed to get Plugin from volumeSpec for volume "pvc-9f273fbc" err=no volume plugin matched
```
If you encounter this, just **restart the Kubelet process**, as described in the **[Restart Kubelet](./quickstart.md#restart-kubelet)** section of the Rook deployment guide.

### Volume Creation
The volume must first be created in the Rook cluster and then bound to a volume claim before it can be mounted to a pod.
Let's confirm that with the following commands and their output:
```console
> kubectl get pv
NAME                                       CAPACITY   ACCESSMODES   RECLAIMPOLICY   STATUS     CLAIM                    STORAGECLASS   REASON    AGE
pvc-9f273fbc-bdbf-11e7-bc4c-001c428b9fc8   20Gi       RWO           Delete          Bound      default/mysql-pv-claim   rook-block               25m

> kubectl get pvc
NAME             STATUS    VOLUME                                     CAPACITY   ACCESSMODES   STORAGECLASS   AGE
mysql-pv-claim   Bound     pvc-9f273fbc-bdbf-11e7-bc4c-001c428b9fc8   20Gi       RWO           rook-block     25m
```
Both your volume and its claim should be in the `Bound` status.
If one or neither of them is not in the `Bound` status, then look for details of the issue in the `rook-operator` logs:
```console
kubectl -n rook-system logs `kubectl -n rook-system -l app=rook-operator get pods -o jsonpath='{.items[*].metadata.name}'`
```
If the volume is failing to be created, there should be details in the log output, especially those tagged with `op-provisioner`.

### Volume Mounting
The final step in preparing Rook storage for your pod is for the `rook-agent` pod to mount and format it.
If all the preceding sections have been successful or inconclusive, then take a look at the `rook-agent` pod logs for further clues.
You can determine which `rook-agent` is running on the same node that your pod is scheduled on by using the `-o wide` output, then you can get the logs for that `rook-agent` pod similar to the example below:
```console
> kubectl -n rook-system get pod -o wide
NAME                             READY     STATUS    RESTARTS   AGE       IP             NODE
rook-agent-h6scx                 1/1       Running   0          9m        172.17.8.102   172.17.8.102
rook-agent-mp7tn                 1/1       Running   0          9m        172.17.8.101   172.17.8.101
rook-operator-2203999069-3tb68   1/1       Running   0          9m        10.32.0.7      172.17.8.101

> kubectl -n rook-system logs rook-agent-h6scx
2017-10-30 23:07:06.984108 I | rook: starting Rook v0.5.0-241.g48ce6de.dirty with arguments '/usr/local/bin/rook agent'
...
```

In the `rook-agent` pod logs, you may see a snippet similar to the following:
```console
Failed to complete rbd: signal: interrupt.
```
In this case, the agent waited for the `rbd` command but it did not finish in a timely manner so the agent gave up and stopped it.
This can happen for multiple reasons, but using `dmesg` will likely give you insight into the root cause.
If `dmesg` shows something similar to below, then it means you have an old kernel that can't talk to the cluster:
```console
libceph: mon2 10.205.92.13:6790 feature set mismatch, my 4a042a42 < server's 2004a042a42, missing 20000000000
```
If `uname -a` shows that you have a kernel version older than `3.15`, you'll need to perform **one** of the following:
* Disable some Ceph features by starting the [rook toolbox](./toolbox.md) and running `ceph osd crush tunables bobtail`
* Upgrade your kernel to `3.15` or later.

### Filesystem Mounting

In the `rook-agent` pod logs, you may see a snippet similar to the following:
```console
2017-11-07 00:04:37.808870 I | rook-flexdriver: WARNING: The node kernel version is 4.4.0-87-generic, which do not support multiple ceph filesystems. The kernel version has to be at least 4.7. If you have multiple ceph filesystems, the result could be inconsistent
```

This will happen in kernels with versions older than 4.7, where the option `mds_namespace` is not supported. This option is used to specify a filesystem namespace.

In this case, if there is only one filesystem in the Rook cluster, there should be no issues and the mount should succeed. If you have more than one filesystem, inconsistent results may arise and the filesystem mounted may not be the one you specified.

If the issue is still not resolved from the steps above, please come chat with us on the **#general** channel of our [Rook Slack](https://rook-slackin.herokuapp.com/).
We want to help you get your storage working and learn from those lessons to prevent users in the future from seeing the same issue.

# Cluster failing to service requests

## Symptoms
* Execution of the `ceph` command hangs
* Execution of the `rookctl` command hangs
* PersistentVolumes are not being created
* Large amount of slow requests are blocking
* Large amount of stuck requests are blocking
* One or more MONs are restarting periodically

## Investigation
Create a [rook-tools pod](./toolbox.md) to investigate the current state of CEPH. Here is an example of what one might see. In this case the `ceph status` command would just hang so a CTRL-C needed to be send. The `rookctl status` command is able to give a good amount of detail. In some cases the rook-api pod needs to be restarted for `rookctl` to be able to gather information. If the rook-api is restarted, then the rook-tools pod should be restarted also. 

```console
$ kubectl -n rook exec -it rook-tools bash
root@rook-tools:/# ceph status
^CCluster connection interrupted or timed out
root@rook-tools:/# rookctl status
OVERALL STATUS: ERROR

SUMMARY:
SEVERITY   NAME              MESSAGE
WARNING    REQUEST_SLOW      1664 slow requests are blocked > 32 sec
ERROR      REQUEST_STUCK     102722 stuck requests are blocked > 4096 sec
WARNING    TOO_MANY_PGS      too many PGs per OSD (323 > max 300)
WARNING    OSD_DOWN          1 osds down
WARNING    OSD_HOST_DOWN     1 host (1 osds) down
WARNING    PG_AVAILABILITY   Reduced data availability: 415 pgs stale
WARNING    PG_DEGRADED       Degraded data redundancy: 190/958 objects degraded (19.833%), 53 pgs unclean, 53 pgs degraded, 53 pgs undersized

USAGE:
TOTAL        USED        DATA       AVAILABLE
755.27 GiB   65.29 GiB   1.12 GiB   689.98 GiB

MONITORS:
NAME              ADDRESS               IN QUORUM   STATUS
rook-ceph-mon75   172.18.0.70:6790/0    true        OK
rook-ceph-mon21   172.18.0.245:6790/0   true        OK
rook-ceph-mon65   172.18.0.246:6790/0   true        OK

MGRs:
NAME             STATUS
rook-ceph-mgr0   Active

OSDs:
TOTAL     UP        IN        FULL      NEAR FULL
4         1         2         false     false

PLACEMENT GROUPS (600 total):
STATE                        COUNT
active+clean                 132
active+undersized+degraded   53
stale+active+clean           415
```

Another indication is when one or more of the MON pods restart frequently. Note the 'mon107' that has only been up for 16 minutes in the following output.

```console
$ kubectl -n rook get all -o wide --show-all
NAME                                 READY     STATUS    RESTARTS   AGE       IP               NODE
po/rook-api-41429188-x9l2r           1/1       Running   0          17h       192.168.1.187    k8-host-0401
po/rook-ceph-mgr0-2487684371-gzlbq   1/1       Running   0          17h       192.168.224.46   k8-host-0402
po/rook-ceph-mon107-p74rj            1/1       Running   0          16m       192.168.224.28   k8-host-0402
rook-ceph-mon1-56fgm                 1/1       Running   0          2d        192.168.91.135   k8-host-0404
rook-ceph-mon2-rlxcd                 1/1       Running   0          2d        192.168.123.33   k8-host-0403
rook-ceph-osd-bg2vj                  1/1       Running   0          2d        192.168.91.177   k8-host-0404
rook-ceph-osd-mwxdm                  1/1       Running   0          2d        192.168.123.31   k8-host-0403
```

## Solution
What is happening here is that the MON pods are restarting and one or more of the CEPH daemons are not getting configured with the proper cluster information. This is commonly the result of not specifying a value for `dataDirHostPath` in your Cluster CRD.

The `dataDirHostPath` setting specifies a path on the local host for the CEPH daemons to store configuration and data. Setting this to a path like `/var/lib/rook`, reapplying your Cluster CRD and restarting all the CEPH daemons (MON, MGR, OSD, RGW) should solve this problem. After the CEPH daemons have been restarted, it is advisable to restart the rook-api and [rook-tool Pods](./toolbox.md).

# Only a single monitor pod starts after redeploy

## Symptoms
* After tearing down a working cluster to redeploy a new cluster, the new cluster fails to start
* Rook operator is running
* Only a partial number of the MON daemons are created and are failing
* If the mons started, the OSD pods are failing

## Investigation
When attempting to reinstall Rook, the rook-operator pod gets started successfully and then the cluster CRD is then loaded. The rook-operator only starts up a single MON (possibly on rare occasion a second MON may be started) and just hangs. Looking at the log output of the rook-operator the last operation that was occuring was a `ceph mon_status`.

Looking at the log or the termination status for the `mon` pod, you will see a message indicating the keyring does not match from a previous deployment.

```
# the mon pod is in a crash loop backoff state
$ kubectl -n rook get pod
NAME                   READY     STATUS             RESTARTS   AGE
rook-ceph-mon0-r8tbl   0/1       CrashLoopBackOff   2          47s

# the pod shows a termination status that the keyring does not match the existing keyring
$ kubectl -n rook describe pod -l mon=rook-ceph-mon0
...
    Last State:		Terminated
      Reason:		Error
      Message:		The keyring does not match the existing keyring in /var/lib/rook/rook-ceph-mon0/data/keyring. 
                    You may need to delete the contents of dataDirHostPath on the host from a previous deployment.
...
```

If your cluster is larger than a couple nodes, you may get lucky enough that the monitors were able to start and form quorum. However, now the OSDs pods may fail to start due to state
from a previous deployment. Looking at the OSD pod logs you will see an error about the file already existing.
```
$ kubectl -n rook logs rook-ceph-osd-fl8fs
...
2017-10-31 20:13:11.187106 I | mkfs-osd0: 2017-10-31 20:13:11.186992 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) _read_fsid unparsable uuid 
2017-10-31 20:13:11.187208 I | mkfs-osd0: 2017-10-31 20:13:11.187026 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) _setup_block_symlink_or_file failed to create block symlink to /dev/disk/by-partuuid/651153ba-2dfc-4231-ba06-94759e5ba273: (17) File exists
2017-10-31 20:13:11.187233 I | mkfs-osd0: 2017-10-31 20:13:11.187038 7f0059d62e00 -1 bluestore(/var/lib/rook/osd0) mkfs failed, (17) File exists
2017-10-31 20:13:11.187254 I | mkfs-osd0: 2017-10-31 20:13:11.187042 7f0059d62e00 -1 OSD::mkfs: ObjectStore::mkfs failed with error (17) File exists
2017-10-31 20:13:11.187275 I | mkfs-osd0: 2017-10-31 20:13:11.187121 7f0059d62e00 -1  ** ERROR: error creating empty object store in /var/lib/rook/osd0: (17) File exists
```

## Solution
This is a common problem reinitializing the Rook cluster when the local directory used for persistence has **not** been purged. 
This directory is the `dataDirHostPath` setting in the cluster CRD and is typically set to `/var/lib/rook`. 
To fix the issue you will need to delete all components of Rook and then delete the contents of `/var/lib/rook` (or the directory specified by `dataDirHostPath`) on each of the hosts in the cluster. 
Then when the cluster CRD is applied to start a new cluster, the rook-operator should start all the pods as expected.
