---
title: Disaster Recovery
weight: 11600
indent: true
---

# Disaster Recovery

## Restoring Mon Quorum

Under extenuating circumstances, the mons may lose quorum. If the mons cannot form quorum again,
there is a manual procedure to get the quorum going again. The only requirement is that at least one mon
is still healthy. The following steps will remove the unhealthy
mons from quorum and allow you to form a quorum again with a single mon, then grow the quorum back to the original size.

For example, if you have three mons and lose quorum, you will need to remove the two bad mons from quorum, notify the good mon
that it is the only mon in quorum, and then restart the good mon.

### Stop the operator

First, stop the operator so it will not try to failover the mons while we are modifying the monmap

```console
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=0
```

### Inject a new monmap

> **WARNING**: Injecting a monmap must be done very carefully. If run incorrectly, your cluster could be permanently destroyed.

The Ceph monmap keeps track of the mon quorum. We will update the monmap to only contain the healthy mon.
In this example, the healthy mon is `rook-ceph-mon-b`, while the unhealthy mons are `rook-ceph-mon-a` and `rook-ceph-mon-c`.

Take a backup of the current `rook-ceph-mon-b` Deployment:

```console
kubectl -n rook-ceph get deployment rook-ceph-mon-b -o yaml > rook-ceph-mon-b-deployment.yaml
```

Open the file and copy the `command` and `args` from the `mon` container (see `containers` list). This is needed for the monmap changes.
Cleanup the copied `command` and `args` fields to form a pastable command.
Example:

The following parts of the `mon` container:

```yaml
[...]
  containers:
  - args:
    - --fsid=41a537f2-f282-428e-989f-a9e07be32e47
    - --keyring=/etc/ceph/keyring-store/keyring
    - --log-to-stderr=true
    - --err-to-stderr=true
    - --mon-cluster-log-to-stderr=true
    - '--log-stderr-prefix=debug '
    - --default-log-to-file=false
    - --default-mon-cluster-log-to-file=false
    - --mon-host=$(ROOK_CEPH_MON_HOST)
    - --mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)
    - --id=b
    - --setuser=ceph
    - --setgroup=ceph
    - --foreground
    - --public-addr=10.100.13.242
    - --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db
    - --public-bind-addr=$(ROOK_POD_IP)
    command:
    - ceph-mon
[...]
```

Should be made into a command like this: (**do not copy the example command!**)

```console
ceph-mon \
    --fsid=41a537f2-f282-428e-989f-a9e07be32e47 \
    --keyring=/etc/ceph/keyring-store/keyring \
    --log-to-stderr=true \
    --err-to-stderr=true \
    --mon-cluster-log-to-stderr=true \
    --log-stderr-prefix=debug \
    --default-log-to-file=false \
    --default-mon-cluster-log-to-file=false \
    --mon-host=$(ROOK_CEPH_MON_HOST) \
    --mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS) \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$(ROOK_POD_IP)
```

(be sure to remove the single quotes around the `--log-stderr-prefix` flag)

Patch the `rook-ceph-mon-b` Deployment to run a sleep instead of the `ceph mon` command:

```console
kubectl -n rook-ceph patch deployment rook-ceph-mon-b -p '{"spec": {"template": {"spec": {"containers": [{"name": "mon", "command": ["sleep", "infinity"], "args": []}]}}}}'
```

Connect to the pod of a healthy mon and run the following commands.

```console
kubectl -n rook-ceph exec -it <mon-pod> bash

# set a few simple variables
cluster_namespace=rook-ceph
good_mon_id=b
monmap_path=/tmp/monmap

# extract the monmap to a file, by pasting the ceph mon command
# from the good mon deployment and adding the
# `--extract-monmap=${monmap_path}` flag
ceph-mon \
    --fsid=41a537f2-f282-428e-989f-a9e07be32e47 \
    --keyring=/etc/ceph/keyring-store/keyring \
    --log-to-stderr=true \
    --err-to-stderr=true \
    --mon-cluster-log-to-stderr=true \
    --log-stderr-prefix=debug \
    --default-log-to-file=false \
    --default-mon-cluster-log-to-file=false \
    --mon-host=$(ROOK_CEPH_MON_HOST) \
    --mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS) \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$(ROOK_POD_IP) \
    --extract-monmap=${monmap_path}

# review the contents of the monmap
monmaptool --print /tmp/monmap

# remove the bad mon(s) from the monmap
monmaptool ${monmap_path} --rm <bad_mon>

# in this example we remove mon0 and mon2:
monmaptool ${monmap_path} --rm a
monmaptool ${monmap_path} --rm c

# inject the modified monmap into the good mon, by pasting
# the ceph mon command and adding the
# `--inject-monmap=${monmap_path}` flag, like this
ceph-mon \
    --fsid=41a537f2-f282-428e-989f-a9e07be32e47 \
    --keyring=/etc/ceph/keyring-store/keyring \
    --log-to-stderr=true \
    --err-to-stderr=true \
    --mon-cluster-log-to-stderr=true \
    --log-stderr-prefix=debug \
    --default-log-to-file=false \
    --default-mon-cluster-log-to-file=false \
    --mon-host=$(ROOK_CEPH_MON_HOST) \
    --mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS) \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$(ROOK_POD_IP) \
    --inject-monmap=${monmap_path}
```

