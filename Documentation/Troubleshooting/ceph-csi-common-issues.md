---
title: CSI Common Issues
---

Issues when provisioning volumes with the Ceph CSI driver can happen for many reasons such as:

* Network connectivity between CSI pods and ceph
* Cluster health issues
* Slow operations
* Kubernetes issues
* Ceph-CSI configuration or bugs

The following troubleshooting steps can help identify a number of issues.

### Block (RBD)

If you are mounting block volumes (usually RWO), these are referred to as `RBD` volumes in Ceph.
See the sections below for RBD if you are having block volume issues.

### Shared Filesystem (CephFS)

If you are mounting shared filesystem volumes (usually RWX), these are referred to as `CephFS` volumes in Ceph.
See the sections below for CephFS if you are having filesystem volume issues.

## Network Connectivity

The Ceph monitors are the most critical component of the cluster to check first.
Retrieve the mon endpoints from the services:

```console
$ kubectl -n rook-ceph get svc -l app=rook-ceph-mon
NAME              TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
rook-ceph-mon-a   ClusterIP   10.104.165.31   <none>        6789/TCP,3300/TCP   18h
rook-ceph-mon-b   ClusterIP   10.97.244.93    <none>        6789/TCP,3300/TCP   21s
rook-ceph-mon-c   ClusterIP   10.99.248.163   <none>        6789/TCP,3300/TCP   8s
```

If host networking is enabled in the CephCluster CR, you will instead need to find the
node IPs for the hosts where the mons are running.

The `clusterIP` is the mon IP and `3300` is the port that will be used by Ceph-CSI to connect to the ceph cluster.
These endpoints must be accessible by all clients in the cluster, including the CSI driver.

If you are seeing issues provisioning the PVC then you need to check the network connectivity from the provisioner pods.

* For CephFS PVCs, check network connectivity from the `csi-cephfsplugin` container of the `csi-cephfsplugin-provisioner` pods
* For Block PVCs, check network connectivity from the `csi-rbdplugin` container of the `csi-rbdplugin-provisioner` pods

For redundancy, there are two provisioner pods for each type. Make sure to test connectivity from all provisioner pods.

Connect to the provisioner pods and verify the connection to the mon endpoints such as the following:

```console
# Connect to the csi-cephfsplugin container in the provisioner pod
kubectl -n rook-ceph exec -ti deploy/csi-cephfsplugin-provisioner -c csi-cephfsplugin -- bash

# Test the network connection to the mon endpoint
curl 10.104.165.31:3300 2>/dev/null
ceph v2
```

If you see the response "ceph v2", the connection succeeded.
If there is no response then there is a network issue connecting to the ceph cluster.

Check network connectivity for all monitor IP’s and ports which are passed to ceph-csi.

## Ceph Health

Sometimes an unhealthy Ceph cluster can contribute to the issues in creating or mounting the PVC.
Check that your Ceph cluster is healthy by connecting to the [Toolbox](ceph-toolbox.md) and
running the `ceph` commands:

```console
ceph health detail
```

```console
HEALTH_OK
```

## Slow Operations

Even slow ops in the ceph cluster can contribute to the issues. In the toolbox,
make sure that no slow ops are present and the ceph cluster is healthy

```console
$ ceph -s
cluster:
  id:     ba41ac93-3b55-4f32-9e06-d3d8c6ff7334
  health: HEALTH_WARN
          30 slow ops, oldest one blocked for 10624 sec, mon.a has slow ops
[...]
```

If Ceph is not healthy, check the following health for more clues:

* The Ceph monitor logs for errors
* The OSD logs for errors
* Disk Health
* Network Health

## Ceph Troubleshooting

### Check if the RBD Pool exists

Make sure the pool you have specified in the `storageclass.yaml` exists in the ceph cluster.

Suppose the pool name mentioned in the `storageclass.yaml` is `replicapool`. It can be verified
to exist in the toolbox:

```console
$ ceph osd lspools
1 .mgr
2 replicapool
```

If the pool is not in the list, create the `CephBlockPool` CR for the pool if you have not already.
If you have already created the pool, check the Rook operator log for errors creating the pool.

### Check if the Filesystem exists

For the shared filesystem (CephFS), check that the filesystem and pools you have specified in the `storageclass.yaml` exist in the Ceph cluster.

Suppose the `fsName` name mentioned in the `storageclass.yaml` is `myfs`. It can be verified in the toolbox:

```console
$ ceph fs ls
name: myfs, metadata pool: myfs-metadata, data pools: [myfs-data0 ]
```

Now verify the `pool` mentioned in the `storageclass.yaml` exists, such as the example `myfs-data0`.

```console
ceph osd lspools
1 .mgr
2 replicapool
3 myfs-metadata0
4 myfs-data0
```

