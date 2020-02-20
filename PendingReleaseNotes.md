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

### EdgeFS

### YugabyteDB

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
