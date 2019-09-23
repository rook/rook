# Removing OSDs from a Ceph cluster

**Targeted for v1.2**

## Background

Removing OSDs is a common day-2 operation and users need to be able to perform
this regularly.

With the effort to add OSD replacement in the Ceph Dashboard, there is a demand
for automating the deletion of individual OSDs.

## Status

As of Rook 1.1, there is some preliminary support for removing OSDs:

### Removing Nodes

Rook fully supports the removal of whole nodes from a Cluster, including all
OSDs. This implementation does every call needed to completely remove all OSDs
on that specific node from a Ceph cluster if that cluster no longer appears in
the list of nodes in the CephCluster CR.

This implementation will be removed from Rook, as OSDs are removed, even if a
node is being removed inadvertently.

### Removing individual OSDs

Rook partially supports individual OSD-removal: The Rook operator monitors Ceph
health and initiates the remove of OSDs that are, according to
`ceph safe-to-destroy`, safe to be destroyed. Rook then removes the OSD
Deployment automatically.

Note that the device might stay referenced within the `CephCluster` CR.

This feature is mainly aimed at automatically removing unresponsive OSDs.

### Manual removal

There is an existing Rook issue
[#1827](https://github.com/rook/rook/issues/1827) containing a description of
how to remove OSDs manually.

## Design considerations

Any future implementation within Rook needs to fulfill some requirements:

1. To prevent inadvertent removal of OSDs, the act of removing OSDs needs to be
   executed explicitly and manually.
2. Omitting node or drive declarations within the `CephCluster` CR should not
   physically remove OSDs.
3. Except for physically removing data on disks, there should not be any manual
   step required to allow a subsequent creation of a replacement OSD on the same
   node.
4. There should not be a perceived fundamental difference when removing whole
   nodes versus removing single OSDs.

## Algorithm Outline

Removing OSDs looks as follows when looking at the bare minimum of required
steps (pseudocode):

```python
def remove_osd_mgr(osd_id, node, device_name, replace):
    if replace:
        assert ceph_safe-to-destroy()
        ceph_osd_destroy(osd_id)
    else:
        # reweight to 0.0 prevents data
        # rebalancing upon CRUSH removal
        ceph_crush_reweight(osd_id, 0.0)
        ceph_osd_out(osd_id)

    remove_osd_from_CephCluster_CR(node, device_name)


def remove_osd_rook_operator():
    while True:
        for osd_id in ceph_osd_ls():
            if not osd_safe_to_destroy(osd_id):
                continue
            if 'destroyed' not in ceph_osd_dump().osds[osd_id].state:
                ceph_osd_purge()
            delete_osd_in_kubernetes()
```

## Implementation

Initiating the process of removing OSDs from Ceph cluster is implemented within
the Ceph Orchestrator by implementing `remove_osd_mgr` of the algorithm outlined
above within the Rook MGR module:

Before the Rook operator removes Kubernetes primitives, like Deployments, the
Rook MGR module performs the data migrations and marks the OSD as `out`.

The Rook MGR module continues by removing the drives declarations associated
with that specific OSD within the `CephCluster` CR specification.

As the Rook operator monitors Ceph health, it will detect this OSD, queries
`ceph safe-to-destroy` and removes OSD Deployments automatically and calls
`ceph osd purge`, if the OSD is not marked as `destroyed`

With the addition of the balancer MGR module and the pg-autoscaler MGR module,
there are already existing examples of non-trivial data management
functionality. In addition, the Ceph community aims at providing a
vendor-agnostic orchestrator implementation that would also need to implement
similar same steps. By implementing the algorithm outlined above, that future
orchestrator would directly benefit from the functionality provided by the Rook
MGR module.

In case the OSD is supposed to be replaced, the physical drive needs to be
cleaned:

```
ceph-volume lvm zap /dev/sdX
```
