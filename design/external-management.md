# Enable external Ceph management tools

Target version: 0.9

## TL;DR

Some tools want to use Rook to run containers, but not manage the logical
Ceph resources like Pools.  We should make Rook's pool management optional.

## Background

Currently in Rook 0.8, creating and destroying a Filesystem (or ObjectStore)
in a Ceph cluster also creates and destroys the associated Ceph filesystem
and pools.

The current design works well when the Ceph configuration is within the
scope of what Rook can configure itself, and the user does not modify
the Ceph configuration of pools out of band.

## Limitations

The current model is problematic in some cases:

- A user wants to use Ceph functionality outside of Rook's subset, and
  therefore create their pools by hand before asking Rook to run
  the daemon containers for a filesystem.
- A user externally modifies the configuration of a pool (such as the
  number of replicas), they probably want that new configuration, rather than
  for Rook to change it back to match the Rook Filesystem settings.
- A risk-averse user wants to ensure that mistaken edits to their Rook config cannot
  permanently erase Ceph pools (i.e. they want to only delete pools through
  an imperative interface with confirmation prompts etc).

## Proposal

In FilesystemSpec (and ObjectStoreSpec), when the metadata and
data pool fields are left empty, Rook will not do any management of logical
Ceph resources (Ceph pools and Ceph filesystems) for the filesystem.

The pools may be initially non-nil, and later modified
to be nil.  In this case, while Rook may have created the logical
resources for the filesystem, it will not remove them when the Rook filesystem
is removed.

If either of the metadata/data fields are non-nil, then they both must
be non-nil: Rook will not partially manage the pools for a given filesystem
or object store.

### Before (pools always specified)

```yaml
apiVersion: ceph.rook.io/v1
kind: Filesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataPool:
    replicated:
      size: 3
  dataPools:
    - erasureCoded:
       dataChunks: 2
       codingChunks: 1
  metadataServer:
    activeCount: 1
    activeStandby: true
```

### After (pools may be omitted)

In this example, the pools are omitted.  Rook will not create
any pools or a Ceph filesystem.  A filesystem named ``myfs`` should already
exist in Ceph, otherwise Rook will not start any MDS pods.

```yaml
apiVersion: ceph.rook.io/v1
kind: Filesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataServer:
    activeCount: 1
    activeStandby: true
```


## Impact

- Rook Operator: add logic to skip logical resource
management when pools are omitted in FilesystemSpec or ObjectStoreSpec

- Migration: none required.  Existing filesystems and objectstores always
have pools set explicitly, so will continue to have these managed by Rook.