Exit the shell to continue.

### Edit the Rook configmaps

Edit the configmap that the operator uses to track the mons.

```console
kubectl -n rook-ceph edit configmap rook-ceph-mon-endpoints
```

In the `data` element you will see three mons such as the following (or more depending on your `moncount`):

```yaml
data: a=10.100.35.200:6789;b=10.100.13.242:6789;c=10.100.35.12:6789
```

Delete the bad mons from the list, for example to end up with a single good mon:

```yaml
data: b=10.100.13.242:6789
```

Save the file and exit.

Now we need to adapt a Secret which is used for the mons and other components.
The following `kubectl patch` command is an easy way to do that. In the end it patches the `rook-ceph-config` secret and updates the two key/value pairs `mon_host` and `mon_initial_members`.

```console
mon_host=$(kubectl -n rook-ceph get svc rook-ceph-mon-b -o jsonpath='{.spec.clusterIP}')
kubectl -n rook-ceph patch secret rook-ceph-config -p '{"stringData": {"mon_host": "[v2:'"${mon_host}"':3300,v1:'"${mon_host}"':6789]", "mon_initial_members": "'"${good_mon_id}"'"}}'
```

> **NOTE**: If you are using `hostNetwork: true`, you need to replace the `mon_host` var with the node IP the mon is pinned to (`nodeSelector`). This is because there is no `rook-ceph-mon-*` service created in that "mode".

### Restart the mon

You will need to "restart" the good mon pod with the original `ceph-mon` command to pick up the changes. For this run `kubectl replace` on the backup of the mon deployment yaml:

```console
kubectl replace --force -f rook-ceph-mon-b-deployment.yaml
```

> **NOTE**: Option `--force` will delete the deployment and create a new one

Start the rook [toolbox](/Documentation/ceph-toolbox.md) and verify the status of the cluster.

```console
ceph -s
```

The status should show one mon in quorum. If the status looks good, your cluster should be healthy again.

### Restart the operator
Start the rook operator again to resume monitoring the health of the cluster.
```console
# create the operator. it is safe to ignore the errors that a number of resources already exist.
kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas=1
```

The operator will automatically add more mons to increase the quorum size again, depending on the `mon.count`.

# Adopt an existing Rook Ceph cluster into a new Kubernetes cluster

## Scenario

1. The Kubernetes environment underlying a running Rook Ceph cluster failed catastrophically, requiring a new Kubernetes environment in which the user wishes to recover the previous Rook Ceph cluster.
2. The user wishes to migrate their existing Rook Ceph cluster to a new Kubernetes environment, and downtime can be tolerated.

### Prerequisites

1. A working Kubernetes cluster to which we will migrate the previous Rook Ceph cluster.
2. At least one Ceph mon db is in quorum, and sufficient number of Ceph OSD is `up` and `in` before disaster.
3. The previous Rook Ceph cluster is not running.

### Overview for Steps below

1. Start a new and clean Rook Ceph cluster, with old `CephCluster` `CephBlockPool` `CephFilesystem` `CephNFS` `CephObjectStore`.
2. Shut the new cluster down when it has been created successfully.
3. Replace ceph-mon data with that of the old cluster.
4. Replace `fsid` in `secrets/rook-ceph-mon` with that of the old one.
5. Fix monmap in ceph-mon db.
6. Fix ceph mon auth key.
7. Disable auth.
8. Start the new cluster, watch it resurrect.
9. Fix admin auth key, and enable auth.
10. Restart cluster for the final time.

### Steps

Assuming `dataHostPathData` is `/var/lib/rook`, and the `CephCluster` trying to adopt is named `rook-ceph`.

