# Ceph monitor PV storage

**Target version**: Rook 1.1

## Overview

Currently all of the storage for Ceph monitors (data, logs, etc..) is provided
using HostPath volume mounts. Supporting PV storage for Ceph monitors in
environments with dynamically provisioned volumes (AWS, GCE, etc...) will allow
monitors to migrate without requiring the monitor state to be rebuilt, and
avoids the operational complexity of dealing with HostPath storage.

The general approach taken in this design document is to augment the CRD with a
persistent volume claim template describing the storage requirements of
monitors. This template is used by Rook to dynamically create a volume claim for
each monitor.

## Monitor storage specification

The monitor specification in the CRD is updated to include a persistent volume
claim template that is used to generate PVCs for monitor database storage.

```go
type MonSpec struct {
	Count                int  `json:"count"`
	AllowMultiplePerNode bool `json:"allowMultiplePerNode"`
	VolumeClaimTemplate  *v1.PersistentVolumeClaim
}
```

The `VolumeClaimTemplate` is used by Rook to create PVCs for monitor storage.
The current set of template fields used by Rook when creating PVCs are
`StorageClassName` and `Resources`. Rook follows the standard convention of
using the default storage class when one is not specified in a volume claim
template. If the storage resource requirements are not specified in the claim
template, then Rook will use a default value. This is possible because unlike
the storage requirements of OSDs (xf: StorageClassDeviceSets), reasonable
defaults (e.g. 5-10 GB) exist for monitor daemon storage needs.

*Logs and crash data*. The current implementation continues the use of a
HostPath volume based on `dataDirHostPath` for storing daemon log and crash
data. This is a temporary exception that will be resolved as we converge on an
approach that works for all Ceph daemon types.

Finally, the entire volume claim template may be left unspecified in the CRD
in which case the existing HostPath mechanism is used for all monitor storage.

## Upgrades and CRD changes

When a new monitor is created it uses the _current_ storage specification found
in the CRD. Once a monitor has been created, it's backing storage is not
changed. This makes upgrades particularly simple because existing monitors
continue to use the same storage.

Once a volume claim template is defined in the CRD new monitors will be created
with PVC storage. In order to remove old monitors based on HostPath storage
first define a volume claim template in the CRD and then fail over each monitor.

## Clean up

Like `StatefulSets` removal of an monitor deployment does not automatically
remove the underlying PVC. This is a safety mechanism so that the data is not
automatically destroyed. The PVCs can be removed manually once the cluster is
healthy.

## Requirements and configuration

Rook currently makes explicit scheduling decisions for monitors by using node
selectors to force monitor node affinity. This means that the volume binding
should not occur until the pod is scheduled onto a specific node in the cluster.
This should be done by using the `WaitForFirstConsumer` binding policy on the
storage class used to provide PVs to monitors:

```
kind: StorageClass
volumeBindingMode: WaitForFirstConsumer
```

When using existing HostPath storage or non-local PVs that can migrate (e.g.
network volumes like RBD or EBS) existing monitor scheduling will continue to
work as expected. However, because monitors are scheduled without considering
the set of available PVs, when using statically provisioned local volumes Rook
expects volumes to be available. Therefore, when using locally provisioned
volumes take care to ensure that each node has storage provisioned.

Note that these limitations are currently imposed because of the explicit
scheduling implementation in Rook. These restrictions will be removed or
significantly relaxed once monitor scheduling is moved under control of Kubernetes
itself (ongoing work).

## Scheduling

In previous versions of Rook the operator made explicit scheduling (placement)
decisions when creating monitor deployments. These decisions were made by
implementing a custom scheduling algorithm, and using the pod node selector to
enforce the placement decision.  Unfortunately, schedulers are difficult to
write correctly, and manage.  Furthermore, by maintaining a separate scheduler
from Kubernetes global policies are difficult to achieve.

Despite the benefits of using the Kubernetes scheduler, there are important use
cases for using a node selector on a monitor deployment: pinning a monitor to a
node when HostPath-based storage is used. In this case Rook must prevent k8s
from moving a pod away from the node that contains its storage. The node
selector is used to enforce this affinity. Unfortunately, node selector use is
mutually exclusive with kubernetes scheduling---a pod cannot be scheduled by
Kubernetes and then atomically have its affinity set to that placement decision.

