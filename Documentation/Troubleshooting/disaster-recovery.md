---
title: Disaster Recovery
---

Under extenuating circumstances, steps may be necessary to recover the cluster health. There are several types of recovery addressed in this document.

## Restoring Mon Quorum

Under extenuating circumstances, the mons may lose quorum. If the mons cannot form quorum again,
there is a manual procedure to get the quorum going again. The only requirement is that at least one mon
is still healthy. The following steps will remove the unhealthy
mons from quorum and allow you to form a quorum again with a single mon, then grow the quorum back to the original size.

The [Rook Krew Plugin](https://github.com/rook/kubectl-rook-ceph/) has a command `restore-quorum` that will
walk you through the mon quorum automated restoration process.

If the name of the healthy mon is `c`, you would run the command:

```console
kubectl rook-ceph mons restore-quorum c
```

See the [restore-quorum documentation](https://github.com/rook/kubectl-rook-ceph/blob/master/docs/mons.md#restore-quorum)
for more details.

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

!!! note
    In the following commands, the affected `CephCluster` resource is called `rook-ceph`. If yours is named differently, the
    commands will need to be adjusted.

1.  Scale down the operator.

    ```console
    kubectl -n rook-ceph scale --replicas=0 deploy/rook-ceph-operator
    ```

2.  Backup all Rook CRs and critical metadata

    ```console
    # Store the `CephCluster` CR settings. Also, save other Rook CRs that are in terminating state.
    kubectl -n rook-ceph get cephcluster rook-ceph -o yaml > cluster.yaml

    # Backup critical secrets and configmaps in case something goes wrong later in the procedure
    kubectl -n rook-ceph get secret -o yaml > secrets.yaml
    kubectl -n rook-ceph get configmap -o yaml > configmaps.yaml
    ```

3.  (Optional, if webhook is enabled)
    Delete the `ValidatingWebhookConfiguration`. This is the resource which connects Rook custom resources
    to the operator pod's validating webhook. Because the operator is unavailable, we must temporarily disable
    the valdiating webhook in order to make changes.

        ```console
        kubectl delete ValidatingWebhookConfiguration rook-ceph-webhook
        ```

4.  Remove the owner references from all critical Rook resources that were referencing the `CephCluster` CR.

    1.  Programmatically determine all such resources, using this command:
        ```console
        # Determine the `CephCluster` UID
        ROOK_UID=$(kubectl -n rook-ceph get cephcluster rook-ceph -o 'jsonpath={.metadata.uid}')
        # List all secrets, configmaps, services, deployments, and PVCs with that ownership UID.
        RESOURCES=$(kubectl -n rook-ceph get secret,configmap,service,deployment,pvc -o jsonpath='{range .items[?(@.metadata.ownerReferences[*].uid=="'"$ROOK_UID"'")]}{.kind}{"/"}{.metadata.name}{"\n"}{end}')
        # Show the collected resources.
        kubectl -n rook-ceph get $RESOURCES
        ```

    2.  **Verify that all critical resources are shown in the output.** The critical resources are these:

          - Secrets: `rook-ceph-admin-keyring`, `rook-ceph-config`, `rook-ceph-mon`, `rook-ceph-mons-keyring`
          - ConfigMap: `rook-ceph-mon-endpoints`
          - Services: `rook-ceph-mon-*`, `rook-ceph-mgr-*`
          - Deployments: `rook-ceph-mon-*`, `rook-ceph-osd-*`, `rook-ceph-mgr-*`
          - PVCs (if applicable): `rook-ceph-mon-*` and the OSD PVCs (named `<deviceset>-*`, for example `set1-data-*`)

    3.  For each listed resource, remove the `ownerReferences` metadata field, in order to unlink it from the deleting `CephCluster`
        CR.

        To do so programmatically, use the command:
        ```console
        for resource in $(kubectl -n rook-ceph get $RESOURCES -o name); do
          kubectl -n rook-ceph patch $resource -p '{"metadata": {"ownerReferences":null}}'
        done
        ```

        For a manual alternative, issue `kubectl edit` on each resource, and remove the block matching:
        ```yaml
        ownerReferences:
        - apiVersion: ceph.rook.io/v1
           blockOwnerDeletion: true
           controller: true
           kind: `CephCluster`
           name: rook-ceph
           uid: <uid>
        ```

5.  **Before completing this step, validate these things. Failing to do so could result in data loss.**

    1.  Confirm that `cluster.yaml` contains the `CephCluster` CR.
    2.  Confirm all critical resources listed above have had the `ownerReference` to the `CephCluster` CR removed.


    Remove the finalizer from the `CephCluster` resource. This will cause the resource to be immediately deleted by Kubernetes.

    ```console
    kubectl -n rook-ceph patch cephcluster/rook-ceph --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
    ```

    After the finalizer is removed, the `CephCluster` will be immediately deleted. If all owner references were properly removed,
    all ceph daemons will continue running and there will be no downtime.

6.  Create the `CephCluster` CR with the same settings as previously

    ```console
    # Use the same cluster settings as exported in step 2.
    kubectl create -f cluster.yaml
    ```

7.  If there are other CRs in terminating state such as CephBlockPools, CephObjectStores, or CephFilesystems,
    follow the above steps as well for those CRs:

      1.  Backup the CR
      2.  Remove the finalizer and confirm the CR is deleted (the underlying Ceph resources will be preserved)
      3.  Create the CR again

8.  Scale up the operator

    ```console
    kubectl -n rook-ceph scale --replicas=1 deploy/rook-ceph-operator
    ```

9.  Watch the operator log to confirm that the reconcile completes successfully.

    ```console
    kubectl -n rook-ceph logs -f deployment/rook-ceph-operator
    ```


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

## Restoring the Rook cluster after the Rook namespace is deleted

When the rook-ceph namespace is accidentally deleted, the good news is that the cluster can be restored. With the content in the directory `dataDirHostPath` and the original OSD disks, the ceph cluster could be restored with this guide.

You need to manually create a ConfigMap and a Secret to make it work. The information required for the ConfigMap and Secret can be found in the `dataDirHostPath` directory.

The first resource is the secret named `rook-ceph-mon` as seen in this example below:

```yaml
apiVersion: v1
data:
  ceph-secret: QVFCZ0h6VmorcVNhSGhBQXVtVktNcjcrczNOWW9Oa2psYkErS0E9PQ==
  ceph-username: Y2xpZW50LmFkbWlu
  fsid: M2YyNzE4NDEtNjE4OC00N2MxLWIzZmQtOTBmZDRmOTc4Yzc2
  mon-secret: QVFCZ0h6VmorcVNhSGhBQXVtVktNcjcrczNOWW9Oa2psYkErS0E9PQ==
kind: Secret
metadata:
  finalizers:
  - ceph.rook.io/disaster-protection
  name: rook-ceph-mon
  namespace: rook-ceph
  ownerReferences: null
type: kubernetes.io/rook

```

The values for the secret can be found in `$dataDirHostPath/rook-ceph/client.admin.keyring` and `$dataDirHostPath/rook-ceph/rook-ceph.config`.
- `ceph-secret` and `mon-secret` are to be filled with the `client.admin`'s keyring contents.
- `ceph-username`: set to the string `client.admin`
- `fsid`: set to the original ceph cluster id.

All the fields in data section need to be encoded in base64. Coding could be done like this:
```console
echo -n "string to code" | base64 -i -
```
Now save the secret as `rook-ceph-mon.yaml`, to be created later in the restore.

The second resource is the configmap named rook-ceph-mon-endpoints as seen in this example below:

```yaml
apiVersion: v1
data:
  csi-cluster-config-json: '[{"clusterID":"rook-ceph","monitors":["169.169.241.153:6789","169.169.82.57:6789","169.169.7.81:6789"],"namespace":""}]'
  data: k=169.169.241.153:6789,m=169.169.82.57:6789,o=169.169.7.81:6789
  mapping: '{"node":{"k":{"Name":"10.138.55.111","Hostname":"10.138.55.111","Address":"10.138.55.111"},"m":{"Name":"10.138.55.120","Hostname":"10.138.55.120","Address":"10.138.55.120"},"o":{"Name":"10.138.55.112","Hostname":"10.138.55.112","Address":"10.138.55.112"}}}'
  maxMonId: "15"
kind: ConfigMap
metadata:
  finalizers:
  - ceph.rook.io/disaster-protection
  name: rook-ceph-mon-endpoints
  namespace: rook-ceph
  ownerReferences: null
```

The Monitor's service IPs are kept in the monitor data store and you need to create them by original ones. After you create this configmap with the original service IPs, the rook operator will create the correct services for you with IPs matching in the monitor data store. Along with monitor ids, their service IPs and mapping relationship of them can be found in dataDirHostPath/rook-ceph/rook-ceph.config, for example:

```console
[global]
fsid                = 3f271841-6188-47c1-b3fd-90fd4f978c76
mon initial members = m o k
mon host            = [v2:169.169.82.57:3300,v1:169.169.82.57:6789],[v2:169.169.7.81:3300,v1:169.169.7.81:6789],[v2:169.169.241.153:3300,v1:169.169.241.153:6789]
```

`mon initial members` and `mon host` are holding sequences of monitors' id and IP respectively; the sequence are going in the same order among monitors as a result you can tell which monitors have which service IP addresses. Modify your `rook-ceph-mon-endpoints.yaml` on fields `csi-cluster-config-json` and `data` based on the understanding of `rook-ceph.config` above.
The field `mapping` tells rook where to schedule monitor's pods. you could search in `dataDirHostPath` in all Ceph cluster hosts for `mon-m,mon-o,mon-k`. If you find `mon-m` in host `10.138.55.120`, you should fill `10.138.55.120` in field `mapping` for `m`. Others are the same.
Update the `maxMonId` to be the max numeric ID of the highest monitor ID. For example, 15 is the 0-based ID for mon `o`.
Now save this configmap in the file rook-ceph-mon-endpoints.yaml, to be created later in the restore.

Now that you have the info for the secret and the configmap, you are ready to restore the running cluster.

Deploy Rook Ceph using the YAML files or Helm, with the same settings you had previously.

```console
kubectl create -f crds.yaml -f common.yaml -f operator.yaml
```

After the operator is running, create the configmap and secret you have just crafted:

```console
kubectl create -f rook-ceph-mon.yaml -f rook-ceph-mon-endpoints.yaml
```

Create your Ceph cluster CR (if possible, with the same settings as existed previously):

```console
kubectl create -f cluster.yaml
```

Now your Rook Ceph cluster should be running again.
