# Updating OSDs in parallel
**Targeted for v1.6**

## Background
In clusters with large numbers of OSDs, it can take a very long time to update all of the OSDs. This
occurs on updates of Rook and Ceph both for major as well as the most minor updates. To better
support large clusters, Rook should be able to update (and upgrade) multiple OSDs in parallel.


## High-level requirements

### Ability to set a maximum number of OSDs to be updated simultaneously
In the worst (but unlikely) case, all OSDs which are updated for a given parallel update operation
might fail to come back online after they are updated. Users may wish to limit the number of OSDs
updated in parallel in order to avoid too many OSDs failing in this way.

### Cluster growth takes precedence over updates
Adding new OSDs to a cluster should occur as quickly as possible. This allows users to make use of
newly added storage as quickly as possible, which they may need for critical applications using the
underlying Ceph storage. In some degraded cases, adding new storage may be necessary in order to
allow currently-running Ceph OSDs to be updated without experiencing storage cluster downtime.

This does not necessarily mean that adding new OSDs needs to happen before updates.

This prioritization might delay updates significantly since adding OSDs not only adds capacity to
the Ceph cluster but also necessitates data rebalancing. Rebalancing generates data movement which
needs to settle for updates to be able to proceed.

### OSD updates should not starve other resources of updates
For Ceph cluster with huge numbers of OSDs, Rook's process to update OSDs should not starve other
resources out of the opportunity to get configuration updates.


## Technical implementation details

### Changes to Ceph
The Ceph manager (mgr) will add functionality to allow querying the maximum number of OSDs that are
okay to stop safely. The command will take an initial OSD ID to include in the results. It should 
return error if the initial OSD cannot be stopped safely. Otherwise it returns a list of 1 or more
OSDs that can be stopped safely in parallel. It should take a `--max=<int>` parameter that limits 
the number of OSDs returned.

It will look similar to this on the command line `ceph osd ok-to-stop $id --max $int`.

The command will have an internal algorithm that follows the flow below:
1. Query `ok-to-stop` for the "seed" OSD ID. This represents the CRUSH hierarchy bucket at the "osd"
   (or "device") level.
2. If the previous operation reports that it safe to update, batch query `ok-to-stop` for all OSDs
   that fall under the CRUSH bucket one level up from the current level.
3. Repeat step 3 moving up the CRUSH hierarchy until one of the following two conditions:
    1. The number of OSDs in the batch query is greater than or equal to the `max` parameter, OR
    2. It is no longer `ok-to-stop` all OSDs in the CRUSH bucket.
4. Update OSD Deployments in parallel for the last CRUSH bucket where it was `ok-to-stop` the OSDs.
    - If there are more OSDs in the CRUSH bucket than allowed by the user that are okay to stop, 
      return only the `max` number of OSD IDs from the CRUSH bucket.

The pull request for this feature in the Ceph project can be found at 
https://github.com/ceph/ceph/pull/39455.

### Rook Operator Workflow
1. Build an "existence list" of OSDs which already have Deployments created for them.
1. Build an "update queue" of OSD Deployments which need updated.
1. Start OSD prepare Jobs as needed for OSDs on PVC and OSDs on nodes.
   1. Note which prepare Jobs are started
1. Provision Loop
   1. If all prepare Jobs have been completed and the update queue is empty, stop Provision Loop.
   1. If there is a `CephCluster` update/delete, stop Provision Loop with a special error.
   1. Create OSDs: if a prepare Job has completed, read the results.
      1. If any OSDs reported by prepare Job do not exist in the "existence list", create them.
      1. Mark the prepare Job as completed.
      1. Restart Provision Loop.
   1. Update OSDs: if the update queue is not empty, update a batch of OSD Deployments.
      1. Query `ceph osd ok-to-stop <osd-id> --max=<int>` for each OSD in the update queue until 
         a list of OSD IDs is returned.
         - If no OSDs in the update queue are okay to stop, Restart Provision Loop.
      1. Update all of the OSD Deployments in parallel.
      1. Record any failures.
      1. Remove all OSDs from the batch from the update queue (even failures).
      1. Restart Provision Loop.
1. If there are any recorded errors/failures, return with an error. Otherwise return success.

