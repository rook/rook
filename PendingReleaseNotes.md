# Major Themes

## Action Required

## Notable Features

### Ceph

- Added a [toolbox job](Documentation/ceph-toolbox.md#toolbox-job) for running a script with Ceph commands, similar to running commands in the Rook toolbox.
- Ceph RBD Mirror daemon has been extracted to its own CRD, it has been removed from the `CephCluster` CRD, see the [rbd-mirror crd](Documentation/ceph-rbd-mirror-crd.html).
- CephCluster CRD has been converted to use the controller-runtime framework.
- Unknown CRD fields will be pruned by Kubernetes.

### EdgeFS

### YugabyteDB

### Cassandra

## Breaking Changes

### Ceph

- rbd-mirror daemons that were deployed through the CephCluster CR won't be managed anymore as they have their own CRD now.
To transition, you can inject the new rbd mirror CR with the desired `count` of daemons and delete the previously managed rbd mirror deployments manually.
- OSD changes:
  - OSD on PVC now supports multipath device.

## Known Issues

### <Storage Provider>

## Deprecations

### <Storage Provider>
