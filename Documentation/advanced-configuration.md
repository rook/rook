# Advanced Cluster Configuration

These examples show how to perform advanced configuration tasks on your Rook
storage cluster.

- [OSD Information](#osd-information)
- [Separate Storage Groups](#separate-storage-groups)
- [Configuring Pools](#configuring-pools)
- [Custom ceph.conf Settings](#custom-cephconf-settings)
- [OSD CRUSH Settings](#osd-crush-settings)

## Prerequisites

Most of the examples make use of the `ceph` client command.  A quick way to use
the Ceph client suite is from a [Rook Toolbox container](toolbox.md).

The Kubernetes based examples assume Rook OSD pods are in the `rook` namespace.
If you run them in a different namespace, modify `kubectl -n rook [...]` to fit
your situation.

## OSD Information

Keeping track of OSDs and their underlying storage devices/directories can be
difficult.  The following scripts will clear things up quickly.

### Standalone

```bash
# Run this on each storage node

echo "Node:" $(hostname -s)
for i in /var/lib/rook/osd*; do
  [ -f ${i}/ready ] || continue
  echo -ne "-$(basename ${i}) "
  echo $(lsblk -n -o NAME,SIZE ${i}/block 2> /dev/null || \
  findmnt -n -v -o SOURCE,SIZE -T ${i}) $(cat ${i}/type)
done|sort -V|column -t
```

### Kubernetes

```bash
# Get OSD Pods
OSD_PODS=$(kubectl get pods --all-namespaces -l \
  app=osd,rook_cluster=cluster -o jsonpath='{.items[*].metadata.name}')

# Find node and drive associations from OSD pods
for pod in $(echo ${OSD_PODS})
do
 echo "Pod:  ${pod}"
 echo "Node: $(kubectl -n rook get pod ${pod} -o jsonpath='{.spec.nodeName}')"
 kubectl -n rook exec ${pod} -- sh -c '\
  for i in /var/lib/rook/osd*; do
    [ -f ${i}/ready ] || continue
    echo -ne "-$(basename ${i}) "
    echo $(lsblk -n -o NAME,SIZE ${i}/block 2> /dev/null || \
    findmnt -n -v -o SOURCE,SIZE -T ${i}) $(cat ${i}/type)
  done|sort -V|column -t
  echo'
done
```

The output should look something like this:
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

### CRUSH Heirarchy

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
until we associate a pool with it.
The default group already has a pool called `rbd` in many cases.  If you
[created a pool via ThirdPartyResource](pool-tpr.md), it will use the default
storage group as well.

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
pool via ThirdPartyResource](pool-tpr.md) or after creation with `ceph`.

So for example let's change the `size` of the `rbd` pool to three
```
ceph osd pool set rbd size 3
```

Now if you run `ceph -s` or `rookctl status` you may see "recovery" operations and
PGs in "undersized" and other "unclean" states.  The cluster is essentially
fixing itself since the number of replicas has been increased, and should go
back to "active/clean" state shortly, after data has been replicated between
hosts.  When that's done you will be able to lose 2/3 of your storage nodes and
still have access to all your data in that pool.  Of course you will only have
1/3 the capacity as a tradeoff.

### Setting PG Count

Be sure to read the [placement group sizing](#placement-group-sizing) section
before changing the numner of PGs.

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

### Standalone Rook

Here's our custom `ceph.conf` for the OSDs:
```bash
[global]
osd crush update on start = false
osd pool default size     = 2
```

With our ceph.conf file created we pass the settings to `rookd` with the
`--ceph-config-override` argument, for example:
```bash
rookd --ceph-config-override=ceph.conf --data-devices=sdf
```

### Kubernetes
When the Rook Operator creates a cluster, a placeholder ConfigMap is created that
will allow you to override Ceph configuration settings. When the daemon pods are started, the 
settings specified in this ConfigMap will be merged with the default settings
generated by Rook. 

The default override settings are blank. Cutting out the extraneous properties, 
we would see the following defaults after creating a cluster:
```bash
$ kubectl -n rook get ConfigMap rook-config-override -o yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-config-override
  namespace: rook
data:
  config: ""
```

To apply your desired configuration, you will need to update this ConfigMap.
The next time the daemon pod(s) start, the settings will be merged with the default
settings created by Rook.

```bash
kubectl -n rook edit configmap rook-config-override
```

Modify the settings and save. Each line you add should be indented from the `config` property as such:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-config-override
  namespace: rook
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
the ConfigMap in the `rook` namespace before the cluster is even created 
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
