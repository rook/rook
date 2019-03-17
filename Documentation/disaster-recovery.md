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
```bash
kubectl -n rook-ceph delete deployment rook-ceph-operator
```

### Inject a new monmap
**WARNING: Injecting a monmap must be done very carefully. If run incorrectly, your cluster could be permanently destroyed.**

The Ceph monmap keeps track of the mon quorum. We will update the monmap to only contain the healthy mon.
In this example, the healthy mon is `rook-ceph-mon-b`, while the unhealthy mons are `rook-ceph-mon-a` and `rook-ceph-mon-c`.

Connect to the pod of a healthy mon and run the following commands.
```bash
kubectl -n rook-ceph exec -it <mon-pod> bash

# set a few simple variables
cluster_namespace=rook
good_mon_id=rook-ceph-mon-b
monmap_path=/tmp/monmap

# make sure the quorum lock file does not exist
rm -f /var/lib/rook/${good_mon_id}/data/store.db/LOCK

# extract the monmap to a file
ceph-mon -i ${good_mon_id} --extract-monmap ${monmap_path} \
  --cluster=${cluster_namespace} --mon-data=/var/lib/rook/${good_mon_id}/data \
  --conf=/var/lib/rook/${good_mon_id}/${cluster_namespace}.config \
  --keyring=/var/lib/rook/${good_mon_id}/keyring \
  --monmap=/var/lib/rook/${good_mon_id}/monmap

# review the contents of the monmap
monmaptool --print /tmp/monmap

# remove the bad mon(s) from the monmap
monmaptool ${monmap_path} --rm <bad_mon>

# in this example we remove mon0 and mon2:
monmaptool ${monmap_path} --rm rook-ceph-mon-a
monmaptool ${monmap_path} --rm rook-ceph-mon-c

# inject the monmap into the good mon
ceph-mon -i ${good_mon_id} --inject-monmap ${monmap_path} \
  --cluster=${cluster_namespace} --mon-data=/var/lib/rook/${good_mon_id}/data \
  --conf=/var/lib/rook/${good_mon_id}/${cluster_namespace}.config \
  --keyring=/var/lib/rook/${good_mon_id}/keyring
```

Exit the shell to continue.

### Edit the rook configmap for mons

Edit the configmap that the operator uses to track the mons.
```bash
kubectl -n rook-ceph edit configmap rook-ceph-mon-endpoints
```

In the `data` element you will see three mons such as the following (or more depending on your `moncount`):
```
data: rook-ceph-mon-a=10.100.35.200:6789;rook-ceph-mon-b=10.100.35.233:6789;rook-ceph-mon-c=10.100.35.12:6789
```

Delete the bad mons from the list, for example to end up with a single good mon:
```
data: rook-ceph-mon-b=10.100.35.233:6789
```

Save the file and exit.

### Restart the mon
You will need to restart the good mon pod to pick up the changes. Delete the good mon pod and kubernetes will automatically restart the mon.
```bash
kubectl -n rook-ceph delete pod -l mon=rook-ceph-mon-b
```

Start the rook [toolbox](/Documentation/ceph-toolbox.md) and verify the status of the cluster.
```bash
ceph -s
```

The status should show one mon in quorum. If the status looks good, your cluster should be healthy again.

### Restart the operator
Start the rook operator again to resume monitoring the health of the cluster.
```bash
# create the operator. it is safe to ignore the errors that a number of resources already exist.
kubectl create -f operator.yaml
```

The operator will automatically add more mons to increase the quorum size again, depending on the `monCount`.
