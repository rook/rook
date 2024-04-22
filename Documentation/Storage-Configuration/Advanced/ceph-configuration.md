---
title: Ceph Configuration
---

These examples show how to perform advanced configuration tasks on your Rook
storage cluster.

## Prerequisites

Most of the examples make use of the `ceph` client command.  A quick way to use
the Ceph client suite is from a [Rook Toolbox container](../../Troubleshooting/ceph-toolbox.md).

The Kubernetes based examples assume Rook OSD pods are in the `rook-ceph` namespace.
If you run them in a different namespace, modify `kubectl -n rook-ceph [...]` to fit
your situation.

## Using alternate namespaces

If you wish to deploy the Rook Operator and/or Ceph clusters to namespaces other than the default
`rook-ceph`, the manifests are commented to allow for easy `sed` replacements. Change
`ROOK_CLUSTER_NAMESPACE` to tailor the manifests for additional Ceph clusters. You can choose
to also change `ROOK_OPERATOR_NAMESPACE` to create a new Rook Operator for each Ceph cluster (don't
forget to set `ROOK_CURRENT_NAMESPACE_ONLY`), or you can leave it at the same value for every
Ceph cluster if you only wish to have one Operator manage all Ceph clusters.

If the operator namespace is different from the cluster namespace, the operator namespace must be
created before running the steps below. The cluster namespace does not need to be created first,
as it will be created by `common.yaml` in the script below.

```console
kubectl create namespace $ROOK_OPERATOR_NAMESPACE
```

This will help you manage namespaces more easily, but you should still make sure the resources are
configured to your liking.

```console
cd deploy/examples

export ROOK_OPERATOR_NAMESPACE="rook-ceph"
export ROOK_CLUSTER_NAMESPACE="rook-ceph"

sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:operator/\1:$ROOK_OPERATOR_NAMESPACE:\2 # serviceaccount:namespace:operator/g" \
    -e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:cluster/\1:$ROOK_CLUSTER_NAMESPACE:\2 # serviceaccount:namespace:cluster/g" \
    -e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # driver:namespace:cluster/\1: $ROOK_CLUSTER_NAMESPACE.\2 # driver:namespace:cluster/g" \
  common.yaml operator.yaml cluster.yaml # add other files or change these as desired for your config

# You need to use `apply` for all Ceph clusters after the first if you have only one Operator
kubectl apply -f common.yaml -f operator.yaml -f cluster.yaml # add other files as desired for yourconfig
```

