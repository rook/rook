# Ceph Config via Ceph Cluster CRD

## Goal

Adding the ability to configure the Ceph config options via the CephCluster CRD.
The goal is not to replace the `rook-config-override` ConfigMap since it is still needed in some scenarios such as if mons need settings applied at first launch.

## Current Solution

Users need to update the `rook-config-override` and add their custom options, e.g., for setting a custom OSD scrubbing schedule ([Ceph OSD Scrubbing config options](https://docs.ceph.com/en/latest/rados/configuration/osd-config-ref/#scrubbing)).

## Proposed Solution

Adding a new structure to the CephCluster CRD under `.spec` named `cephConfig:`.

The `cephConfig` structure will be structured in a way of `Target -> Options` (target being the service to set the options for, e.g., whole cluster `global`, specific OSD `osd.3`).

```yaml
spec:
  # [...]
  cephConfig:
    global:
      osd_max_scrubs: "5"
      # It could be used to set these options for a test cluster (cluster-test.yaml)
      osd_pool_default_size: "1"
      mon_warn_on_pool_no_redundancy: "false"
      bdev_flock_retry: "20"
      bluefs_buffered_io: "false"
      mon_data_avail_warn: "10"
    "osd.3":
      bluestore_cache_autotune: "false"
```

This structure would be equal to a `ceph.conf` like this:

```console
[global]
osd_max_scrubs = 5
osd_pool_default_size = 1
mon_warn_on_pool_no_redundancy = false
bdev_flock_retry = 20
bluefs_buffered_io = false
mon_data_avail_warn = 10

[osd.3]
bluestore_cache_autotune = false
```

The operator will use the Ceph config store (that is accessed via `ceph config assimilate-conf`) to apply the config options to the Ceph cluster (just after the MONs have been created and have formed quorum, before anything else is created).

### Limitations

The operator won't be unsetting any previously set config options or restore config options to their default value.