1. Make sure the old Kubernetes cluster is completely torn down and the new Kubernetes cluster is up and running without Rook Ceph.
1. Backup `/var/lib/rook` in all the Rook Ceph nodes to a different directory. Backups will be used later.
1. Pick a `/var/lib/rook/rook-ceph/rook-ceph.config` from any previous Rook Ceph node and save the old cluster `fsid` from its content.
1. Remove `/var/lib/rook` from all the Rook Ceph nodes.
1. Add identical `CephCluster` descriptor to the new Kubernetes cluster, especially identical `spec.storage.config` and `spec.storage.nodes`, except `mon.count`, which should be set to `1`.
1. Add identical `CephFilesystem` `CephBlockPool` `CephNFS` `CephObjectStore` descriptors (if any) to the new Kubernetes cluster.
1. Install Rook Ceph in the new Kubernetes cluster.
1. Watch the operator logs with `kubectl -n rook-ceph logs -f rook-ceph-operator-xxxxxxx`, and wait until the orchestration has settled.
1. **STATE**: Now the cluster will have `rook-ceph-mon-a`, `rook-ceph-mgr-a`, and all the auxiliary pods up and running, and zero (hopefully) `rook-ceph-osd-ID-xxxxxx` running. `ceph -s` output should report 1 mon, 1 mgr running, and all of the OSDs down, all PGs are in `unknown` state. Rook should not start any OSD daemon since all devices belongs to the old cluster (which have a different `fsid`).
1. Run `kubectl -n rook-ceph exec -it rook-ceph-mon-a-xxxxxxxx bash` to enter the `rook-ceph-mon-a` pod,

    ```shell
    mon-a# cat /etc/ceph/keyring-store/keyring  # save this keyring content for later use
    mon-a# exit
    ```

1. Stop the Rook operator by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `0`.
1. Stop cluster daemons by running `kubectl -n rook-ceph delete deploy/X` where X is every deployment in namespace `rook-ceph`, except `rook-ceph-operator` and `rook-ceph-tools`.
1. Save the `rook-ceph-mon-a` address with `kubectl -n rook-ceph get cm/rook-ceph-mon-endpoints -o yaml` in the new Kubernetes cluster for later use.

1. SSH to the host where `rook-ceph-mon-a` in the new Kubernetes cluster resides.
    1. Remove `/var/lib/rook/mon-a`
    2. Pick a healthy `rook-ceph-mon-ID` directory (`/var/lib/rook/mon-ID`) in the previous backup, copy to `/var/lib/rook/mon-a`. `ID` is any healthy mon node ID of the old cluster.
    3. Replace `/var/lib/rook/mon-a/keyring` with the saved keyring, preserving only the `[mon.]` section, remove `[client.admin]` section.
    4. Run `docker run -it --rm -v /var/lib/rook:/var/lib/rook ceph/ceph:v14.2.1-20190430 bash`. The Docker image tag should match the Ceph version used in the Rook cluster. The `/etc/ceph/ceph.conf` file needs to exist for `ceph-mon` to work.

        ```shell
        container# touch /etc/ceph/ceph.conf
        container# cd /var/lib/rook
        container# ceph-mon --extract-monmap monmap --mon-data ./mon-a/data  # Extract monmap from old ceph-mon db and save as monmap
        container# monmaptool --print monmap  # Print the monmap content, which reflects the old cluster ceph-mon configuration.
        container# monmaptool --rm a monmap  # Delete `a` from monmap.
        container# monmaptool --rm b monmap  # Repeat, and delete `b` from monmap.
        container# monmaptool --rm c monmap  # Repeat this pattern until all the old ceph-mons are removed
        container# monmaptool --rm d monmap
        container# monmaptool --rm e monmap
        container# monmaptool --add a 10.77.2.216:6789 monmap   # Replace it with the rook-ceph-mon-a address you got from previous command.
        container# ceph-mon --inject-monmap monmap --mon-data ./mon-a/data  # Replace monmap in ceph-mon db with our modified version.
        container# rm monmap
        container# exit
        ```

1. Tell Rook to run as old cluster by running `kubectl -n rook-ceph edit secret/rook-ceph-mon` and changing `fsid` to the original `fsid`. Note that the `fsid` is base64 encoded and must not contain a trailing carriage return. For example:

    ```shell
    echo -n a811f99a-d865-46b7-8f2c-f94c064e4356 | base64  # Replace with the fsid from your old cluster.
    ```

1. Disable authentication by running `kubectl -n rook-ceph edit cm/rook-config-override` and adding content below:

    ```yaml
    data:
    config: |
        [global]
        auth cluster required = none
        auth service required = none
        auth client required = none
        auth supported = none
    ```

