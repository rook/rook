# Ceph cluster clean up policy

## Use case

As a rook user, I want to clean up data on the hosts after I intentionally uninstall ceph cluster, so that I can start a new cluster without having to do any manual clean up.

## Background

### Cluster deletion
If the user deletes a rook-ceph cluster and wants to start a new cluster on the same hosts, then following manual steps should be performed:
- Delete the dataDirHostPath on each host. Otherwise, stale keys and other configs will remain from the previous cluster and the new mons will fail to start.
- Clean the OSD disks from the previous cluster before starting a new one.

Read more about the manual clean up steps [here](https://github.com/rook/rook/blob/master/Documentation/Storage-Configuration/ceph-teardown.md#delete-the-data-on-hosts)

This implementation aims to automate both of these manual steps.

## Design Flow

### User confirmation

- **Important**: User confirmation is mandatory before cleaning up the data on hosts. This is important because user might have accidentally deleted the CR and in that case cleaning up the hostpath won’t recover the cluster.
- Adding these user confirmation on the ceph cluster would cause the operator to refuse running an orchestration

### How to add user confirmation

- If the user really wants to clean up the data on the cluster, then update the ceph cluster CRD with cleanupPolicy configuration like below :

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v17.2.6
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  storage:
    useAllNodes: true
    useAllDevices: true
  cleanupPolicy:
    confirmation: yes-really-destroy-data
    sanitizeDisks:
      method: quick
      dataSource: zero
      iteration: 1
    allowUninstallWithVolumes: false
```

- Updating the cluster `cleanupPolicy` with `confirmation: yes-really-destroy-data` would cause the operator to refuse running any further orchestration.

### How the Operator cleans up the cluster

- Operator starts the clean up flow only when deletionTimeStamp is present on the ceph Cluster.
- Operator checks for user confirmation (that is, `confirmation: yes-really-destroy-data`) on the ceph cluster before starting the clean up.
- Identify the nodes where ceph daemons are running.
- Wait till all the ceph daemons are destroyed on each node. This is important because deleting the data (say dataDirHostPath) before the daemons would cause the daemons to panic.
- Create a batch job that runs on each of the above nodes.
- The job performs the following action on each node based on the user confirmation:
  - cleanup the cluster namespace on the dataDirHostPath. For example `/var/lib/rook/rook-ceph`
  - Delete all the ceph monitor directories on the dataDirHostPath. For example `/var/lib/rook/mon-a`, `/var/lib/rook/mon-b`, etc.
  - Sanitize the local disks used by OSDs on each node.
- Local disk sanitization can be further configured by the admin with following options:
  - `method`: use `complete` to sanitize the entire disk and `quick` (default) to sanitize only ceph's metadata.
  - `dataSource`: indicate where to get random bytes from to write on the disk. Possible choices are `zero` (default) or `random`.
  Using random sources will consume entropy from the system and will take much more time then the zero source.
  - `iteration`: overwrite N times instead of the default (1). Takes an integer value.
- If `allowUninstallWithVolumes` is `false` (default), then operator would wait for the PVCs to be deleted before finally deleting the cluster.

#### Cleanup Job Spec:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: cluster-cleanup-job-<node-name>
  namespace: <namespace>
  labels:
    app: rook-ceph-cleanup
    rook-ceph-cleanup: "true"
    rook_cluster: <namespace>
spec:
  template:
    spec:
      containers:
        - name: rook-ceph-cleanup-<node-name>
          securityContext:
            privileged: true
          image: <rook-image>
          env:
          # if ROOK_DATA_DIR_HOST_PATH is available, then delete the dataDirHostPath
          - name: ROOK_DATA_DIR_HOST_PATH
            value: <dataDirHostPath>
          - name: ROOK_NAMESPACE_DIR
            value: <namespace>
          - name: ROOK_MON_SECRET
            value: <dataDirHostPath>
          - name: ROOK_CLUSTER_FSID
            value: <dataDirHostPath>
          - name: ROOK_LOG_LEVEL
            value: <dataDirHostPath>
          - name: ROOK_SANITIZE_METHOD
            value: <method>
          - name: ROOK_SANITIZE_DATA_SOURCE
            value: <dataSource>
            - name: ROOK_SANITIZE_ITERATION
            value: <iteration>
          args: []string{"ceph", "clean"}
          volumeMounts:
            - name: cleanup-volume
              # data dir host path that needs to be cleaned up.
              mountPath: <dataDirHostPath>
            - name: devices
              mountPath: /dev
      volume:
        - name: cleanup-volume
          hostPath:
            #directory location on the host
            path: <dataDirHostPath>
        - name: devices
          hostpath:
            path: /dev
      restartPolicy: OnFailure
```
