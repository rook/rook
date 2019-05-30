Ceph OSD redesign for Ceph Octopus (May 2019)
==============================================
**Targeted for v1.1-v1.2**

Background
-----------
Rook's current implementation for preparing and starting OSDs includes several different patterns
that have evolved over Rook's lifetime. These patterns include different code paths for supporting
directories and devices as storage targets, and further separated into support for both Filestore
and Bluestore as backing stores. There is further complication added by support for `ceph-volume` as
a tool for storage provisioning versus continued support for legacy installations of Rook which used
manual partitioning.

There has been talk for quite some time about removing support for directories as storage targets,
as this is a use case in Ceph that is not supported for production and is risky for user data
reliability. Additionally, Filestore is effectively in sustaining mode, and Bluestore is the defacto
replacement for it.

All of the various OSD support patterns/code-paths need changes to allow the [planned updates to
Rook's configuration strategy](/design/ceph-config-updates.md) to proceed. The various code paths
are intertangled and difficult to separate, and this presents a huge hurdle for the config changes.
In general, the intertangled OSD code poses a risk for long-term maintenance, as it may be more
prone to bugs being introduced, and it has and will continue to slow development.

In light of this background information, this design proposes three high-level goals:
1. Identify OSD features/code-paths that should be deprecated due to having become outmoded in some
   form.
2. Propose a new architecture for Rook's OSD code that is compatible with the planned config updates
   and that is extensible for the future.
3. Provide a timeline for deprecating outmoded features, and provide users tools for transitioning
   their deployments which use old methods to begin using the new methods.


Deprecated features and code paths
-----------------------------------
This design proposes to deprecate the use of directories as storage targets. This is unsupported for
production in Ceph, and it is not a safe option for storage. Users who wish to run Rook on systems
without dedicated disks will have the option to use individual partitions as storage targets.

Ceph's Filestore backing store will also be deprecated, as it is already in sustaining mode in
Nautilus. Existing OSDs using deprecated features will be supported until there is documentation
explaining how to migrate legacy OSDs to the new format and until users have had ample time to
migrate. Plans for a migration utility are also included.


New OSD operator architecture
------------------------------
The new OSD operator design focuses on devices provisioned using `ceph-volume` and the default OSD
backing store which today is Bluestore.

### Design considerations
#### Viewing disks as state declarations
In iterating through this design, it is clear that the declarative view of applications in
Kubernetes breaks down at the point of reaching the actual disk hardware. Currently, disks can be
"declared" in the Ceph cluster resource, and when those disks are removed from the manifest Rook
automatically removes those from the Ceph cluster -- deletion by omission. While this is not a
problem, this could allow users to inadvertently destroy data in their Ceph cluster.

As an example, the Ceph community has seen that very often OpenStack administrators are not
simultaneously storage administrators. OpenStack administrators often make uninformed decisions that
impact the reliability of their Ceph clusters. The Rook project can safely assume that most
Kubernetes users/administrators are also not simultaneously storage administrators, and it as a
responsibility of Rook to not allow users to undermine the reliability of their storage cluster when
it can be prevented.

This design proposes to change the mental model of where state is "declared" for disk resources.
Ceph OSDs already keep metadata regarding which Ceph cluster they belong to, and Rook can view this
metadata as a declaration of state.

Rook users will still wish to add individual disks to their Ceph clusters to be used as OSDs, but
the deletion-by-omission functionality is removed. Disks which users select for their Ceph clusters
can be viewed as a declaration of intent to **add** disks as OSDs, but once disks are added, the
disk content itself becomes the declaration of **existence** of OSDs. Deleting OSDs currently part
of the Ceph cluster will be a process which requires some manual user intervention via Ceph's CLI or
dashboard tools.

#### Scheduling OSDs on nodes
User error is the largest cause of cluster failures, and Rook should also take actions to favor data
reliability over changes in Kubernetes configurations that might result in OSD pods not being
scheduled on nodes which contain disks that are part of the current Ceph cluster. Specifically, a
user might change taints/tolerations, affinities/anti-affinities, or the Cluster CRD such that a
node previously used for Ceph OSDs is no longer eligible for running OSDs.

This design also proposes to change the mental model of state "declaration" in these cases as well.
In some ways, node selection methods can be thought of as acting similarly to Kubernetes'
`RequiredDuringSchedulingIgnoredDuringExecution` affinity. Rook should view node-selection criteria
as requirements which must be met for provisioning storage from **new** nodes to the Ceph cluster,
but  Rook should still keep storage which was previously provisioned.

Practically, speaking, Kubernetes keeps the state required to know when to orchestrate
previously-existing OSDs on nodes which are no longer eligible for provisioning new OSDs. Rook
should continue to orchestrate existing OSD deployments regardless of their node placement. To do
this, Rook will need to automatically apply new tolerations to OSD existing deployments if the node
on which those OSDs are running has a taint added which would prevent the OSD pod from starting.

#### Automatic OSD deletion
Deletion of OSDs can happen automatically for OSDs which are no longer part of the Ceph cluster. In
the event of an OSD failure, for example, Ceph will eventually detect that the osd has failed and
mark it `out`. Once the data for that OSD is rebalanced, Ceph will mark that OSD as being safe to
delete. When running an orchestration, Rook can safely delete OSDs that are marked safe to delete,
because at this point, reintroducing the OSD to the Ceph cluster would have no benefits compared to
creating a new OSD. For die-hard Ceph administrators, this should have an enable/disable switch in
the CRD to give power users the control they desire.

#### Test environments without disks
Rook must occasionally support test environments without disks. Ceph Bluestore has the ability to
use files as disks for such test environments, and this design proposes to enable this as an option
for such environments.

To enable this feature, the Rook-Ceph operator will observe the environment variable
`TEST_ENVIRONMENT_OSD_STORE_ON_FILE`. If this variable is set with a non-empty-string value, the
operator will not search hosts for devices. It will ignore `useAllDevices`, `deviceFilter`, and any
`devices` directives in the Ceph cluster config and instead create a file in the `dataDirHostPath`
with a size of 10 Gigabytes. This file will be provisioned as an OSD for the tests.

This option is similar to Rook's current usage of directory-based OSDs, but it is critically
different in that it is explicitly reserved for test environment usage and is not made to be
configurable or flexible. This inflexibility is important for keeping the new OSD implementation as
clean, extensible, and maintainable as possible for production features.

#### PVCs as backing storage
The new code path must support using PVCs as backing storage, which is a design that is currently in
progress. PVCs may (or may not) be an an exception to the previous mental model changes. Rook must
be able to query nodes for storage devices, but it may not be able to query PVCs which may be
claimed for Rook's use.

#### Operator needs knowledge of OSD host hardware
Currently, the Rook operator has limited knowledge of the hardware being configured on hosts.
Hardware knowledge is confined to the OSD preparation binary. Mechanisms are not currently in place
to report the needed disk information to the operator. The updated config plan calls for a Ceph
config file that is minimal and standard for all daemons; daemons are individually configured using
command line flags. In order to properly set command line flags for object storage daemons, the
operator must have knowledge of the disks which an OSD will use. The operator must be able to
configure, for example, an OSD to use one disk as the main storage, another disk as wal storage, and
another disk as db storage. The new design will give the operator requisite knowledge.


### Proposed Rook operator design
The existing OSD operator design uses a two-stage process. The first stage is preparation of OSDs on
a per-host basis, and the second stage is running individual OSDs. The design proposed here
separates OSD tasks into more stages.

A Rook OSD prepare binary currently handles the equivalent of stages 1 through 4, and in early
implementations of the new OSD design, a version of the current binary could be used for stages 1
through 3 to achieve the minimum viable product more quickly. Splitting the stages can be done
afterwards as makes sense.

#### Stage 1: Host device inventory
The first stage is an inspection stage that is currently done as part of the existing preparation
stage. Similar to the current Rook OSD prepare binary, this proposal calls for a Rook inspection
binary which will examine the disk hardware, rejecting disks that are already in use by applications
other than Rook (as per current behavior). Rook currently uses `lsblk` and custom logic to find free
disks, but this design proposes to use `ceph-volume inventory` to find free disks. The inspection
binary stores the result of the `ceph-volume inventory` command to a ConfigMap which  will be read
by the operator.

#### Stage 2: Process device inspection report
In the second stage, the operator examines the host device inspection report in the ConfigMap. The
primary inspection processing done would be to find the list of devices which are free for Rook to
use. Free devices are then filtered based on the `deviceFilter` and the host `devices` specified in
the cluster config resulting in a final list of free devices on which Rook is allowed to provision
OSDs.

The insertion of stage 2 executed within the operator itself, placed in between stages 1 and 3
keeps the majority of the Ceph configuration logic in the operator itself rather than distributing
the logic between multiple binaries. Existing logic distribution has contributed to operation which
is not always transparent or easy to extend. More on this below.

This stage should report an error (but not crash the operator) if a device specified in `devices`
could be found or is not available; however, this stage should not report an error if the device
already belongs to the current Ceph cluster.

#### Stage 3: Host device preparation
This stage is one in which `ceph-volume` provisions the free, valid devices. Unless a custom Rook
binary is absolutely necessary, `ceph-volume` can be called directly from the pod's container entrypoint.
Arguments to `ceph-volume` will be based on the host devices found in stage 1 and processing done
in **stage 2**.

As a specific example, adding new OSD preparation options like support for device encryption
involves changes to the Ceph Cluster CRD (required), to the OSD prepare pod specification created by
the Ceph operator, to the Ceph command entrypoint for OSDs, and to the OSD preparation daemon
itself. Splitting the preparation binary into 4 smaller, operator-centric steps means that changes
to the Ceph Cluster CRD only require changes to the CRD and to the pods created by the operator,
cutting the number of locations where code is modified in half.

#### Stage 4: Host device OSD inspection
Rook can determine which OSDs are prepared on the host by running `ceph-volume lvm list` in the same
manner as **Stage 1**. The result is similarly stored in a ConfigMap may now report existing OSDs on
the host. These OSDs may have been created by Rook, or they may have been created manually or via an
action of Ceph's Rook orchestrator.

To find legacy OSDs,

#### Stage 5: Start OSDs
The last step remaining is to start the OSDs which were found by `ceph-volume`'s prior inspection.
The device inspection report stored in **Stage 4** will be used to determine which OSDs on the host
belong to the operator's Ceph cluster (by UUID examination), and appropriate deployments will be
created for each OSD. The report should contain the information necessary to determine which flags
should be passed to `ceph-osd`.

If possible, the existing trend of using init containers instead of a separate Rook binary
(trend established for Ceph mons, mgrs, MDSes, and RGWs) should be used here with the intent being
again to keep most of the logic in the operator. If teardown operations are needed, the Kubernetes
container `PreStop` lifecycle hook should be used if reasonably possible.

#### Shared code
**Stage 1** and **Stage 4** have the same function: run a command and put the result in a ConfigMap.
Implementing this as a building block would allow for reuse in other parts of Rook and in the
future. This could be used immediately for `ceph version` reporting at the beginning of
orchestration without relying on the pod logs being intact, as has been a problem in some user
environments.


Deprecation plan
--------------------
The first step in the deprecation plan is to have the new design in-place. Once the new design is
available, the new design should always be used for any new OSD devices. Legacy OSDs will
not be able to be created, but existing legacy OSDs will still be run using the legacy code path.

Users will be informed in the Rook v1.1 release notes of the intention to deprecate directory-based
OSDs and Filestore as a backing store and informed of the progress toward that goal. Before support
is totally removed for the deprecated features, users should have OSD migration documentation
available, and they should have had a good amount of time to migrate their OSDs.

Once the new OSD design is operational in Rook and is the only code path used for new
OSDs, the deprecation will be official, and that should be reported to users in the release notes
and upgrade guide. After there is documentation for manually migrating OSDs, we can expect that the
following version of Rook will officially drop support for legacy OSDs.

It is ideal if users have access to the migration utility at the time of deprecation; however, the
utility needn't be released at the same time as Rook since it is a separate utility. The utility
must be released before or at the same time as the Rook release which officially drops support for
legacy OSDs. The Rook release which officially drops this support should perform a preflight check
to ensure that no legacy OSDs exist, and if it finds existing legacy OSDs, it should error out with
a message instructing users to migrate.

### OSD migration

For nodes with only directory-based OSDs, the user will need to attach some number of new disks to
those nodes and wait for Rook to provision the new disks for use. The directory-based OSDs will be
migrated by removing one directory-based OSD from the CRUSH map at a time and waiting for Ceph to
report that it is okay to remove the OSD, presumably after its data has been migrated to other
disks. It will be important that the disks added have the capacity to accept the existing data.

For nodes with directory-based OSDs **and** device-based OSDs, new disks need not be added unless
the existing disks do not have the capacity to accept the existing data from directory-based OSDs.
The directory-based OSDs will be migrated as explained previously.

For nodes with device-based OSDs, OSDs will be migrated by removing one OSD from the CRUSH map and
waiting for Ceph to report that it is okay to remove the OSD, presumably after its data has been
migrated to other disks. Once an OSD is removed, it can then be re-created using the new design's
OSD provisioning process. The migration will proceed by repeating this for all deprecated OSDs.

Some users with larger clusters might wish to migrate whole nodes at a time, which could benefit
from improvements that `ceph-volume` can offer when it is able to prepare all of a nodes disks at
once. This could also be done for small clusters but would be a risk for data integrity if the
cluster suffers failures during migration.

This OSD migration process could be done manually by users, but for users with reasonably large
clusters, it would be tedious, and there would be a lot of room for human error. This design
proposes the idea of writing an operator-like utility that would perform the migration in an
automated fashion. The utility would run as a Kubernetes job and iteratively migrate OSDs to the new
form as described above. The user would still be responsible for understanding the generalities of
the migration and for adding new disks to nodes if that would be necessary.

### Timelines

#### Ideal timeline
- Rook v1.1 release
  - new OSD design is complete
  - old OSDs are deprecated
  - migration documentation exists
  - migration utility is released (not strictly required)
- Rook v1.2 release
  - (Rook supports Ceph Octopus)
  - Users still have this release to migrate OSDs
- Rook v1.3 release
  - support for legacy OSDs is officially dropped
  - Rook performs preflight check for legacy OSDs, fails with message to migrate if necessary
  - this is the last release where a migration utility would be useful
- Rook v1.4 release
  - Preflight check for legacy OSDs can be removed

#### Slip timeline
- Rook v1.1 release
  - new OSD design may be complete but unstable
  - documentation informs users of intention to deprecate
  - manual migration steps to remove directory-based OSDs are documented
- Rook v1.2 release
  - new OSD design is complete
  - osd OSDs are deprecated
  - migration documentation exists
  - migration utility is released
  - (Rook supports Ceph Octopus)
- Rook v1.3 release
  - Users still have this release to migrate OSDs
- Rook v1.4 release
  - support for legacy OSDs is officially dropped
  - Rook performs preflight check for legacy OSDs, fails with message to migrate if necessary
  - this is the last release where a migration utility would be useful
- Rook v1.5 release
  - Preflight check for legacy OSDs can be removed

Action plan
------------
With such a complex design, where to start can be daunting. This proposal includes an action plan to
combat this. To begin, this design plans to create an entirely new code tree for the redesigned
OSDs. The new code tree should make copy-paste use of existing code where appropriate to maintain a
good velocity.

This code tree will not be enabled in Rook until the following features are met:
- the new code tree is fully featured for creating new Bluestore OSDs on devices
- there is a selection mechanism in place which can select between the existing code tree and the
  new code tree "correctly"
  - the new code tree should be used for creating new Bluestore OSDs and for running OSDs created
    with the new code tree
  - the old code tree should be used for all directory-based OSD operations, all Filestore OSD
    operations, and for running Bluestore OSDs created with the old code tree
- enabling the new code tree in Rook passes all existing integration tests

It may be possible for the new code tree to take on running Bluestore OSDs which were created using
`ceph-volume` by the old code tree. Work on the initial code tree when it is isolated from the
current code tree (unused in Rook) can proceed both quickly and in small increments by not needing
to pass integration tests in this initial phase.

Deprecating the existing OSD code tree cannot proceed until the new code tree is at feature parity
with the existing tree minus the features intended to be deprecated. Notably, the new code tree must
support PVCs as backing devices and using a file as a Bluestore device.

### Action timeline
In order:
1. Work on foundational features (in any order)
  - Stages 1-3 host device preparation
  - Stage 4 host device inspection and ConfigMap reporting
  - Stage 5 osd starting
2. Implement selection mechanism; must pass integration tests

At any time with the above:
- Support PVCs as backing devices in new code path
- Support using a file as a Bluestore device
  - Note that this feature is independent from both code paths; it should short-circuit both the
    new code path as well as the old code path
- Support using partitions as backing devices

After the above, write migration documentation, and make deprecation official. And as a stretch
goal, write a migration utility, and document migration with the utility.

Finally, wait one extra release for users to migrate OSDs before dropping support for legacy OSDs.
