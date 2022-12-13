# Ceph 2 (More) CRDs

## Use Case

For Rook users it would be great to be able to easily view, create and manage certain Ceph components through additional Kubernetes CRDs.

Currently to view certain status of the Ceph cluster, e.g., health status, OSD tree, one needs to run `ceph` commands in the Rook toolbox.
To make this easier the `rook-ceph` krew plugin has "aliases"/ automations for these tasks/ processes, see [krew Plugin section](#krew-plugin).

The krew plugin is great for getting input from users (e.g., "wizards"), but it would be great to be able to also just run `kubectl get -n rook-ceph cephosds` and see the OSDs in your cluster.

**Example**: Creating an OSD, requires a configuration change to the CephCluster CustomResource and/ or depending on the case the device hotplug detection to be enabled in a cluster.

This design wouldn't take away the "manual" configuration change, but "split" the operator into a pure observatory layer of making sure the desired state is achieved and the executive layer for creating and managing daemons.

### krew Plugin

For many tasks around Ceph it is necessary to exec into the Rook toolbox, to address this from a general point Kubernetes has added ["plugin" support to `kubectl`](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/).
That is where the [Krew plugin manager for `kubectl`](https://github.com/kubernetes-sigs/krew/) has been used by many projects and Rook to extend the `kubectl` command.

**Examples**:

```console
$ kubectl rook-ceph health
Info:  Checking if at least three mon pods are running on different nodes
rook-ceph-mon-b-857ff8c945-46qrz                                  1/1     Running     1              34d
rook-ceph-mon-f-68dccd88f6-sh66s                                  1/1     Running     0              34d
rook-ceph-mon-g-75db648b86-g5tgn                                  1/1     Running     0              34d

Info:  Checking mon quorum and ceph health details
[...]

$ kubectl rook-ceph ceph -s
  cluster:
[...]
  services:
    mon: 3 daemons, quorum b,f,g (age 4w)
    mgr: a(active, since 4w), standbys: b
    mds: 1/1 daemons up, 1 hot standby
    osd: 10 osds: 10 up (since 2d), 10 in (since 5w)
    rgw: 2 daemons active (2 hosts, 1 zones)
[...]
```

(Parts of the command outputs has been omitted to save lines)

The `rook-ceph` Krew plugin offers a great way to make interacting with Ceph easier (e.g., disaster recovery processes).
The plugin technically allows for interactive input from the user.

## Design

Split the operator into two major logics.

1. "Observation"/ Management layer
    1. Detect new disks.
    2. Take care of changes to the CephCluster object (e.g., new PVCs based OSDs)
    3. Health check components and react accordingly by creating/updating/deleting `Ceph*` objects.
2. "Executive" Layer
    1. Take care of deploying the Ceph components to the cluster.

### Focus: OSD

Instead of creating OSDs directly a CephOSD object is created by the operator per device.

#### CustomResourceDefinition - CephOSD

```yaml
apiVersion: ceph.rook.io/v1
kind: CephOSD
metadata:
  name: osd-0
  namespace: rook-ceph
spec:
  # Ability to "pause" the daemon (scale to 0)
  paused: false
  # Per daemon Ceph version
  cephVersion:
    image: koorinc/koor-ceph-container:v17.2.5-20221017
  affinity: {}
  resources: {}
  # OSD related parameters for creation/ running the OSD
  parameters:
    osdsPerDevice: 3
    #min_alloc_size: 4k
  disks:
    data:
      # For Bare metal with non PV disks
      localDisk:
        nodeName: k8s-node-storage-01
        deviceFilter: "^sd."
        devices:
          - name: sdb
      # For a PVC backed OSD
      volumeClaimTemplate:
        apiVersion: v1
        kind: PersistentVolumeClaim
        spec:
            accessModes:
            - ReadWriteOnce
            resources:
            requests:
                storage: 10Gi
            storageClassName: awsebs
            volumeMode: Block
    # A "VolumeClaimTemplate" can be specified for other disks for an OSD as well
    # metadata:
  # Override global healthCheck settings
  healthCheck:
    daemonHealth:
      disabled: false
      interval: 60s
status:
  state: Created
  phase: Ready
  ceph:
    health: OK
    capacity:
      objects: 235113
      bytesAvailable: 240576078
      bytesTotal: 4036612172
      bytesUsed: 163085139
    scrub:
      lastScrub: 2022-12-13 16:13:41
```

### Focus: Other Components

Technically speaking this could be added for other Ceph components such as the MONs, MGR, RGWs as well in some way.

Example for RGW: Creating an CephObjectStore would cause the "Observation"/ Management layer to simply create the amount of CephRGWDaemon objects that are requested by the user.

## Summary

This might be a thing for a **future major** Rook release to completely re-write the logic on how Rook handles the Ceph components to be more independent and "expressive" through objects for "everything"(/ most things).
In my eyes this removes logic from certain areas of the operator in regards to the direct "creation" and management of Ceph components, so that the "Observation"/ Management layer can fully just orchestrate from the top.

E.g., changing OSD store type could be handled with ease from the management layer by simply updating the CephOSD objects (using them as the "abstraction" layer) and the "Executive" layer than takes care of scheduling the necessary Kubernetes `Jobs`.

The CephCluster object would and could still be used to create and manage OSDs though in a way that the management layer would take care of creating the necessary CephOSD and other objects.
