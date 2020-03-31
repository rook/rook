# Major Themes

## Action Required

## Notable Features

- Added K8s 1.17 to the test matrix and removed K8s 1.12 from the test matrix.

### Ceph

- OSD refactor: drop support for Rook legacy OSD, directory OSD and Filestore OSD. For more details refer to the [corresponding issue](https://github.com/rook/rook/issues/4724).
- OSD on PVC now supports a metadata device, [refer to the cluster on PVC section](Documentation/ceph-cluster-crd.html#dedicated-metatada-device) or the [corresponding issue](https://github.com/rook/rook/issues/3852).
- OSD on PVC now supports PVC expansion, if the size of the underlying block increases the Bluestore main block and the overall storage capacity will grow up.
- Ceph Nautilus 14.2.5 is the minimum supported version
- OSD on PVC doesn't use LVM anymore to configure OSD, but solely relies on the entire block device, done [here](https://github.com/rook/rook/pull/4435).
- Specific devices for OSDs can now be specified using the full udev path (e.g. /dev/disk/by-id/ata-ST4000DM004-XXXX) instead of the device name.
- OSD on PVC CRUSH device storage class can now be changed by setting an annotation "crushDeviceClass" on the "data" volume template. See "cluster-on-pvc.yaml" for example.
- Rook will now refuse to create pools with replica size 1 unless `requireSafeReplicaSize` is set to false.
- CSI drivers can now be configured using ["rook-ceph-operator-config"](https://github.com/rook/rook/blob/master/cluster/examples/kubernetes/ceph/operator.yaml) ConfigMap.
ConfigMap can be used in mix with the already existing Env Vars defined in operator deployment manifest. Precedence will be given to ConfigMap in case of conflicting configurations.
- Rook Ceph cleanupPolicy will clean up the dataDirHostPath only after user confirmation. For more info about CleanUpPolicy [read the design](https://github.com/rook/rook/blob/master/design/ceph/ceph-cluster-cleanup.md) as well as the documentation [cleanupPolicy](Documentation/ceph-cluster-crd.md#cluster-settings)
- Rook monitor, mds and osd now have liveness probe checks on their respective sockets
- Pools can now be configured to inline compress the data using the `compressionMode` parameter. Support added [here](https://github.com/rook/rook/pull/5124)
- Ceph OSDs do not use the host PID, but the PID namespace of the pod (more security). The OSD does not see host running processes anymore.
- placement of all the ceph daemons now supports [topologySpreadConstraints](Documentation/ceph-cluster-crd.md#placement-configuration-settings).

### EdgeFS

### YugabyteDB

- Master and TServer pods for YugabyteDB will have resources requests and limits specified as per YugabyteDB recommendations. This will help avoid the soft/hard memory limit issue.

### Cassandra

- Added [JMX Prometheus exporter](https://github.com/prometheus/jmx_exporter) support.

## Breaking Changes

### <Storage Provider>

### Minio
- The minio operator was removed from Rook

## Known Issues

### <Storage Provider>

## Deprecations

### <Storage Provider>