### How to make sure Ceph cluster updates are not starved by OSD updates
Because [cluster growth takes precedence over updates](#cluster-growth-takes-precedence-over-updates),
it could take a long time for all OSDs in a cluster to be updated. In order for Rook to have
opportunity to reconcile other components of a Ceph cluster's `CephCluster` resource, Rook should
ensure that the OSD update reconciliation does not create a scenario where the `CephCluster` cannot
be modified in other ways.

https://github.com/rook/rook/pull/6693 introduced a means of interrupting the current OSD
orchestration to handle newer `CephCluster` resource changes. This functionality should remain so
that user changes to the `CephCluster` can begin reconciliation quickly. The Rook Operator should
stop OSD orchestration on any updates to the `CephCluster` spec and be able to resume OSD 
orchestration with the next reconcile.

### How to build the existence list
List all OSD Deployments belonging to the Rook cluster. Build a list of OSD IDs matching the OSD
Deployments. Record this in a data structure that allows O(1) lookup.

### How to build the update queue
List all OSD Deployments belonging to the Rook cluster to use as the update queue. All OSDs should
be updated in case there are changes to the CephCluster resource that result in OSD deployments
being updated.

The minimal information each item in the queue needs is only the OSD ID. The OSD Deployment managed
by Rook can easily be inferred from the OSD ID.

Note: A previous version of this design planned to ignore OSD Deployments which are already updated.
The plan was to identify OSD Deployments which need updated by looking at the OSD Deployments for:
(1) a `rook-version` label that does not match the current version of the Rook operator AND/OR
(2) a `ceph-version` label that does not match the current Ceph version being deployed. This is an
invalid optimization that does not account for OSD Deployments changing due to CephCluster resource
updates. Instead of trying to optimize, it is better to always update OSD Deployments and rely on the
lower level update calls to finish quickly when there is no update to apply.

### User configuration

#### Modifications to `CephCluster` CRD
Establish a new `updatePolicy` section in the `CephCluster` `spec`. In this section, users can
set options for how OSDs should be updated in parallel. Additionally, we can move some existing
one-off configs related to updates to this section for better coherence. This also allows for
a natural location where future update options can be added.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
# ...
spec:
  # ...
  # Move these to the new updatePolicy but keep them here for backwards compatibility.
  # These can be marked deprecated, but do not remove them until CephCluster CRD v2.
  skipUpgradeChecks:
  continueUpgradeAfterChecksEvenIfNotHealthy:
  removeOSDsIfOutAndSafeToDestroy:

  # Specify policies related to updating the Ceph cluster and its components. This applies to
  # minor updates as well as upgrades.
  updatePolicy:
    # skipUpgradeChecks is merely relocated from spec
    skipUpgradeChecks: <bool, default=false>

    # continueUpgradeAfterChecksEvenIfNotHealthy is merely relocated from spec
    continueUpgradeAfterChecksEvenIfNotHealthy: <bool, default=false, relocated from spec>

    # allow for future additions to updatePolicy like healthErrorsToIgnore

    # Update policy for OSDs.
    osds:
      # removeIfOutAndSafeToDestroy is merely relocated from spec (removeOSDsIfOutAndSafeToRemove)
      removeIfOutAndSafeToDestroy: <bool, default=false>

      # Max number of OSDs in the cluster to update at once. Rook will try to update this many OSDs 
      # at once if it is safe to do so. It will update fewer OSDs at once if it would be unsafe to 
      # update maxInParallelPerCluster at once. This can be a discrete number or a percentage of 
      # total OSDs in the Ceph cluster.
      # Rook defaults to updating 15% of OSDs in the cluster simultaneously if this value is unset.
      # Inspired by Kubernetes apps/v1 RollingUpdateDeployment.MaxUnavailable.
      # Note: I think we can hide the information about CRUSH from the user since it is not
      #       necessary for them to understand that complexity.
      maxInParallelPerCluster: <k8s.io/apimachinery/pkg/util/intstr.intOrString, default=15%>
```

Default `maxInParallelPerCluster`: Ceph defaults to keeping 3 replicas of an item or 2+1 erasure
coding. It should be impossible to update more than one-third (33.3%) of a default Ceph cluster at
any given time. It should be safe and fairly easy to update slightly less than half of one-third at
once, which rounds down to 16%. 15% is a more round number, so that is chosen instead.


## Future considerations
Some users may wish to update OSDs in a particular failure domain or zone completely before moving
onto updates in another zone to minimize risk from updates to a single failure domain. This is out 
of scope for this initial design, but we should consider how to allow space to more easily implement
this change when it is needed.