The workaround in Rook is to use a temporary _canary pod_ that is scheduled by
Kubernetes, but whose placement is enforced by Rook.  The canary deployment is a
deployment configured identically to a monitor deployment, except the container
entrypoints have no side affects. The canary deployments are used to solve a
fundamental bootstrapping issue: we want to avoid making explicit scheduling
decisions in Rook, but in some configurations a node selector needs to be used
to pin a monitor to a node.

### Health checks

Previous versions of Rook performed periodic health checks that included checks
on monitor health as well as looking for scheduling violations. The health
checks related to scheduling violations have been removed. Fundamentally a
placement violation requires understanding or accessing the scheduling algorithm.

The rescheduling or eviction aspects of Rook's scheduling caused more problems
than it helped, so going with K8s scheduling is the right thing.  If/when K8s
has eviction policies in the future we could then make use of it (e.g. with
`*RequiredDuringExecution` variants of anti-affinity rules are available).

### Target Monitor Count

The `PreferredCount` feature has been removed.

The CRD monitor count specifies a target minimum number of monitors to maintain.
Additionally, a preferred count is available which will be the desired number of
sufficient number of nodes are available. Unfortunately, this calculation
relies on determining if monitor pods may be placed on a node, requiring
knowledge of the scheduling policy and algorithm. The scenario to watch out for
is an endless loop in which the health check is determining a bad placement but
the k8s schedule thinks otherwise.

## Monitor Storage Class Support

It is generally desirable in production to have multiple zones availability. A
whole storage class could go unavailable then and enough mons could stay up to
keep the cluster running.
In such a case the distribution of mons can be made more homogeneous for each
zone, by this we can leverage the spread of mons for providing high
availability. This could be a useful scenario for a datacenter with own storage
class in contrast to ones provided by cloud providers.

To support this scenario, we can do something similar to the mon deployment for
[Stretch clusters](https://github.com/ceph/ceph/blob/master/doc/rados/operations/stretch-mode.rst).
The operator will have zonal awareness, associating mon deployments based on
zones configuration provided.

The type of failure domain used for this scenario is commonly "zone", but can
be set to a different failure domain.

### Failure Domain Configuration

The topology of the K8s cluster is to be determined by the admin, outside the scope of Rook.
Rook will simply detect the topology labels that have been added to the nodes.

If the desired failure domain is a "zone", the `topology.kubernetes.io/zone` label should
be added to the nodes. Any of the [topology labels](https://rook.io/docs/rook/latest/ceph-cluster-crd.html#osd-topology)
supported by OSDs can be used also for this scenario.

For example:

```yaml
topology.kubernetes.io/zone=a
topology.kubernetes.io/zone=b
topology.kubernetes.io/zone=c
```

###  Design

The core changes to rook are to associate the mons with the required zones.

- Each zone will have at most one mon assigned using labels for nodeAffinity.
 If there are more zones than mons, some zones may not have a mon.

The new configuration will support the `zones` configuration where the
failure domains available must be listed.

If the mon.zones are specified, the mon canary pod should not specify the volume
for the mon store. The whole purpose of the mon canaries is to allow the k8s
scheduler to decide where the mon should be scheduled.
So if we don't add a volume to the canary pod, we can delay the creation of the
mon PVC until the canary pod is done. Then after the mon canary is done
scheduling, the operator can look at the node where it was scheduled and
determine which zone (or other topology) the node belonged to.
For the mon daemon pod, then the operator would create the PVC from the
volumeClaimTemplate matching the zone where the mon canary was created.
This way mon canary pod will be able to take care of all scheduling tasks.
All zones must be listed in the cluster CR. If mons are expected to run on a
subset of zones, the needed node affinity must be added to the placement.mon on
the cluster CR so the mon canaries and daemons will be scheduled in the correct
zones.

In this example, each of the mons will be backed by a volumeClaimTemplate
specific to a different zone:

```yaml
 mon:
    count: 3
    allowMultiplePerNode: false
    zones:
    - name: a
      volumeClaimTemplate:
        spec:
          storageClassName: zone-a-storage
            resources:
              requests:
                storage: 10Gi
    - name: b
      volumeClaimTemplate:
        spec:
          storageClassName: zone-b-storage
            resources:
              requests:
                storage: 10Gi
    - name: c
      volumeClaimTemplate:
        spec:
          storageClassName: zone-c-storage
            resources:
              requests:
                storage: 10Gi
```

