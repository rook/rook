# Major Themes

## Action Required

## Notable Features

### Ceph

- Added a [toolbox job](Documentation/ceph-toolbox.md#toolbox-job) for running a script with Ceph commands, similar to running commands in the Rook toolbox.
- Ceph RBD Mirror daemon has been extracted to its own CRD, it has been removed from the `CephCluster` CRD, see the [rbd-mirror crd](Documentation/ceph-rbd-mirror-crd.html).
- CephCluster CRD changes:
  - Converted to use the controller-runtime framework
  - ability to control health check as well as pod liveness probe, refer to [health check section](Documentation/ceph-cluster-crd.html#health-settings)
- CephBlockPool CRD has a new field called `parameters` which allows to set any property on a given [pool](Documentation/ceph-pool-crd.html#add-specific-pool-properties)
- OSD changes:
  - OSD on PVC now supports multipath device.
  - OSDs can now be provisioned using Ceph's Drive Groups definitions for Ceph Octopus v15.2.5+.
    [See docs for more](Documentation/ceph-cluster-crd.md#storage-selection-via-ceph-drive-groups)
  - OSDs can be provisioned on support /dev/disk/by-path/pci-HHHH:HH:HH.H devices with colons (`:`)
  - OSDs on PVC can now be encrypted
- Added [admission controller](Documentation/admission-controller-usage.md) support for CRD validations.
  - Support for Ceph CRDs is provided. Some validations for CephClusters are included and additional validations can be added for other CRDs
  - Can be extended to add support for other providers
- OBC changes:
  - Updated lib bucket provisioner version to support multithread and required change can be found in [operator.yaml](cluster/examples/kubernetes/ceph/operator.yaml#L449)
    - Can be extended to add support for other providers
  - Added support for [quota](Documentation/ceph-object-bucket-claim.md#obc-custom-resource), have options for object count and total size.
- CephObjectStore CRD changes:
  - Health displayed in the Status field
  - Supports connecting to external Ceph Rados Gateways, refer to the [external object section](Documentation/ceph-object.html#connect-to-external-object-store)
  - The CephObjectStore CR runs health checks on the object store endpoint, refer to the [health check section](Documentation/ceph-object-store-crd.html#health-settings)
  - The endpoint is now displayed in the Status field
- Prometheus monitoring for external clusters is now possible, refer to the [external cluster section](Documentation/ceph-cluster-crd.html#external-cluster)
- The operator will check for the presence of the `lvm2` package on the host where OSDs will run. If not available, the prepare job will fail. This will prevent issues of OSDs not restarting on node reboot.
- Added a new label "ceph_daemon_type" label to Ceph daemon pods to go alongside the existing "ceph_daemon_id" label.
- The dashboard for the ceph object store will be enabled if the dashboard module is loaded

### EdgeFS

### YugabyteDB

### Cassandra

- Updated Jolokia javaagent from 1.6.0 to 1.6.2 due to CVEs.
- Updated Base image from Alpine 3.8 to 3.12 due to CVEs.

## Breaking Changes

### Ceph

- rbd-mirror daemons that were deployed through the CephCluster CR won't be managed anymore as they have their own CRD now.
To transition, you can inject the new rbd mirror CR with the desired `count` of daemons and delete the previously managed rbd mirror deployments manually.
- old monitoring settings used in the `operator.yaml`: `ROOK_CEPH_STATUS_CHECK_INTERVAL`, `ROOK_MON_HEALTHCHECK_INTERVAL`, `ROOK_MON_OUT_TIMEOUT` are now deprecated.
Backward compatibility is maintained for existing deployments. These settings are now in the `CephCluster` CR, refer to [health check section](Documentation/ceph-cluster-crd.html#health-settings)

## Known Issues

### Ceph

- The Ceph dashboard currently only supports a single object store (RGW) and can only be enabled for the first object store created by Rook. Object stores created after the first will not be able to have the same dashboard view as the first.

## Deprecations

### <Storage Provider>
