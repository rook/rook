---
title: Common Problems
weight: 42
indent: true
---

Many of these problem cases are hard to summarize down to a short phrase that adequately describes the problem. Each problem will start with a bulleted list of symptoms. Keep in mind that all symptoms may not apply depending upon the configuration of the Rook. If the majority of the symptoms are seen there is a fair chance you are experiencing that problem.

If after trying the suggestions found on this page and the problem is not resolved, the Rook team is very happy to help you troubleshoot the issues in their Slack channel. Once you have [registered for the Rook Slack](https://rook-slackin.herokuapp.com/), proceed to the General channel to ask for assistance.

- [Cluster failing to service requests](#cluster-failing-to-service-requests)
- [Only a single monitor pod starts](#only-a-single-monitor-pod-starts)

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


# Common Problems

## Cluster failing to service requests

### Symptoms
* Execution of the `ceph` command hangs
* Execution of the `rookctl` command hangs
* PersistentVolumes are not being created
* Large amount of slow requests are blocking
* Large amount of stuck requests are blocking
* One or more MONs are restarting periodically

### Investigation
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

### Solution
What is happening here is that the MON pods are restarting and one or more of the CEPH daemons are not getting configured with the proper cluster information. This is commonly the result of not specifying a value for `dataDirHostPath` in your Cluster CRD.

The `dataDirHostPath` setting specifies a path on the local host for the CEPH daemons to store configuration and data. Setting this to a path like `/var/lib/rook`, reapplying your Cluster CRD and restarting all the CEPH daemons (MON, MGR, OSD, RGW) should solve this problem. After the CEPH daemons have been restarted, it is advisable to restart the rook-api and [rook-tool Pods](./toolbox.md).

## Only a single monitor pod starts

### Symptoms
* Entire Rook CRDs have been deleted and the Cluster CRD has been reapplied
* Rook operator is running
* Only a partial number of the MON daemons are started

### Investigation
When attempting to reinstall Rook, the rook-operator pod gets started successfully and then the cluster CRD is then loaded. The rook-operator only starts up a single MON (possibly on rare occasion a second MON may be started) and just hangs. Looking at the log output of the rook-operator the last operation that was occuring was a `ceph mon_status`.

Attempting to run the same command inside the MON container shows that it is having authentication problems as demonstrated below.

```console
root@rook-ceph-mon0-rc568:/# ceph mon_status --cluster=rook --conf=/var/lib/rook/rook/rook.config --keyring=/var/lib/rook/rook/client.admin.keyring
2017-10-11 19:47:24.894698 7f9b85919700  0 monclient(hunting): authenticate timed out after 300
2017-10-11 19:47:24.894698 7f9b85919700  0 monclient(hunting): authenticate timed out after 300
2017-10-11 19:47:24.894757 7f9b85919700  0 librados: client.admin authentication error (110) Connection timed out
2017-10-11 19:47:24.894757 7f9b85919700  0 librados: client.admin authentication error (110) Connection timed out
[errno 110] error connecting to the cluster
```

### Solution
This is a common problem when reinitializing the Rook cluster and the local directory used for persistence has not been purged. This directory is the same directory reference by the `dataDiskHostPath` setting in the cluster CRD and is typically set to `/var/lib/rook`. To remedy simply shutdown all components of Rook and then delete the contents of `/var/lib/rook` or the directory specified by `dataDiskHostPath` on each of the hosts in the cluster. This time when the cluster CRD is applied, the rook-operator should start all the Pods as expected.
