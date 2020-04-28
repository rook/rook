# Ceph cluster clean up policy

## Use case

As a rook user, I want to clean up data on the hosts after I intentionally uninstall ceph cluster, so that I can start a new cluster without having to do any manual clean up.

## Background

### Host-based cluster deletion

If the user deletes a host-based cluster (running on raw devices and not on PVs) and starts a new cluster on the same hosts, the path used by dataDirHostPath must be deleted. Otherwise, stale keys and other config will remain from the previous cluster and the new mons will fail to start. As of now, the user has to manually delete the dataDirHostPath on each host. This implementation aims to automate this deletion.

## Design Flow

### User confirmation

- **Important**: User confirmation is mandatory before cleaning up the data on hosts. This is important because user might have accidentally deleted the CR and in that case cleaning up the hostpath won’t recover the cluster.
- Adding these user confirmation on the ceph cluster would cause the operator to refuse running an orchestration

### How to add user confirmation

- If the user really wants to clean up the data on the cluster, then user should update the ceph cluster CRD with cleanupPolicy configuration like below :

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v14.2.7
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: true
  storage:
    useAllNodes: true
    useAllDevices: true
  cleanupPolicy:
    deleteDataDirOnHosts: yes-really-destroy-data
```

- Adding `cleanupPolicy` would cause the operator to refuse running an orchestration

### How the Operator cleans up the cluster

- Operator starts the clean up flow only when deletionTimeStamp is present on the ceph Cluster.
- Operator checks for user confirmation (for example `deleteDataDirOnHosts: yes-really-destroy-data`) on the ceph cluster before starting the clean up.
- Identify the nodes where ceph daemons are running.
- Wait till all the ceph daemons are destroyed on each node. This is important because deleting the data (say dataDirHostPath) before the daemons would cause the daemons to panic.
- Create a batch job that runs on each of the above nodes.
- The job performs the following action on each node based on the user confirmation:
  - cleanup the cluster namespace on the dataDirHostPath
  - Delete all the ceph monitor directories on the dataDirHostPath. For example mon-a, mon-b, etc.
  - Clean up the devices on each node.

#### Cleanup Job Spec:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: rook-ceph-cleanup-<node-name>
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
          args: []string{"ceph", "clean"}
          volumeMounts:
            - name: cleanup-volume
              # data dir host path that needs to be cleaned up.
              mountPath: <dataDirHostPath>
      volume:
        - name: cleanup-volume
          hostPath:
            #directory location on the host
            path: <dataDirHostPath>
      restartPolicy: Never
```
