# Adding and removing Ceph osds via Ceph orchestrator interface

## Goals

1. Give Ceph's Rook Orchestrator Plugin the ability to add and remove OSDs to/from a Rook-Ceph
   cluster running in Kubernetes.
2. Users of the Ceph Orchestrator CLI and/or Ceph Dashboard should be able to
   create OSDs on nodes with as much complexity as `ceph-volume` is able to support all possible
   user needs.
3. Give as much room as possible for the Ceph Mgr Orchestrator Module and `ceph-volume` to implement
   OSD management logic so that Rook's code can be simpler and long-term code maintenance on Rook
   can be reduced.


## Current status

Rook expects to manage storage declaratively. Users can add disks to a Rook cluster by adding them
to the `CephCluster` resource via the disk name itself, via 2 forms of pattern matching, or via the
`useAllDisks` option.

The Ceph Mgr Orchestrator Module expects to add disks to a Ceph cluster declaratively using a Ceph
[Drive Group](https://github.com/ceph/ceph/blob/master/src/python-common/ceph/deployment/drive_group.py)
specification. The parent Orchestrator Module in Ceph will not store user-specified Drive Groups;
however, it does expect the underlying Orchestrator or Orchestrator Mgr Module to store the Drive
Group spec.

Even with Drive Groups, Ceph expects the on-disk content of disks to be the declarative state of
whether an OSD belongs to a Ceph cluster or not and what its configuration is. Rook is currently
able to read this on-disk content via `ceph-volume` to determine if it should run a Kubernetes
Deployment for a disk-based OSD.

Rook currently does not implement OSD removal via any declarations in the `CephCluster` resource in
order to prioritize the safety of user data. If a user wishes to remove an OSD, they must perform
manual, imperative steps to do so.


## Proposal

### Feature requests to Ceph project
This proposal will require changes to Rook and to Ceph's Rook Orchestrator Mgr Module. These changes
are assumed, but requested changes to other Ceph components will be noted here.

1. `ceph-volume` - Accept Drive Groups as input via command line flag
   1. Should be automatically converted and issued `ceph-volume lvm batch` commands under the hood
   2. Should also support being converted to and issued as `ceph-volume` `raw` mode commands
      - This means the Python conversion logic should support conversion to `raw` commands
   3. Support JSON and YAML strings as inputs to command line flag
   4. `ceph-volume` should require a `--hostname` flag which contains the node's host name
      - this will be used to match the `host_pattern`, and if the pattern does not match,
      `ceph-volume` should report success without performing any operations

### Adding OSDs
This design proposes to add a section to the `CephCluster` resource's `storage` configuration
section to allow disks to be added via a Drive Groups specification. The spec is proposed below with
behavior described to follow.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
spec:
  storage: # preexisting
    driveGroups: < YAML blob of Drive Groups > # proposed new
      # # Examples below
      # regular_nodes:
      #   host_pattern: '*-osd'
      #   data_devices:
      #     all: true
      # fast_nodes:
      #   host_pattern: '*-osd-fast'
      #   data_devices:
      #     limit: 6
      #     size: "10TB:10TB"
```

`driveGroups` accepts a YAML blob of Drive Group specifications. Each Drive Group spec begins with a
named key which is the name of the Drive Group, used to uniquely identify the Drive Group. Rook's
internal `Go` spec for a drive group is given below.

```go
// DriveGroups is a map of Drive Group names to Drive Group specifications.
type DriveGroups map[string]DriveGroupSpec

// DriveGroupSpec is a mapping from keys to values
// Rook must use the "host_pattern" key to determine the host on which to run the Drive Group
type DriveGroupSpec map[string]interface{}
```

`ceph-volume` feature request #1 will be used here to allow Rook to pass along the Drive Group YAML
blob without modification or inspection to `ceph-volume` when running the OSD provisioning job. OSD
creation via Drive Groups is given priority and executed before any other CRD-based OSD creation
methods are used (the other CRD-based creations methods are still used after the Drive Group
method).

Drive Groups should be defined only at the cluster level, not at node level. Drive Groups support a
standard glob matching pattern for nodes, and Rook should pass `ceph-volume` the `--hostname` flag
defined in feature #1-4. Rook will run Drive Groups on all nodes, allowing the glob pattern logic
present Ceph and `ceph-volume` to determine whether the node should have a drive group applied to
it. Drive Groups in Rook will be passed through to `ceph-volume` without modification.

Users of `CephCluster` via Kubernetes Manifest will now be able to supply their own Drive Groups
spec to Rook if they so desire to allow themselves greater flexibility in OSD management. Rook will
treat this user spec as a declarative intent for the cluster.

The Rook Orchestrator Mgr Module should be able to report the Drive Groups in the Rook configuration
to the user, and the Orchestrator Module will have the ability to add new drive groups, remove
existing drive groups, and modify existing drive groups. The user should be advised to only manage
drive groups via one method: either via the `CephCluster` resource themselves manually or via the
Orchestrator CLI or Dashboard interface.

#### Reporting Status
Rook should report failure messages back to the Rook Orchestrator Mgr Module via a `CephCluster`
`status` mechanism so that the Orchestrator Module can take appropriate actions ot notify the user
of failure. Rook may report successful statuses back if needed by the Orchestrator Module; however,
limiting the information reported back will keep the `CephCluster` resource's `status` field smaller
and less unwieldy for users.

```yaml
# first draft suggestions for status mechanism
apiVersion: ceph.rook.io/v1
kind: CephCluster
status:
  osdsOnNodes: # proposed new
    - node: < string, node name > # proposed new
      driveGroups:
        - name: < string, name of drive group applied to the node > # proposed new
          success: < bool, true if applied successfully, false otherwise >
          message: < string, message from results of last drive group application >
```

Rook should report the Drive Groups it has selected to create on each node by reporting the name of
the Drive Group (each drive group has an identifying name.)

The failure status of the last-applied `ceph-volume` command with the Drive Group spec is
reported in `status`, and a corresponding `message` is also reported. The message must be filled in
if the status is `false` (unsuccessful).

If a Drive Group is removed from the `storage` spec, its corresponding statuses must be removed as
well.

### Removing OSDs
When removing an OSD (or OSDs) via the Rook Orchestrator Module, the Ceph mgr running in Kubernetes
will also perform whatever Ceph operations are necessary to remove the OSD as would be expected by
the user. This will replace the manual, imperative steps currently required by Rook-Ceph users to
remove OSDs from the storage cluster documented
[here for Rook v1.2](https://rook.io/docs/rook/v1.2/ceph-osd-mgmt.html#remove-an-osd).

There is currently a proposal for a shared Ceph Mgr Orchestrator Module implementing common OSD
removal steps [[link]](https://github.com/ceph/ceph/pull/32677) usable by any other Ceph Mgr
Orchestrator module. The OSD removal mechanism should use this where possible.

Rook will not automatically remove Kubernetes Deployments for OSDs unless
`removeOSDsIfOutAndSafeToRemove: true`. This should be documented for users, and users should be
advised to set this value to `true`.

The `CephCluster` CRD allows users to add disks to nodes in several places including disks on all
nodes in the Kubernetes cluster, to a specific node, or via disk pattern matching (in two different
ways). To avoid the complexity of having to manage the logic for determining how to remove a node
from the `CephCluster` resource, OSD removal will **not** alter the `CephCluster` resource's
`storage` specification with the exception of Drive Groups. While modifying other items might be
convenient for users, there are a lot of opportunities in development to miss corner cases or
introduce logic bugs, which we should avoid.

Users should be advised in documentation that if they plan to use the Ceph CLI or Dashboard to
manage OSD addition in the Rook-Ceph cluster, it is recommended to set `useAllDevices: false`, leave
both disk pattern matching empty, and to not specify any other disks in the `CephCluster` resource
to avoid giving Rook the ability to re-add OSDs when the user does not wish it to. Particularly to
avoid possible confusion and annoyance, we should note to users that Rook can automatically re-add
removed OSDs if removed OSDs are zapped manually and not removed from OSD nodes.


## Not in scope

It is **not** within the scope of this design to add or remove nodes from the `CephCluster` resource
via Orchestrator Module `osd` commands. If a node does not exist in the `CephCluster` resource, they
should not be allowed to add OSDs to the node via the Orchestrator Module. Orchestrator Module users
must first use an Orchestrator `add node` command. If the user removes all OSDs from a node via the
Orchestrator Module, that node should remain in the `CephCluster` resource assuming to have been
added by an Orchestrator Module `add node` command.

It is **not** the Rook Orchestrator Mgr Module's job to ensure that disks are added or removed from
the `CephCluster` resource's `storage` configuration. It will only add, modify, and remove Drive
Group specs.

Adding and removing OSDs builds a foundation for how to **replace** OSDs; however, this topic is
left for future design and implementation.


## Alternatives considered

We considered that the Rook Orchestrator Mgr Module could run a Kubernetes Job independently of Rook
to run an arbitrary `ceph-volume` command on a node. however, this would require a complicated
sync/locking mechanism to be developed to ensure that the Rook orchestration loop and the Mgr-run
job do not try to modify disks on the same node at once, and it was decided that this was too
complex compared to adding imperative Drive Groups-based OSD addition to the `CephCluster` resource.

We considered that the Rook Operator could remove a Drive Group spec from the `CephCluster` resource
once it had applied the spec; however, this goes against the design guideline that Operators should
not alter `spec`s for Custom Resources. Similarly, we considered that the Rook Orchestrator Mgr
Module could clear statuses in the `CephCluster` `status` section once it had read them, but this
also goes against a related design guideline that users should not alter the `status` of a resource.

We considered not giving the Rook (or the Orchestrator Mgr Module) the ability to zap disks with
Rook, but this is a feature many users desire. It should be included, even though implementation is
complicated due to difficulties harmonizing declarative and imperative operations into the same
paradigm.

We considered requesting a `--blacklist` flag to `ceph-volume` which would use LVM to apply
labels to PVs or LVs which `ceph-volume` would see and ignore; however, this conflicts with user
desires to completely zap a disk clean and have no LVM info left on it. Requesting a blacklist to
Drive Groups was deemed a more natural solution.

We also considered requesting a blacklist feature to be added to Drive Groups, but after
deliberation, the Ceph community decided that the best way to blacklist devices would be simply to
not zap them. Blacklisting a zapped device after OSD removal so it doesn't get automatically added
back into the Ceph cluster is something of a corner case.


## Appendix A
The OSD `status` suggested here includes only the detail needed for Drive Group status reporting,
but it is extensible for adding more status in the future, a topic that has come up in Rook a number
of times. A suggestion for a more full status is shown below to make sure that the status proposed
herein is extensible as desired, but this fuller status is not under review herein.

```yaml
# Example only
osdsOnNodes:
  - node: <name>
    count: <number of OSDs configured on the node>
    success: <true | false>
    messages:
      - <failure message 1>
      - <failure message 2>
    driveGroups:
      name: <name>
      success: <true | false>
      message: <failure message>
  osdsOnPVCs:
  - pvc: <name>
    success: <true | false>
    message: <failure message>
```
