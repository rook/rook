---
title: Disaster Recovery
---

Under extenuating circumstances, steps may be necessary to recover the cluster health. There are several types of recovery addressed in this document.

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

!!! warning
    Injecting a monmap must be done very carefully. If run incorrectly, your cluster could be permanently destroyed.

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
    --mon-host=$ROOK_CEPH_MON_HOST \
    --mon-initial-members=$ROOK_CEPH_MON_INITIAL_MEMBERS \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$ROOK_POD_IP
```

(be sure to remove the single quotes around the `--log-stderr-prefix` flag and the parenthesis around the variables being passed ROOK_CEPH_MON_HOST, ROOK_CEPH_MON_INITIAL_MEMBERS and ROOK_POD_IP )

Patch the `rook-ceph-mon-b` Deployment to stop this mon working without deleting the mon pod:

```console
kubectl -n rook-ceph patch deployment rook-ceph-mon-b  --type='json' -p '[{"op":"remove", "path":"/spec/template/spec/containers/0/livenessProbe"}]'

kubectl -n rook-ceph patch deployment rook-ceph-mon-b -p '{"spec": {"template": {"spec": {"containers": [{"name": "mon", "command": ["sleep", "infinity"], "args": []}]}}}}'
```

Connect to the pod of a healthy mon and run the following commands.

```console
$ kubectl -n rook-ceph exec -it <mon-pod> bash
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
    --mon-host=$ROOK_CEPH_MON_HOST \
    --mon-initial-members=$ROOK_CEPH_MON_INITIAL_MEMBERS \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$ROOK_POD_IP \
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
    --mon-host=$ROOK_CEPH_MON_HOST \
    --mon-initial-members=$ROOK_CEPH_MON_INITIAL_MEMBERS \
    --id=b \
    --setuser=ceph \
    --setgroup=ceph \
    --foreground \
    --public-addr=10.100.13.242 \
    --setuser-match-path=/var/lib/ceph/mon/ceph-b/store.db \
    --public-bind-addr=$ROOK_POD_IP \
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

!!! note
    If you are using `hostNetwork: true`, you need to replace the `mon_host` var with the node IP the mon is pinned to (`nodeSelector`). This is because there is no `rook-ceph-mon-*` service created in that "mode".

### Restart the mon

You will need to "restart" the good mon pod with the original `ceph-mon` command to pick up the changes. For this run `kubectl replace` on the backup of the mon deployment yaml:

```console
kubectl replace --force -f rook-ceph-mon-b-deployment.yaml
```

!!! note
    Option `--force` will delete the deployment and create a new one

Start the rook [toolbox](ceph-toolbox.md) and verify the status of the cluster.

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

## Restoring CRDs After Deletion