Also see the CSI driver
[documentation](../Ceph-CSI/ceph-csi-drivers.md#Configure-CSI-Drivers-in-non-default-namespace)
to update the csi provisioner names in the storageclass and volumesnapshotclass.

## Deploying a second cluster

If you wish to create a new CephCluster in a separate namespace, you can easily do so
by modifying the `ROOK_OPERATOR_NAMESPACE` and `SECOND_ROOK_CLUSTER_NAMESPACE` values in the
below instructions. The default configuration in `common-second-cluster.yaml` is already
set up to utilize `rook-ceph` for the operator and `rook-ceph-secondary` for the cluster.
There's no need to run the `sed` command if you prefer to use these default values.

```console
cd deploy/examples
export ROOK_OPERATOR_NAMESPACE="rook-ceph"
export SECOND_ROOK_CLUSTER_NAMESPACE="rook-ceph-secondary"

sed -i.bak \
    -e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NAMESPACE # namespace:operator/g" \
    -e "s/\(.*\):.*# namespace:cluster/\1: $SECOND_ROOK_CLUSTER_NAMESPACE # namespace:cluster/g" \
  common-second-cluster.yaml

kubectl create -f common-second-cluster.yaml
```

This will create all the necessary RBACs as well as the new namespace. The script assumes that `common.yaml` was already created.
When you create the second CephCluster CR, use the same `NAMESPACE` and the operator will configure the second cluster.

## Log Collection

All Rook logs can be collected in a Kubernetes environment with the following command:

```console
for p in $(kubectl -n rook-ceph get pods -o jsonpath='{.items[*].metadata.name}')
do
    for c in $(kubectl -n rook-ceph get pod ${p} -o jsonpath='{.spec.containers[*].name}')
    do
        echo "BEGIN logs from pod: ${p} ${c}"
        kubectl -n rook-ceph logs -c ${c} ${p}
        echo "END logs from pod: ${p} ${c}"
    done
done
```

This gets the logs for every container in every Rook pod and then compresses them into a `.gz` archive
for easy sharing.  Note that instead of `gzip`, you could instead pipe to `less` or to a single text file.

## OSD Information

Keeping track of OSDs and their underlying storage devices can be
difficult. The following scripts will clear things up quickly.

### Kubernetes

```console
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
  for i in /var/lib/ceph/osd/ceph-*; do
    [ -f ${i}/ready ] || continue
    echo -ne "-$(basename ${i}) "
    echo $(lsblk -n -o NAME,SIZE ${i}/block 2> /dev/null || \
    findmnt -n -v -o SOURCE,SIZE -T ${i}) $(cat ${i}/type)
  done | sort -V
  echo'
done
```

The output should look something like this.

```console
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

!!! attention
    It is **deprecated to manually need to set this**, the `deviceClass` property can be used on Pool structures in `CephBlockPool`, `CephFilesystem` and `CephObjectStore` CRD objects.

By default Rook/Ceph puts all storage under one replication rule in the CRUSH
Map which provides the maximum amount of storage capacity for a cluster.  If you
would like to use different storage endpoints for different purposes, you'll
have to create separate storage groups.

In the following example we will separate SSD drives from spindle-based drives,
a common practice for those looking to target certain workloads onto faster
(database) or slower (file archive) storage.

## Configuring Pools

### Placement Group Sizing

!!! note
    Since Ceph Nautilus (v14.x), you can use the Ceph MGR `pg_autoscaler`
    module to auto scale the PGs as needed. It is highly advisable to configure
    default pg_num value on per-pool basis, If you want to enable this feature,
    please refer to [Default PG and PGP
    counts](configuration.md#default-pg-and-pgp-counts).

The general rules for deciding how many PGs your pool(s) should contain is:

* Fewer than 5 OSDs set `pg_num` to 128
* Between 5 and 10 OSDs set `pg_num` to 512
* Between 10 and 50 OSDs set `pg_num` to 1024

If you have more than 50 OSDs, you need to understand the tradeoffs and how to
calculate the pg_num value by yourself. For calculating pg_num yourself please
make use of [the pgcalc tool](https://old.ceph.com/pgcalc/).

### Setting PG Count

Be sure to read the [placement group sizing](#placement-group-sizing) section
before changing the number of PGs.

```console
# Set the number of PGs in the rbd pool to 512
ceph osd pool set rbd pg_num 512
```

## Custom `ceph.conf` Settings

!!! warning
    The advised method for controlling Ceph configuration is to use the [`cephConfig:` structure](../../CRDs/Cluster/ceph-cluster-crd.md#ceph-config)
    in the `CephCluster` CRD.
    <br><br>It is highly recommended that this only be used when absolutely necessary and that the `config` be
    reset to an empty string if/when the configurations are no longer necessary. Configurations in the
    config file will make the Ceph cluster less configurable from the CLI and dashboard and may make
    future tuning or debugging difficult.

Setting configs via Ceph's CLI requires that at least one mon be available for the configs to be
set, and setting configs via dashboard requires at least one mgr to be available. Ceph also has
a number of very advanced settings that cannot be modified easily via the CLI or
dashboard. In order to set configurations before monitors are available or to set advanced
configuration settings, the `rook-config-override` ConfigMap exists, and the `config` field can be
set with the contents of a `ceph.conf` file. The contents will be propagated to all mon, mgr, OSD,
MDS, and RGW daemons as an `/etc/ceph/ceph.conf` file.

!!! warning
    Rook performs no validation on the config, so the  validity of the settings is the
    user's responsibility.

If the `rook-config-override` ConfigMap is created before the cluster is started, the Ceph daemons
will automatically pick up the settings. If you add the settings to the ConfigMap after the cluster
has been initialized, each daemon will need to be restarted where you want the settings applied:

* mons: ensure all three mons are online and healthy before restarting each mon pod, one at a time.
* mgrs: the pods are stateless and can be restarted as needed, but note that this will disrupt the
    Ceph dashboard during restart.
* OSDs: restart your the pods by deleting them, one at a time, and running `ceph -s`
between each restart to ensure the cluster goes back to "active/clean" state.
* RGW: the pods are stateless and can be restarted as needed.
* MDS: the pods are stateless and can be restarted as needed.

After the pod restart, the new settings should be in effect. Note that if the ConfigMap in the Ceph
cluster's namespace is created before the cluster is created, the daemons will pick up the settings
at first launch.

To automate the restart of the Ceph daemon pods, you will need to trigger an update to the pod specs.
The simplest way to trigger the update is to add [annotations or labels](../../CRDs/Cluster/ceph-cluster-crd.md#annotations-and-labels)
to the CephCluster CR for the daemons you want to restart. The operator will then proceed with a rolling
update, similar to any other update to the cluster.

### Example

In this example we will set the default pool `size` to two, and tell OSD
daemons not to change the weight of OSDs on startup.

!!! warning
    Modify Ceph settings carefully. You are leaving the sandbox tested by Rook.
    Changing the settings could result in unhealthy daemons or even data loss if used incorrectly.

When the Rook Operator creates a cluster, a placeholder ConfigMap is created that
will allow you to override Ceph configuration settings. When the daemon pods are started, the
settings specified in this ConfigMap will be merged with the default settings
generated by Rook.

The default override settings are blank. Cutting out the extraneous properties,
we would see the following defaults after creating a cluster:

```console
kubectl -n rook-ceph get ConfigMap rook-config-override -o yaml
```

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-config-override
  namespace: rook-ceph
data:
  config: ""
```

To apply your desired configuration, you will need to update this ConfigMap. The next time the
daemon pod(s) start, they will use the updated configs.

```console
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

## Custom CSI `ceph.conf` Settings

!!! warning
    It is highly recommended to use the default setting that comes with
    CephCSI and this can only be used when absolutely necessary.
    The `ceph.conf` should be reset back to default values if/when the configurations are no
    longer necessary.

If the `csi-ceph-conf-override` ConfigMap is created before the cluster is
started, the CephCSI pods will automatically pick up the settings. If you
add the settings to the ConfigMap after the cluster has been initialized,
you can restart the Rook operator pod and wait for Rook to recreate CSI pods
to take immediate effect.

After the CSI pods are restarted, the new settings should be in effect.

### Example CSI `ceph.conf` Settings

In this [Example](https://github.com/rook/rook/tree/master/deploy/examples/csi-ceph-conf-override.yaml) we
will set the `rbd_validate_pool` to `false` to skip rbd pool validation.

!!! warning
    Modify Ceph settings carefully to avoid modifying the default
    configuration.
    Changing the settings could result in unexpected results if used incorrectly.

```console
kubectl create -f csi-ceph-conf-override.yaml
```

Restart the Rook operator pod and wait for CSI pods to be recreated.

## OSD CRUSH Settings

A useful view of the [CRUSH Map](http://docs.ceph.com/docs/master/rados/operations/crush-map/)
is generated with the following command:

```console
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

* Your cluster has some relatively slow OSDs or nodes. Lowering their weight can
    reduce the impact of this bottleneck.
* You're using bluestore drives provisioned with Rook v0.3.1 or older.  In this
    case you may notice OSD weights did not get set relative to their storage
    capacity.  Changing the weight can fix this and maximize cluster capacity.

This example sets the weight of osd.0 which is 600GiB

```console
ceph osd crush reweight osd.0 .600
```

### OSD Primary Affinity

When pools are set with a size setting greater than one, data is replicated
between nodes and OSDs.  For every chunk of data a Primary OSD is selected to be
used for reading that data to be sent to clients.  You can control how likely it
is for an OSD to become a Primary using the Primary Affinity setting.  This is
similar to the OSD weight setting, except it only affects reads on the storage
device, not capacity or writes.

In this example we will ensure that `osd.0` is only selected as Primary if all
other OSDs holding data replicas are unavailable:

```console
ceph osd primary-affinity osd.0 0
```

## OSD Dedicated Network

!!! tip
    This documentation is left for historical purposes. It is still valid, but Rook offers native
    support for this feature via the
    [CephCluster network configuration](../../CRDs/Cluster/ceph-cluster-crd.md#ceph-public-and-cluster-networks).

It is possible to configure ceph to leverage a dedicated network for the OSDs to
communicate across. A useful overview is the [Ceph Networks](http://docs.ceph.com/docs/master/rados/configuration/network-config-ref/#ceph-networks)
section of the Ceph documentation. If you declare a cluster network, OSDs will
route heartbeat, object replication, and recovery traffic over the cluster
network. This may improve performance compared to using a single network,
especially when slower network technologies are used. The tradeoff is
additional expense and subtle failure modes.

Two changes are necessary to the configuration to enable this capability:

### Use hostNetwork in the cluster configuration

Enable the `hostNetwork` setting in the [Ceph Cluster CRD configuration](../../CRDs/Cluster/ceph-cluster-crd.md#samples).
For example,

```yaml
  network:
    provider: host
```

!!! important
    Changing this setting is not supported in a running Rook cluster. Host networking
    should be configured when the cluster is first created.

### Define the subnets to use for public and private OSD networks

Edit the `rook-config-override` configmap to define the custom network
configuration:

```console
kubectl -n rook-ceph edit configmap rook-config-override
```

In the editor, add a custom configuration to instruct ceph which subnet is the
public network and which subnet is the private network. For example:

```yaml
apiVersion: v1
data:
  config: |
    [global]
    public network = 10.0.7.0/24
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
To check for "Phantom OSDs", you can run (example output):

```console
$ ceph osd tree
ID  CLASS WEIGHT  TYPE NAME STATUS REWEIGHT PRI-AFF
-1       57.38062 root default
-13        7.17258     host node1.example.com
2   hdd  3.61859         osd.2                up  1.00000 1.00000
-7              0     host node2.example.com   down    0    1.00000
```

The host `node2.example.com` in the output has no disks, so it is most likely a "Phantom OSD".

Now to remove it, use the ID in the first column of the output and replace `<ID>` with it. In the example output above the ID would be `-7`.
The commands are:

```console
ceph osd out <ID>
ceph osd crush remove osd.<ID>
ceph auth del osd.<ID>
ceph osd rm <ID>
```

To recheck that the Phantom OSD was removed, re-run the following command and check if the OSD with the ID doesn't show up anymore:

```console
ceph osd tree
```

## Auto Expansion of OSDs

### Prerequisites for Auto Expansion of OSDs

1) A [PVC-based cluster](../../CRDs/Cluster/ceph-cluster-crd.md#pvc-based-cluster) deployed in dynamic provisioning environment with a `storageClassDeviceSet`.

2) Create the Rook [Toolbox](../../Troubleshooting/ceph-toolbox.md).

!!! note
    [Prometheus Operator](../Monitoring/ceph-monitoring.md#prometheus-operator) and [Prometheus ../Monitoring/ceph-monitoring.mdnitoring.md#prometheus-instances) are Prerequisites that are created by the auto-grow-storage script.

### To scale OSDs Vertically

Run the following script to auto-grow the size of OSDs on a PVC-based Rook cluster whenever the OSDs have reached the storage near-full threshold.

```console
tests/scripts/auto-grow-storage.sh size  --max maxSize --growth-rate percent
```

`growth-rate` percentage represents the percent increase you want in the OSD capacity and maxSize represent the maximum disk size.

For example, if you need to increase the size of OSD by 30% and max disk size is 1Ti

```console
./auto-grow-storage.sh size  --max 1Ti --growth-rate 30
```

### To scale OSDs Horizontally

Run the following script to auto-grow the number of OSDs on a PVC-based Rook cluster whenever the OSDs have reached the storage near-full threshold.

```console
tests/scripts/auto-grow-storage.sh count --max maxCount --count rate
```

Count of OSD represents the number of OSDs you need to add and maxCount represents the number of disks a storage cluster will support.

For example, if you need to increase the number of OSDs by 3 and maxCount is 10

```console
./auto-grow-storage.sh count --max 10 --count 3
```