The pool for the filesystem will have the suffix `-data0` compared the filesystem name that is created
by the CephFilesystem CR.

### subvolumegroups

If the subvolumegroup is not specified in the ceph-csi configmap (where you have passed the ceph monitor information),
Ceph-CSI creates the default subvolumegroup with the name csi. Verify that the subvolumegroup
exists:

```console
$ ceph fs subvolumegroup ls myfs
[
    {
        "name": "csi"
    }
]
```

If you don’t see any issues with your Ceph cluster, the following sections will start debugging the issue from the CSI side.

## Provisioning Volumes

At times the issue can also exist in the Ceph-CSI or the sidecar containers used in Ceph-CSI.

Ceph-CSI has included number of sidecar containers in the provisioner pods such as:
`csi-attacher`, `csi-resizer`, `csi-provisioner`, `csi-cephfsplugin`, `csi-snapshotter`, and `liveness-prometheus`.

The CephFS provisioner core CSI driver container name is `csi-cephfsplugin` as one of the container names.
For the RBD (Block) provisioner you will see `csi-rbdplugin` as the container name.

Here is a summary of the sidecar containers:

### csi-provisioner

The external-provisioner is a sidecar container that dynamically provisions volumes by calling `ControllerCreateVolume()`
and `ControllerDeleteVolume()` functions of CSI drivers. More details about external-provisioner can be found here.

If there is an issue with PVC Create or Delete, check the logs of the `csi-provisioner` sidecar container.

```console
kubectl -n rook-ceph logs deploy/csi-rbdplugin-provisioner -c csi-provisioner
```

### csi-resizer

The CSI `external-resizer` is a sidecar container that watches the Kubernetes API server for PersistentVolumeClaim
updates and triggers `ControllerExpandVolume` operations against a CSI endpoint if the user requested more storage
on the PersistentVolumeClaim object. More details about external-provisioner can be found here.

If any issue exists in PVC expansion you can check the logs of the `csi-resizer` sidecar container.

```console
kubectl -n rook-ceph logs deploy/csi-rbdplugin-provisioner -c csi-resizer
```

### csi-snapshotter

