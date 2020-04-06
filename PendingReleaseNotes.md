# Major Themes

## Action Required

## Notable Features

- Added K8s 1.17 and K8s 1.18 to the test matrix and removed K8s 1.12 from the test matrix.
- Tests are still run against K8s 1.11 in the release branch as the minimum supported version.
- Rook is now built with go modules in golang 1.13

### Ceph

- Ceph Octopus is now supported, with continued support for Nautilus.
   - Ceph Nautilus 14.2.5 is the minimum supported version
   - Support for Mimic was removed. Clusters must be upgraded to Nautilus before upgrading to v1.3.
- OSD refactor: drop support for Rook legacy OSD, directory OSD and Filestore OSD. For more details refer to the [corresponding issue](https://github.com/rook/rook/issues/4724).
- OSD on PVC now supports a metadata device, [refer to the cluster on PVC section](Documentation/ceph-cluster-crd.html#dedicated-metatada-device) or the [corresponding issue](https://github.com/rook/rook/issues/3852).
- OSD on PVC now supports PVC expansion, if the size of the underlying block increases the Bluestore main block and the overall storage capacity will grow up.
- OSD on PVC doesn't use LVM anymore to configure OSD, but solely relies on the entire block device, done [here](https://github.com/rook/rook/pull/4435).
- Specific devices for OSDs can now be specified using the full udev path (e.g. /dev/disk/by-id/ata-ST4000DM004-XXXX) instead of the device name.
- OSD on PVC CRUSH device storage class can now be changed by setting an annotation "crushDeviceClass" on the "data" volume template. See "cluster-on-pvc.yaml" for example.
- Rook will now refuse to create pools with replica size 1 unless `requireSafeReplicaSize` is set to false.
- CSI drivers can now be configured using ["rook-ceph-operator-config"](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml) ConfigMap.
ConfigMap can be used in mix with the already existing Env Vars defined in operator deployment manifest. Precedence will be given to ConfigMap in case of conflicting configurations.
- Rook Ceph cleanupPolicy will clean up the dataDirHostPath only after user confirmation. For more info about CleanUpPolicy [read the design](https://github.com/rook/rook/blob/master/design/ceph/ceph-cluster-cleanup.md) as well as the documentation [cleanupPolicy](Documentation/ceph-cluster-crd.md#cluster-settings)
- Rook monitor, mds and osd now have liveness probe checks on their respective sockets
- Most of the operator's controller implementations are now based on the controller runtime. No impact to user clusters is expected.
- Object stores can be created without specifying pools in the CephObjectStore CR to allow for pool management outside of Rook such as in the dashboard.
- Operator logs are more concise with the Ceph commands only printing at debug level.
- Various improvements to the integration tests including tests for an external cluster and for a cluster running on PVCs.
- Pools can now be configured to inline compress the data using the `compressionMode` parameter. Support added [here](https://github.com/rook/rook/pull/5124)
- Ceph OSDs in Octopus do not use the host PID namespace, but the PID namespace of the pod (more security). The OSD does not see running host processes anymore.
- placement of all the ceph daemons now supports [topologySpreadConstraints](Documentation/ceph-cluster-crd.md#placement-configuration-settings).
- Rook is now capable of working with Multus to expose dedicated interfaces to pods, for more information please refer to the [network configuration doc](Documentation/ceph-cluster-crd.html#network-configuration-settings).

### EdgeFS

- Enhanced support for Time zones, now geo-distributed name space can be properly identified in case of Kubernetes stretched clusters.
- Added support for Microsoft Windows CIFS/SMB CRD. Now, it is possible to orchestrate Windows CIFS/SMB Scale-Out service managed by [Rook SMB CRD](Documentation/edgefs-smb-crd.md).

### YugabyteDB

- Master and TServer pods for YugabyteDB will have resources requests and limits specified as per YugabyteDB recommendations. This will help avoid the soft/hard memory limit issue.

### Cassandra

- Added [JMX Prometheus exporter](https://github.com/prometheus/jmx_exporter) support.

## Breaking Changes

### Ceph
- Support for Mimic was removed
- Support for filestore (directory-based) OSDs was removed. See the upgrade guide for migration instructions.
- Support for OSDs created in Rook v0.9 or earlier was removed (created with Rook partitions instead of ceph-volume). See the upgrade guide for migration instructions.

### Minio
- The minio operator was removed from Rook

## Known Issues

### <Storage Provider>

## Deprecations

### <Storage Provider>
