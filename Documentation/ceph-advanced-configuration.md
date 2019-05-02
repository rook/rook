---
title: Advanced Configuration
weight: 11300
indent: true
---

# Advanced Cluster Configuration

These examples show how to perform advanced configuration tasks on your Rook
storage cluster.

- [Use custom Ceph user and secret for mounting](#use-custom-ceph-user-and-secret-for-mounting)
- [Log Collection](#log-collection)
- [OSD Information](#osd-information)
- [Separate Storage Groups](#separate-storage-groups)
- [Configuring Pools](#configuring-pools)
- [Custom ceph.conf Settings](#custom-cephconf-settings)
- [OSD CRUSH Settings](#osd-crush-settings)
- [OSD Dedicated Network](#osd-dedicated-network)
- [Phantom OSD Removal](#phantom-osd-removal)
- [Change Failure Domain](#change-failure-domain)

## Prerequisites

Most of the examples make use of the `ceph` client command.  A quick way to use
the Ceph client suite is from a [Rook Toolbox container](ceph-toolbox.md).

The Kubernetes based examples assume Rook OSD pods are in the `rook-ceph` namespace.
If you run them in a different namespace, modify `kubectl -n rook-ceph [...]` to fit
your situation.

## Use custom Ceph user and secret for mounting

**NOTE** For extensive info about creating Ceph users, consult the Ceph documentation: http://docs.ceph.com/docs/mimic/rados/operations/user-management/#add-a-user.
Using a custom Ceph user and secret can be done for filesystem and block storage.

Create a custom user in Ceph with read-write access in the `/bar` directory on CephFS (For Ceph Mimic or newer, use `data=POOL_NAME` instead of `pool=POOL_NAME`):

```
ceph auth get-or-create-key client.user1 mon 'allow r' osd 'allow rw tag cephfs pool=YOUR_FS_DATA_POOL' mds 'allow r, allow rw path=/bar'
```

The command will return a Ceph secret key, this key should be added as a secret in Kubernetes like this:

```
kubectl create secret generic ceph-user1-secret --from-literal=key=YOUR_CEPH_KEY
```

**NOTE** This secret with the same name must be created in each namespace where the StorageClass will be used.
In addition to this Secret you must create a RoleBinding to allow the Rook Ceph agent to get the secret from each namespace.
The RoleBinding is optional if you are using a ClusterRoleBinding for the Rook Ceph agent secret access.
A ClusterRole which contains the permissions which are needed and used for the Bindings are shown as an example after the next step.

On a StorageClass `parameters` and/or flexvolume Volume entry `options` set the following options:

```
mountUser: user1
mountSecret: ceph-user1-secret
```

If you want the Rook Ceph agent to require a `mountUser` and `mountSecret` to be set in StorageClasses using Rook, you must set the environment variable `AGENT_MOUNT_SECURITY_MODE` to `Restricted` on the Rook Ceph operator Deployment.

For more information on using the Ceph feature to limit access to CephFS paths, see [Ceph Documentation - Path Restriction](http://docs.ceph.com/docs/mimic/cephfs/client-auth/#path-restriction).

### ClusterRole

**NOTE**: When you are using the Helm chart to install the Rook Ceph operator and have set `mountSecurityMode` to e.g., `Restricted`,  then the below ClusterRole has already been created for you.

**This ClusterRole is needed no matter if you want to use a RoleBinding per namespace or a ClusterRoleBinding.**

```yaml
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: rook-ceph-agent-mount
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
```

### RoleBinding

**NOTE**: You either need a RoleBinding in each namespace in which a mount secret resides in or create a ClusterRoleBinding with which the Rook Ceph agent
has access to Kubernetes secrets in all namespaces.

Create the RoleBinding shown here in each namespace the Rook Ceph agent should read secrets for mounting.
The RoleBinding `subjects`' `namespace` must be the one the Rook Ceph agent runs in (default `rook-ceph` for version 1.0 and newer. The default namespace in
previous versions was `rook-ceph-system`).

Replace `namespace: name-of-namespace-with-mountsecret` according to the name of all namespaces a `mountSecret` can be in.
```yaml
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-agent-mount
  namespace: name-of-namespace-with-mountsecret
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-agent-mount
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph
```

### ClusterRoleBinding

This ClusterRoleBinding only needs to be created once, as it covers the whole cluster.

```yaml
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: rook-ceph-agent-mount
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-agent-mount
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph
```

## Log Collection

All Rook logs can be collected in a Kubernetes environment with the following command:
```bash
(for p in $(kubectl -n rook-ceph get pods -o jsonpath='{.items[*].metadata.name}')
do
    for c in $(kubectl -n rook-ceph get pod ${p} -o jsonpath='{.spec.containers[*].name}')
    do
        echo "BEGIN logs from pod: ${p} ${c}"
        kubectl -n rook-ceph logs -c ${c} ${p}
        echo "END logs from pod: ${p} ${c}"
    done
done
for i in $(kubectl -n rook-ceph-system get pods -o jsonpath='{.items[*].metadata.name}')
do
    echo "BEGIN logs from pod: ${i}"
    kubectl -n rook-ceph-system logs ${i}
    echo "END logs from pod: ${i}"
done) | gzip > /tmp/rook-logs.gz
```
This gets the logs for every container in every Rook pod and then compresses them into a `.gz` archive
for easy sharing.  Note that instead of `gzip`, you could instead pipe to `less` or to a single text file.

## OSD Information

Keeping track of OSDs and their underlying storage devices/directories can be
difficult.  The following scripts will clear things up quickly.

### Kubernetes

```bash
# Get OSD Pods
# This uses the example/default cluster name "rook"
OSD_PODS=$(kubectl get pods --all-namespaces -l \
  app=rook-ceph-osd,rook_cluster=rook-ceph -o jsonpath='{.items[*].metadata.name}')

# Find node and drive associations from OSD pods
for pod in $(echo ${OSD_PODS})
do
 echo "Pod:  ${pod}"
 echo "Node: $(kubectl -n rook-ceph get pod ${pod} -o jsonpath='{.spec.nodeName}')"
 kubectl -n rook-ceph exec ${pod} -- sh -c '\
  for i in /var/lib/rook/osd*; do
    [ -f ${i}/ready ] || continue
    echo -ne "-$(basename ${i}) "
    echo $(lsblk -n -o NAME,SIZE ${i}/block 2> /dev/null || \
    findmnt -n -v -o SOURCE,SIZE -T ${i}) $(cat ${i}/type)
  done|sort -V
  echo'
done
```

The output should look something like this. Note that OSDs on the same node will show duplicate information.

```bash
Pod:  osd-m2fz2
Node: node1.zbrbdl
-osd0  sda3  557.3G  bluestore
-osd1  sdf3  110.2G  bluestore
-osd2  sdd3  277.8G  bluestore
-osd3  sdb3  557.3G  bluestore
-osd4  sde3  464.2G  bluestore
-osd5  sdc3  557.3G  bluestore

Pod:  osd-nxxnq
Node: node3.zbrbdl
-osd6   sda3  110.7G  bluestore
-osd17  sdd3  1.8T    bluestore
-osd18  sdb3  231.8G  bluestore
-osd19  sdc3  231.8G  bluestore

Pod:  osd-tww1h
Node: node2.zbrbdl
-osd7   sdc3  464.2G  bluestore
-osd8   sdj3  557.3G  bluestore
-osd9   sdf3  66.7G   bluestore
-osd10  sdd3  464.2G  bluestore
-osd11  sdb3  147.4G  bluestore
-osd12  sdi3  557.3G  bluestore
-osd13  sdk3  557.3G  bluestore
-osd14  sde3  66.7G   bluestore
-osd15  sda3  110.2G  bluestore
-osd16  sdh3  135.1G  bluestore
```

## Separate Storage Groups

By default Rook/Ceph puts all storage under one replication rule in the CRUSH
Map which provides the maximum amount of storage capacity for a cluster.  If you
would like to use different storage endpoints for different purposes, you'll
have to create separate storage groups.

In the following example we will separate SSD drives from spindle-based drives,
a common practice for those looking to target certain workloads onto faster
(database) or slower (file archive) storage.

### CRUSH Hierarchy

To see the CRUSH hierarchy of all your hosts and OSDs run:
```bash
ceph osd tree
```

Before we separate our disks into groups, our example cluster looks like this:
```bash
ID WEIGHT  TYPE NAME          UP/DOWN REWEIGHT PRIMARY-AFFINITY
-1 7.21828 root default
-2 0.94529     host node1
 0 0.55730         osd.0           up  1.00000          1.00000
 1 0.11020         osd.1           up  1.00000          1.00000
 2 0.27779         osd.2           up  1.00000          1.00000
-3 1.22480     host node2
 3 0.55730         osd.3           up  1.00000          1.00000
 4 0.11020         osd.4           up  1.00000          1.00000
 5 0.55730         osd.5           up  1.00000          1.00000
-4 1.22480     host node3
 6 0.55730         osd.6           up  1.00000          1.00000
 7 0.11020         osd.7           up  1.00000          1.00000
 8 0.06670         osd.8           up  1.00000          1.00000
```

We have one root bucket `default` that every host and OSD is under, so all of
these storage locations get pooled together for reads/writes/replication.

Let's say that `osd.1`, `osd.3`, and `osd.7` are our small SSD drives that we
want to use separately.

First we will create a new `root` bucket called `ssd` in our CRUSH map.  Under
this new bucket we will add new `host` buckets for each node that contains an
SSD drive so data can be replicated and used separately from the default HDD
group.

```bash
# Create a new tree in the CRUSH Map for SSD hosts and OSDs
ceph osd crush add-bucket ssd root
ceph osd crush add-bucket node1-ssd host
ceph osd crush add-bucket node2-ssd host
ceph osd crush add-bucket node3-ssd host
ceph osd crush move node1-ssd root=ssd
ceph osd crush move node2-ssd root=ssd
ceph osd crush move node3-ssd root=ssd

# Create a new rule for replication using the new tree
ceph osd crush rule create-simple ssd ssd host firstn
```

Secondly we will move the SSD OSDs into the new `ssd` tree, under their
respective `host` buckets:

```bash
ceph osd crush set osd.1 .1102 root=ssd host=node1-ssd
ceph osd crush set osd.3 .1102 root=ssd host=node2-ssd
ceph osd crush set osd.7 .1102 root=ssd host=node3-ssd
```

It's important to note that the `ceph osd crush set` command requires a weight
to be specified (our example uses `.1102`).  If you'd like to change their
weight you can do that here, otherwise be sure to specify their original weight
seen in the `ceph osd tree` output.

So let's look at our CRUSH tree again with these changes:

```bash
ID WEIGHT  TYPE NAME          UP/DOWN REWEIGHT PRIMARY-AFFINITY
-8 0.22040 root ssd
-5 0.11020     host node1-ssd
 1 0.11020         osd.1           up  1.00000          1.00000
-6 0.11020     host node2-ssd
 4 0.11020         osd.4           up  1.00000          1.00000
-7 0.11020     host node3-ssd
 7 0.11020         osd.7           up  1.00000          1.00000
-1 7.21828 root default
-2 0.83509     host node1
 0 0.55730         osd.0           up  1.00000          1.00000
 2 0.27779         osd.2           up  1.00000          1.00000
-3 1.11460     host node2
 3 0.55730         osd.3           up  1.00000          1.00000
 5 0.55730         osd.5           up  1.00000          1.00000
-4 1.11460     host node3
 6 0.55730         osd.6           up  1.00000          1.00000
 8 0.55730         osd.8           up  1.00000          1.00000
```

### Using Disk Groups With Pools

Now we have a separate storage group for our SSDs, but we can't use that storage
until we associate a pool with it.  The default group already has a pool called
`rbd` in many cases.  If you [created a pool via CustomResourceDefinition](ceph-pool-crd.md),
it will use the default storage group as well.

Here's how to create new pools:

```bash
# SSD backed pool with 128 (total) PGs
ceph osd pool create ssd 128 128 replicated ssd
```

Now all you need to do is create RBD images or Kubernetes `StorageClass`es that
specify the `ssd` pool to put it to use.

## Configuring Pools

### Placement Group Sizing

The general rules for deciding how many PGs your pool(s) should contain is:
- Less than 5 OSDs set pg_num to 128
- Between 5 and 10 OSDs set pg_num to 512
- Between 10 and 50 OSDs set pg_num to 1024

If you have more than 50 OSDs, you need to understand the tradeoffs and how to
calculate the pg_num value by yourself. For calculating pg_num yourself please
make use of [the pgcalc tool](http://ceph.com/pgcalc/)

If you're already using a pool it is generally safe to [increase its PG
count](#setting-pg-count) on-the-fly.  Decreasing the PG count is not
recommended on a pool that is in use.  The safest way to decrease the PG count
is to back-up the data, [delete the pool](#deleting-a-pool), and [recreate
it](#creating-a-pool).  With backups you can try a few potentially unsafe
tricks for live pools, documented
[here](http://cephnotes.ksperis.com/blog/2015/04/15/ceph-pool-migration).

### Deleting A Pool

Be warned that this deletes all data from the pool, so Ceph by default makes it
somewhat difficult to do.

First you must inject arguments to the Mon daemons to tell them to allow the
deletion of pools.  In Rook Tools you can do this:

```bash
ceph tell mon.\* injectargs '--mon-allow-pool-delete=true'
```

Then to delete a pool, `rbd` in this example, run:
```bash
ceph osd pool rm rbd rbd --yes-i-really-really-mean-it
```

### Creating A Pool

```bash
# Create a pool called rbd with 1024 total PGs, using the default
# replication ruleset
ceph osd pool create rbd 1024 1024 replicated replicated_ruleset
```

`replicated_ruleset` is the default CRUSH rule that replicates between the hosts
and OSDs in the `default` root hierarchy.

### Setting The Number Of Replicas

The `size` setting of a pool tells the cluster how many copies of the data
should be kept for redundancy.  By default the cluster will distribute these
copies between `host` buckets in the CRUSH Map This can be set when [creating a
pool via CustomResourceDefinition](ceph-pool-crd.md) or after creation with `ceph`.

So for example let's change the `size` of the `rbd` pool to three:

```
ceph osd pool set rbd size 3
```

Now if you run `ceph -s` you may see "recovery" operations and
PGs in "undersized" and other "unclean" states.  The cluster is essentially
fixing itself since the number of replicas has been increased, and should go
back to "active/clean" state shortly, after data has been replicated between
hosts.  When that's done you will be able to lose two of your storage nodes and
still have access to all your data in that pool, since the CRUSH algorithm will
guarantee that at least one replica will still be available on another storage node.
Of course you will only have 1/3 the capacity as a tradeoff.

### Setting PG Count

Be sure to read the [placement group sizing](#placement-group-sizing) section
before changing the number of PGs.

```bash
# Set the number of PGs in the rbd pool to 512
ceph osd pool set rbd pg_num 512
```

## Custom ceph.conf Settings

With Rook the full swath of
[Ceph settings](http://docs.ceph.com/docs/kraken/rados/configuration/) are available
to use on your storage cluster.  When we supply Rook with a ceph.conf file those
settings will be propagated to all Mon, OSD, MDS, and RGW daemons to use.

In this example we will set the default pool `size` to two, and tell OSD
daemons not to change the weight of OSDs on startup.

**WARNING**: Modify Ceph settings carefully. You are leaving the sandbox tested by Rook.
Changing the settings could result in unhealthy daemons or even data loss if used incorrectly.

### Kubernetes
When the Rook Operator creates a cluster, a placeholder ConfigMap is created that
will allow you to override Ceph configuration settings. When the daemon pods are started, the
settings specified in this ConfigMap will be merged with the default settings
generated by Rook.

The default override settings are blank. Cutting out the extraneous properties,
we would see the following defaults after creating a cluster:

```bash
$ kubectl -n rook-ceph get ConfigMap rook-config-override -o yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-config-override
  namespace: rook-ceph
data:
  config: ""
```

To apply your desired configuration, you will need to update this ConfigMap.
The next time the daemon pod(s) start, the settings will be merged with the default
settings created by Rook.

```bash
kubectl -n rook-ceph edit configmap rook-config-override
```

Modify the settings and save. Each line you add should be indented from the `config` property as such:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-config-override
  namespace: rook-ceph
data:
  config: |
    [global]
    osd crush update on start = false
    osd pool default size = 2
```

Each daemon will need to be restarted where you want the settings applied:

- Mons: ensure all three mons are online and healthy before restarting each mon pod, one at a time
- OSDs: restart your the pods by deleting them, one at a time, and running `ceph -s`
between each restart to ensure the cluster goes back to "active/clean" state.
- RGW: the pods are stateless and can be restarted as needed
- MDS: the pods are stateless and can be restarted as needed

After the pod restart, your new settings should be in effect. Note that if you create
the ConfigMap in the `rook-ceph` namespace before the cluster is even created
the daemons will pick up the settings at first launch.

The only validation of the settings done by Rook is whether the settings can be merged
using the ini file format with the default settings created by Rook. Beyond that,
the validity of the settings is your responsibility.

## OSD CRUSH Settings

A useful view of the [CRUSH Map](http://docs.ceph.com/docs/kraken/rados/operations/crush-map/)
is generated with the following command:

```bash
ceph osd tree
```

In this section we will be tweaking some of the values seen in the output.

### OSD Weight

The CRUSH weight controls the ratio of data that should be distributed to each
OSD.  This also means a higher or lower amount of disk I/O operations for an OSD
with higher/lower weight, respectively.

By default OSDs get a weight relative to their storage capacity, which maximizes
overall cluster capacity by filling all drives at the same rate, even if drive
sizes vary.  This should work for most use-cases, but the following situations
could warrant weight changes:

- Your cluster has some relatively slow OSDs or nodes. Lowering their weight can
  reduce the impact of this bottleneck.
- You're using bluestore drives provisioned with Rook v0.3.1 or older.  In this
  case you may notice OSD weights did not get set relative to their storage
  capacity.  Changing the weight can fix this and maximize cluster capacity.

This example sets the weight of osd.0 which is 600GiB

```bash
ceph osd crush reweight osd.0 .600
```

### OSD Primary Affinity

When pools are set with a size setting greater than one, data is replicated
between nodes and OSDs.  For every chunk of data a Primary OSD is selected to be
used for reading that data to be sent to clients.  You can control how likely it
is for an OSD to become a Primary using the Primary Affinity setting.  This is
similar to the OSD weight setting, except it only affects reads on the storage
device, not capacity or writes.

In this example we will make sure `osd.0` is only selected as Primary if all
other OSDs holding replica data are unavailable:

```bash
ceph osd primary-affinity osd.0 0
```

## OSD Dedicated Network

It is possible to configure ceph to leverage a dedicated network for the OSDs to
communicate across. A useful overview is the [CEPH Networks](http://docs.ceph.com/docs/master/rados/configuration/network-config-ref/#ceph-networks)
section of the Ceph documentation. If you declare a cluster network, OSDs will
route heartbeat, object replication and recovery traffic over the cluster
network. This may improve performance compared to using a single network.

Two changes are necessary to the configuration to enable this capability:

### Use hostNetwork in the rook ceph cluster configuration

Enable the `hostNetwork` setting in the [Ceph Cluster CRD configuration](https://rook.io/docs/rook/v1.0/ceph-cluster-crd.html#samples).
For example,

```yaml
  network:
    hostNetwork: true
```

### Define the subnets to use for public and private OSD networks

Edit the `rook-config-override` configmap to define the custom network
configuration:

```bash
kubectl -n rook-ceph edit configmap rook-config-override
```

In the editor, add a custom configuration to instruct ceph which subnet is the
public network and which subnet is the private network. For example:

```yaml
apiVersion: v1
data:
  config: |
    [global]
    public network =  10.0.7.0/24
    cluster network = 10.0.10.0/24
    public addr = ""
    cluster addr = ""
```

After applying the updated rook-config-override configmap, it will be necessary
to restart the OSDs by deleting the OSD pods in order to apply the change.
Restart the OSD pods by deleting them, one at a time, and running ceph -s
between each restart to ensure the cluster goes back to "active/clean" state.

## Phantom OSD Removal

If you have OSDs in which are not showing any disks, you can remove those "Phantom OSDs" by following the instructions below.
To check for "Phantom OSDs", you can run:
```bash
ceph osd tree
```
An example output looks like this:
```
ID  CLASS WEIGHT  TYPE NAME STATUS REWEIGHT PRI-AFF
 -1       57.38062 root default
-13        7.17258     host node1.example.com
  2   hdd  3.61859         osd.2                up  1.00000 1.00000
 -7              0     host node2.example.com   down    0    1.00000
```
The host `node2.example.com` in the output has no disks, so it is most likely a "Phantom OSD".

Now to remove it, use the ID in the first column of the output and replace `<ID>` with it. In the example output above the ID would be `-7`.
The commands are:
```bash
ceph osd out <ID>
ceph osd crush remove osd.<ID>
ceph auth del osd.<ID>
ceph osd rm <ID>
```

To recheck that the Phantom OSD got removed, re-run the following command and check if the OSD with the ID doesn't show up anymore:
```bash
ceph osd tree
```

## Change Failure Domain
In Rook, it is now possible to indicate how the default CRUSH failure domain rule must be configured in order to ensure that replicas or erasure code shards are separated across hosts, and a single host failure does not affect availability. For instance, this is an example manifest of a block pool named `replicapool` configured with a `failureDomain` set to `osd`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook
spec:
  # The failure domain will spread the replicas of the data across different failure zones
  failureDomain: osd
  ...
```

However, due to several reasons, we may need to change such failure domain to its other value: `host`. Unfortunately, changing it directly in the YAML manifest is not currently handled by Rook, so we need to perform the change directly using Ceph commands using the Rook tools pod, for instance:

```bash
$ ceph osd pool get replicapool crush_rule
crush_rule: replicapool

$ceph osd crush rule create-replicated replicapool_host_rule default host
```

Notice that the suffix `host_rule` in the name of the rule is just for clearness about the type of rule we are creating here, and can be anything else as long as it is different from the existing one. Once the new rule has been created, we simply apply it to our block pool:

```bash
$ ceph osd pool set replicapool crush_rule replicapool_host_rule
```

And validate that it has been actually applied properly:

```bash
$ ceph osd pool get replicapool crush_rule
crush_rule: replicapool_host_rule
```

If the cluster's health was `HEALTH_OK` when we performed this change, immediately, the new rule is applied to the cluster transparently without service disruption.

Exactly the same approach can be used to change from `host` back to `osd`.