The CSI external-snapshotter sidecar only watches for `VolumeSnapshotContent` create/update/delete events.
It will talk to ceph-csi containers to create or delete snapshots. More details about external-snapshotter can
be found [here](https://github.com/kubernetes-csi/external-snapshotter).

**In Kubernetes 1.17 the volume snapshot feature was promoted to beta. In Kubernetes 1.20, the feature gate is enabled by default on standard Kubernetes deployments and cannot be turned off.**

Make sure you have installed the correct snapshotter CRD version. If you have not installed the snapshotter
controller, see the [Snapshots guide](../Storage-Configuration/Ceph-CSI/ceph-csi-snapshot.md).

```console
$ kubectl get crd | grep snapshot
volumesnapshotclasses.snapshot.storage.k8s.io    2021-01-25T11:19:38Z
volumesnapshotcontents.snapshot.storage.k8s.io   2021-01-25T11:19:39Z
volumesnapshots.snapshot.storage.k8s.io          2021-01-25T11:19:40Z
```

The above CRDs must have the matching version in your `snapshotclass.yaml` or `snapshot.yaml`.
Otherwise, the `VolumeSnapshot` and `VolumesnapshotContent` will not be created.

The snapshot controller is responsible for creating both `VolumeSnapshot` and
`VolumesnapshotContent` object. If the objects are not getting created, you may need to
check the logs of the snapshot-controller container.

Rook only installs the snapshotter sidecar container, not the controller. It is recommended
that Kubernetes distributors bundle and deploy the controller and CRDs as part of their Kubernetes cluster
management process (independent of any CSI Driver).

If your Kubernetes distribution does not bundle the snapshot controller, you may manually install these components.

If any issue exists in the snapshot Create/Delete operation you can check the logs of the csi-snapshotter sidecar container.

```console
kubectl -n rook-ceph logs deploy/csi-rbdplugin-provisioner -c csi-snapshotter
```

If you see an error about a volume already existing such as:

```console
GRPC error: rpc error: code = Aborted desc = an operation with the given Volume ID
0001-0009-rook-ceph-0000000000000001-8d0ba728-0e17-11eb-a680-ce6eecc894de already exists.
```

The issue typically is in the Ceph cluster or network connectivity. If the issue is
in Provisioning the PVC Restarting the Provisioner pods help(for CephFS issue
restart `csi-cephfsplugin-provisioner-xxxxxx` CephFS Provisioner. For RBD, restart
the `csi-rbdplugin-provisioner-xxxxxx` pod. If the issue is in mounting the PVC,
restart the `csi-rbdplugin-xxxxx` pod (for RBD) and the `csi-cephfsplugin-xxxxx` pod
for CephFS issue.

## Mounting the volume to application pods

When a user requests to create the application pod with PVC, there is a three-step process

* CSI driver registration
* Create volume attachment object
* Stage and publish the volume

### csi-driver registration

`csi-cephfsplugin-xxxx` or `csi-rbdplugin-xxxx` is a daemonset pod running on all the nodes
where your application gets scheduled. If the plugin pods are not running on the node where
your application is scheduled might cause the issue, make sure plugin pods are always running.

Each plugin pod has two important containers: one is `driver-registrar` and `csi-rbdplugin` or
`csi-cephfsplugin`. Sometimes there is also a `liveness-prometheus` container.

### driver-registrar

The node-driver-registrar is a sidecar container that registers the CSI driver with Kubelet.
More details can be found [here](https://github.com/kubernetes-csi/node-driver-registrar).

If any issue exists in attaching the PVC to the application pod check logs from driver-registrar
sidecar container in plugin pod where your application pod is scheduled.

```console
$ kubectl -n rook-ceph logs deploy/csi-rbdplugin -c driver-registrar
[...]
I0120 12:28:34.231761  124018 main.go:112] Version: v2.0.1
I0120 12:28:34.233910  124018 connection.go:151] Connecting to unix:///csi/csi.sock
I0120 12:28:35.242469  124018 node_register.go:55] Starting Registration Server at: /registration/rook-ceph.rbd.csi.ceph.com-reg.sock
I0120 12:28:35.243364  124018 node_register.go:64] Registration Server started at: /registration/rook-ceph.rbd.csi.ceph.com-reg.sock
I0120 12:28:35.243673  124018 node_register.go:86] Skipping healthz server because port set to: 0
I0120 12:28:36.318482  124018 main.go:79] Received GetInfo call: &InfoRequest{}
I0120 12:28:37.455211  124018 main.go:89] Received NotifyRegistrationStatus call: &RegistrationStatus{PluginRegistered:true,Error:,}
E0121 05:19:28.658390  124018 connection.go:129] Lost connection to unix:///csi/csi.sock.
E0125 07:11:42.926133  124018 connection.go:129] Lost connection to unix:///csi/csi.sock.
[...]
```

You should see the response `RegistrationStatus{PluginRegistered:true,Error:,}` in the logs to
confirm that plugin is registered with kubelet.

If you see a driver not found an error in the application pod describe output.
Restarting the `csi-xxxxplugin-xxx` pod on the node may help.

## Volume Attachment

Each provisioner pod also has a sidecar container called `csi-attacher`.

### csi-attacher

The external-attacher is a sidecar container that attaches volumes to nodes by calling `ControllerPublish` and
`ControllerUnpublish` functions of CSI drivers. It is necessary because the internal Attach/Detach controller
running in Kubernetes controller-manager does not have any direct interfaces to CSI drivers. More details can
be found [here](https://github.com/kubernetes-csi/external-attacher).

If any issue exists in attaching the PVC to the application pod first check the volumeattachment object created
and also log from csi-attacher sidecar container in provisioner pod.

```console
$ kubectl get volumeattachment
NAME                                                                   ATTACHER                        PV                                         NODE       ATTACHED   AGE
csi-75903d8a902744853900d188f12137ea1cafb6c6f922ebc1c116fd58e950fc92   rook-ceph.cephfs.csi.ceph.com   pvc-5c547d2a-fdb8-4cb2-b7fe-e0f30b88d454   minikube   true       4m26s
```

```console
kubectl logs po/csi-rbdplugin-provisioner-d857bfb5f-ddctl -c csi-attacher
```

## CephFS Stale operations

Check for any stale mount commands on the `csi-cephfsplugin-xxxx` pod on the node where your application pod is scheduled.

You need to exec in the `csi-cephfsplugin-xxxx` pod and grep for stale mount operators.

Identify the `csi-cephfsplugin-xxxx` pod running on the node where your application is scheduled with
`kubectl get po -o wide` and match the node names.

```console
$ kubectl exec -it csi-cephfsplugin-tfk2g -c csi-cephfsplugin -- sh
$ ps -ef |grep mount
[...]
root          67      60  0 11:55 pts/0    00:00:00 grep mount
```

```console
ps -ef |grep ceph
[...]
root           1       0  0 Jan20 ?        00:00:26 /usr/local/bin/cephcsi --nodeid=minikube --type=cephfs --endpoint=unix:///csi/csi.sock --v=0 --nodeserver=true --drivername=rook-ceph.cephfs.csi.ceph.com --pidlimit=-1 --metricsport=9091 --forcecephkernelclient=true --metricspath=/metrics --enablegrpcmetrics=true
root          69      60  0 11:55 pts/0    00:00:00 grep ceph
```

If any commands are stuck check the **dmesg** logs from the node.
Restarting the `csi-cephfsplugin` pod may also help sometimes.

If you don’t see any stuck messages, confirm the network connectivity, Ceph health, and slow ops.

## RBD Stale operations

Check for any stale `map/mkfs/mount` commands on the `csi-rbdplugin-xxxx` pod on the node where your application pod is scheduled.

You need to exec in the `csi-rbdplugin-xxxx` pod and grep for stale operators like (`rbd map, rbd unmap, mkfs, mount` and `umount`).

Identify the `csi-rbdplugin-xxxx` pod running on the node where your application is scheduled with
`kubectl get po -o wide` and match the node names.

```console
$ kubectl exec -it csi-rbdplugin-vh8d5 -c csi-rbdplugin -- sh
$ ps -ef |grep map
[...]
root     1297024 1296907  0 12:00 pts/0    00:00:00 grep map
```

```console
$ ps -ef |grep mount
[...]
root        1824       1  0 Jan19 ?        00:00:00 /usr/sbin/rpc.mountd
ceph     1041020 1040955  1 07:11 ?        00:03:43 ceph-mgr --fsid=ba41ac93-3b55-4f32-9e06-d3d8c6ff7334 --keyring=/etc/ceph/keyring-store/keyring --log-to-stderr=true --err-to-stderr=true --mon-cluster-log-to-stderr=true --log-stderr-prefix=debug  --default-log-to-file=false --default-mon-cluster-log-to-file=false --mon-host=[v2:10.111.136.166:3300,v1:10.111.136.166:6789] --mon-initial-members=a --id=a --setuser=ceph --setgroup=ceph --client-mount-uid=0 --client-mount-gid=0 --foreground --public-addr=172.17.0.6
root     1297115 1296907  0 12:00 pts/0    00:00:00 grep mount
```

```console
$ ps -ef |grep mkfs
[...]
root     1297291 1296907  0 12:00 pts/0    00:00:00 grep mkfs
```

```console
$ ps -ef |grep umount
[...]
root     1298500 1296907  0 12:01 pts/0    00:00:00 grep umount
```

```console
$ ps -ef |grep unmap
[...]
root     1298578 1296907  0 12:01 pts/0    00:00:00 grep unmap
```

If any commands are stuck check the **dmesg** logs from the node.
Restarting the `csi-rbdplugin` pod also may help sometimes.

If you don’t see any stuck messages, confirm the network connectivity, Ceph health, and slow ops.

## dmesg logs

Check the dmesg logs on the node where pvc mounting is failing or the `csi-rbdplugin` container of the
`csi-rbdplugin-xxxx` pod on that node.

```console
dmesg
```

## RBD Commands

If nothing else helps, get the last executed command from the ceph-csi pod logs and run it manually inside
the provisioner or plugin pod to see if there are errors returned even if they couldn't be seen in the logs.

```console
rbd ls --id=csi-rbd-node -m=10.111.136.166:6789 --key=AQDpIQhg+v83EhAAgLboWIbl+FL/nThJzoI3Fg==
```

Where `-m` is one of the mon endpoints and the `--key` is the key used by the CSI driver for accessing the Ceph cluster.

## Node Loss

When a node is lost, you will see application pods on the node stuck in the `Terminating` state while another pod is rescheduled and is in the `ContainerCreating` state.

!!! important
    For clusters with Kubernetes version 1.26 or greater, see the [improved automation](../Storage-Configuration/Block-Storage-RBD/block-storage.md#recover-rbd-rwo-volume-in-case-of-node-loss) to recover from the node loss. If using K8s 1.25 or older, continue with these instructions.

### Force deleting the pod

To force delete the pod stuck in the `Terminating` state:

```console
kubectl -n rook-ceph delete pod my-app-69cd495f9b-nl6hf --grace-period 0 --force
```

After the force delete, wait for a timeout of about 8-10 minutes. If the pod still not in the running state, continue with the next section to blocklist the node.

### Blocklisting a node

To shorten the timeout, you can mark the node as "blocklisted" from the [Rook toolbox](ceph-toolbox.md) so Rook can safely failover the pod sooner.

```console
$ ceph osd blocklist add <NODE_IP> # get the node IP you want to blocklist
blocklisting <NODE_IP>
```

After running the above command within a few minutes the pod will be running.

### Removing a node blocklist

After you are absolutely sure the node is permanently offline and that the node no longer needs to be blocklisted, remove the node from the blocklist.

```console
$ ceph osd blocklist rm <NODE_IP>
un-blocklisting <NODE_IP>
```