1. Bring the Rook Ceph operator back online by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `1`.
1. Watch the operator logs with `kubectl -n rook-ceph logs -f rook-ceph-operator-xxxxxxx`, and wait until the orchestration has settled.
1. **STATE**: Now the new cluster should be up and running with authentication disabled. `ceph -s` should report 1 mon & 1 mgr & all of the OSDs up and running, and all PGs in either `active` or `degraded` state.
1. Run `kubectl -n rook-ceph exec -it rook-ceph-tools-XXXXXXX bash` to enter tools pod:

    ```console
    tools# vi key
    [paste keyring content saved before, preserving only `[client admin]` section]
    tools# ceph auth import -i key
    tools# rm key
    ```

1. Re-enable authentication by running `kubectl -n rook-ceph edit cm/rook-config-override` and removing auth configuration added in previous steps.
1. Stop the Rook operator by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `0`.
1. Shut down entire new cluster by running `kubectl -n rook-ceph delete deploy/X` where X is every deployment in namespace `rook-ceph`, except `rook-ceph-operator` and `rook-ceph-tools`, again. This time OSD daemons are present and should be removed too.
1. Bring the Rook Ceph operator back online by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `1`.
1. Watch the operator logs with `kubectl -n rook-ceph logs -f rook-ceph-operator-xxxxxxx`, and wait until the orchestration has settled.
1. **STATE**: Now the new cluster should be up and running with authentication enabled. `ceph -s` output should not change much comparing to previous steps.


# Adopt an existing Rook Ceph cluster (on PVs) into a new Kubernetes cluster

> This section was written for a cluster running in a dynamic environment where the MONs and OSDs were running on PVs.
> For example, the cluster may have been configured from [cluster-on-pvc.yaml](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/cluster-on-pvc.yaml).

## Scenario

1. The Kubernetes environment underlying a running Rook Ceph cluster failed catastrophically, requiring a new Kubernetes environment in which the user wishes to recover the previous Rook Ceph cluster.
2. The underlying PVs with the Ceph data (OSDs) and metadata (MONs) are still available in the cloud environment.

### Prerequisites

1. A working Kubernetes cluster to which we will migrate the previous Rook Ceph cluster.
2. Before the disaster, at least one Ceph mon db was in quorum, and a sufficient number of Ceph OSDs were `up` and `in`.
3. The required resources were exported from the previous K8s cluster before it went down.

### Exporting Critical Info

Critical keys and info about the mons must be exported from the original cluster. This info is not stored on the PVs by either the mons
or osds. This info is necessary to restore the cluster in case of disaster.

```
mkdir critical
export namespace=rook-ceph
kubectl -n ${namespace} get secret rook-ceph-mon -o yaml > critical/rook-ceph-mon.yaml
kubectl -n ${namespace} get cm rook-ceph-mon-endpoints -o yaml > critical/rook-ceph-mon-endpoints.yaml
kubectl -n ${namespace} get svc -l app=rook-ceph-mon -o yaml > critical/rook-ceph-mon-svc.yaml
# information about PVCs and PVs to help reconstruct them later
# TODO: Can we just export these as yamls and import them again directly? At a minimum we would need to filter the PV list since more than Rook PVs would be included.
kubectl -n ${namespace} get pv -o yaml > critical/pvs.txt
kubectl -n ${namespace} get pvc -o yaml > critical/pvcs.txt
```

### Restoring the Cluster

1. Start the new Kubernetes clusterr

2. Modify the critical resources before creating them
   - Remove all of the `ownerReferences` sections from the resources in `critical/*.yaml`
   - TODO: Do we need to remove other properties such as `uid`?

3. Import the critical resources that you had exported from the original cluster

```console
kubectl create namespace rook-ceph
kubectl create -f critical/
```

3. Verify that the volumes for the MONs and OSDs are still available from the previous cluster

   <TODO: How to detect that volumes belonged to Rook?>

4. Create PVs for the volumes. If available, refer to `critical/pvs.txt` to know which volumes existed previously.

   <TODO: Commands to create the PVs>

   <TODO: How do we know which PVs belonged to the MONs or OSDs? The volumes just have random names. Do we need to rely on the PV size to indicate
   which ones were expected to belong to MONs or OSDs?>