When the Rook CRDs are deleted, the Rook operator will respond to the deletion event to attempt to clean up the cluster resources.
If any data appears present in the cluster, Rook will refuse to allow the resources to be deleted since the operator will
refuse to remove the finalizer on the CRs until the underlying data is deleted. For more details, see the
[dependency design doc](https://github.com/rook/rook/blob/master/design/ceph/resource-dependencies.md).

While it is good that the CRs will not be deleted and the underlying Ceph data and daemons continue to be
available, the CRs will be stuck indefinitely in a `Deleting` state in which the operator will not
continue to ensure cluster health. Upgrades will be blocked, further updates to the CRs are prevented, and so on.
Since Kubernetes does not allow undeleting resources, the following procedure will allow you to restore
the CRs to their prior state without even necessarily suffering cluster downtime.

1. Scale down the operator

```console
kubectl -n rook-ceph scale --replicas=0 deploy/rook-ceph-operator
```

2. Backup all Rook CRs and critical metadata

```console
# Store the CephCluster CR settings. Also, save other Rook CRs that are in terminating state.
kubectl -n rook-ceph get cephcluster rook-ceph -o yaml > cluster.yaml

# Backup critical secrets and configmaps in case something goes wrong later in the procedure
kubectl -n rook-ceph get secret -o yaml > secrets.yaml
kubectl -n rook-ceph get configmap -o yaml > configmaps.yaml
```

3. Remove the owner references from all critical Rook resources that were referencing the CephCluster CR.
   The critical resources include:
   - Secrets: `rook-ceph-admin-keyring`, `rook-ceph-config`, `rook-ceph-mon`, `rook-ceph-mons-keyring`
   - ConfigMap: `rook-ceph-mon-endpoints`
   - Services: `rook-ceph-mon-*`, `rook-ceph-mgr-*`
   - Deployments: `rook-ceph-mon-*`, `rook-ceph-osd-*`, `rook-ceph-mgr-*`
   - PVCs (if applicable): `rook-ceph-mon-*` and the OSD PVCs (named `<deviceset>-*`, for example `set1-data-*`)

For example, remove this entire block from each resource:

```yaml
ownerReferences:
- apiVersion: ceph.rook.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: CephCluster
    name: rook-ceph
    uid: <uid>
```

4. **After confirming all critical resources have had the owner reference to the CephCluster CR removed**, now
   we allow the cluster CR to be deleted. Remove the finalizer by editing the CephCluster CR.

```console
kubectl -n rook-ceph edit cephcluster
```

For example, remove the following from the CR metadata:

```yaml
    finalizers:
    - cephcluster.ceph.rook.io
```

After the finalizer is removed, the CR will be immediately deleted. If all owner references were properly removed,
all ceph daemons will continue running and there will be no downtime.

5. Create the CephCluster CR with the same settings as previously

```console
# Use the same cluster settings as exported above in step 2.
kubectl create -f cluster.yaml
```

6. If there are other CRs in terminating state such as CephBlockPools, CephObjectStores, or CephFilesystems,
   follow the above steps as well for those CRs:
   - Backup the CR
   - Remove the finalizer and confirm the CR is deleted (the underlying Ceph resources will be preserved)
   - Create the CR again

7. Scale up the operator

```console
kubectl -n rook-ceph scale --replicas=1 deploy/rook-ceph-operator
```

Watch the operator log to confirm that the reconcile completes successfully.

## Adopt an existing Rook Ceph cluster into a new Kubernetes cluster

Situations this section can help resolve:

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

    ```console
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

        ```console
        touch /etc/ceph/ceph.conf
        cd /var/lib/rook
        ceph-mon --extract-monmap monmap --mon-data ./mon-a/data  # Extract monmap from old ceph-mon db and save as monmap
        monmaptool --print monmap  # Print the monmap content, which reflects the old cluster ceph-mon configuration.
        monmaptool --rm a monmap  # Delete `a` from monmap.
        monmaptool --rm b monmap  # Repeat, and delete `b` from monmap.
        monmaptool --rm c monmap  # Repeat this pattern until all the old ceph-mons are removed
        monmaptool --rm d monmap
        monmaptool --rm e monmap
        monmaptool --addv a [v2:10.77.2.216:3300,v1:10.77.2.216:6789] monmap   # Replace it with the rook-ceph-mon-a address you got from previous command.
        ceph-mon --inject-monmap monmap --mon-data ./mon-a/data  # Replace monmap in ceph-mon db with our modified version.
        rm monmap
        exit
        ```

1. Tell Rook to run as old cluster by running `kubectl -n rook-ceph edit secret/rook-ceph-mon` and changing `fsid` to the original `fsid`. Note that the `fsid` is base64 encoded and must not contain a trailing carriage return. For example:

    ```console
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
    vi key
    # [paste keyring content saved before, preserving only `[client admin]` section]
    ceph auth import -i key
    rm key
    ```

1. Re-enable authentication by running `kubectl -n rook-ceph edit cm/rook-config-override` and removing auth configuration added in previous steps.
1. Stop the Rook operator by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `0`.
1. Shut down entire new cluster by running `kubectl -n rook-ceph delete deploy/X` where X is every deployment in namespace `rook-ceph`, except `rook-ceph-operator` and `rook-ceph-tools`, again. This time OSD daemons are present and should be removed too.
1. Bring the Rook Ceph operator back online by running `kubectl -n rook-ceph edit deploy/rook-ceph-operator` and set `replicas` to `1`.
1. Watch the operator logs with `kubectl -n rook-ceph logs -f rook-ceph-operator-xxxxxxx`, and wait until the orchestration has settled.
1. **STATE**: Now the new cluster should be up and running with authentication enabled. `ceph -s` output should not change much comparing to previous steps.

## Backing up and restoring a cluster based on PVCs into a new Kubernetes cluster

It is possible to migrate/restore an rook/ceph cluster from an existing Kubernetes cluster to a new one without resorting to SSH access or ceph tooling. This allows doing the migration using standard kubernetes resources only. This guide assumes the following:

1. You have a CephCluster that uses PVCs to persist mon and osd data. Let's call it the "old cluster"
1. You can restore the PVCs as-is in the new cluster. Usually this is done by taking regular snapshots of the PVC volumes and using a tool that can re-create PVCs from these snapshots in the underlying cloud provider. [Velero](https://github.com/vmware-tanzu/velero) is one such tool.
1. You have regular backups of the secrets and configmaps in the rook-ceph namespace. Velero provides this functionality too.

Do the following in the new cluster:

1. Stop the rook operator by scaling the deployment `rook-ceph-operator` down to zero: `kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas 0`
and deleting the other deployments. An example command to do this is `k -n rook-ceph delete deployment -l operator!=rook`
1. Restore the rook PVCs to the new cluster.
1. Copy the keyring and fsid secrets from the old cluster: `rook-ceph-mgr-a-keyring`, `rook-ceph-mon`, `rook-ceph-mons-keyring`, `rook-ceph-osd-0-keyring`, ...
1. Delete mon services and copy them from the old cluster: `rook-ceph-mon-a`, `rook-ceph-mon-b`, ... Note that simply re-applying won't work because the goal here is to restore the `clusterIP` in each service and this field is immutable in `Service` resources.
1. Copy the endpoints configmap from the old cluster: `rook-ceph-mon-endpoints`
1. Scale the rook operator up again : `kubectl -n rook-ceph scale deployment rook-ceph-operator --replicas 1`
1. Wait until the reconciliation is over.