5. Create PVCs for the MON volumes. The PVCs must follow the Rook naming convention:
   `rook-ceph-mon-<ID>` where `<ID>` is the name of the mon. Typically, the `<ID>` will be `a`, `b`, or `c`.
   See `critica/pvcs.txt` as a reference for the PVCs that existed in the original cluster.

   On a running cluster, you might see these PVs:
```console
$ kubectl -n rook-ceph get pvc -l app=rook-ceph-mon
NAME              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
rook-ceph-mon-a   Bound    pvc-0cf0e834-1c55-4aec-b1d9-af998382a03c   10Gi       RWO            gp2            31m
rook-ceph-mon-b   Bound    pvc-d6ed5e13-47ac-48ab-8e63-c1f0d9c10c26   10Gi       RWO            gp2            31m
rook-ceph-mon-c   Bound    pvc-ed3911dc-eb39-4335-8561-04497670e9ee   10Gi       RWO            gp2            30m
```

6. Bind the MON PVCs to the PVs. When binding the MON PVs it is important to know that the correct PV must be bound to the mon with the correct name.
   For example, the PV for `mon-a` must be bound to the PVC from `mon-a`.
   - If fewer than three mon PVs are available, you may need to follow the instructions above to [restore mon quorum](#restoring-mon-quorum).

   <TODO: How to know which PV belonged to which mon?>

   <TODO: Commands to bind the PVCs to the PVs>

7. Create PVCs for the OSD volumes.
   - The PVCs must follow the Rook naming convention `<device-set-name>-<index>-<type>-<suffix>` where
     - `device-set-name` is the `name` of the `storageClassDeviceSet` in the clusterCR. The default example in `cluster-on-pvc.yaml` names this `set1`.
     - `index` is a zero-based index of the PVC for the device set. If the `count` of PVCs in the device set is `3`, you will see `0`, `1`, and `2` in this position.
     - `type` is either `data` or `metadata` and corresponds to the `name` of the `volumeClaimTemplate` in the `storageClassDeviceSet`.
     - `suffix` is a generated random name to make the PVC name unique. There is no need to add the suffix if you are creating the PVCs manually.
   - Labels must be added to each of the PVCs so they can be identified correctly as belonging to the `storageClassDeviceSet`.
     The `DeviceSet` label is the same as the `device-set-name` mentioned above and the `DeviceSetPVCId` matches the `device-set-name` and `index` separated by a hyphen.

```yaml
  labels:
    ceph.rook.io/DeviceSet: set1
    ceph.rook.io/DeviceSetPVCId: set1-0
    ceph.rook.io/setIndex: "0"
```

   On a running cluster, you might see these PVs:
```console
$ oc get pvc -l ceph.rook.io/DeviceSet=set1
NAME                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
set1-0-data-msszk   Bound    pvc-aa7814f5-7d6d-44e4-98c0-ceabbd2a3d6d   10Gi       RWO            gp2            45m
set1-1-data-plxzm   Bound    pvc-d7eff2e9-b6d5-4b67-a291-534df243127c   10Gi       RWO            gp2            45m
set1-2-data-26hfh   Bound    pvc-c235b126-7a47-4d11-83c3-4b441f4a5456   10Gi       RWO            gp2            45m
```

8. Bind the OSD PVCs to the PVs. It is not critcal to match the same volume to the same device set or index as in the original cluster,
   so the OSD binding is more relaxed than the MON bindings.

   <TODO: Commands to bind the PVCs to the PVs>

9. Start the Rook operator

```console
kubectl create -f common.yaml
kubectl create -f operator.yaml
```

10. Create the Rook cluster

```console
# Use the same cluster settings that you had previously. This cluster-on-pvc.yaml is just an example.
kubectl create -f cluster-on-pvc.yaml
```

11. Watch for the MON daemons to start and form quorum. If you see the `rook-ceph-mgr-a` deployment created, this
    means the quorum was successfully formed. Also see the logs from the operator pod to see if it is waiting for quorum.
    If the mons don't form quorum, either there was a problem attaching the volumes to the mons, or the mon endpoints are
    incorrect from the services that were imported from the previous cluster. The mon endpoints are part of their identity
    and cannot change. If they do need to change, see the section above on [restoring mon quorum](#restoring-mon-quorum).

12. Verify that the cluster is working. You should see three MONs, some number of OSDs, and one MGR daemon running.
    Connect to the [Rook Toolbox](ceph-toolbox.md) and execute some Ceph commands to view the health.

13. Create any other pool, filesystem, or object store CRs that you had in your previous clusters.
    Note that the data in the pools will not be re-created. Rook will recognize the pools already exist and skip creating them again.
