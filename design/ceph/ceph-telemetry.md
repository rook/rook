# Adding Rook data to Ceph telemetry
The solution plan agreed upon with the telemetry team is for the Rook operator to add telemetry to
the Ceph mon `config-key` database, and Ceph will read each of those items for telemetry retrieval.

## Guidelines from the Ceph telemetry team
- Users must opt in to telemetry
- Users must also opt into the new (delta) telemetry items added between Ceph versions
- Rook should not add information to `config-key` keys that can grow arbitrarily large to keep space
  usage of the mon database low (limited growth is still acceptable)

## Metrics for Rook to collect
Metric names will indicate a hierarchy that can be parsed to add it to Ceph telemetry collection in
a more ordered fashion.

For example `rook/version` and `rook/kubernetes/version` would be put into a structure like shown:
```json
"rook": {
  "version": "vx.y.z"
  "kubernetes": {
    "version": "vX.Y.Z"
  }
}
```

### Overall metrics
- `rook/version` - Rook version.
- `rook/kubernetes/...`
  - `rook/kubernetes/version` - Kubernetes version.
    - Ceph already collects os/kernel versions.
- `rook/csi/...`
  - `rook/csi/version` - Ceph CSI version.

### Node metrics
- `rook/node/count/...` - Node scale information
  - `rook/node/count/kubernetes-total` - Total number of Kubernetes nodes
  - `rook/node/count/with-ceph-daemons` - Number of nodes running Ceph daemons.
    - Since clusters with portable PVCs have one "node" per PVC, this will help show the actual node
      count for Rook installs in portable environments
    - We can get this by counting the number of crash collector pods; however, if any users disable
      the crash collector, we will report `-1` to represent "unknown"
  - `rook/node/count/with-csi-rbd-plugin` - Number of nodes with CSI RBD plugin pods
  - `rook/node/count/with-csi-cephfs-plugin` - Number of nodes with CSI CephFS plugin pods
  - `rook/node/count/with-csi-nfs-plugin` - Number of nodes with CSI NFS plugin pods

### Usage metrics
- `rook/usage/storage-class/...` - Info about storage classes related to the Ceph cluster
  - `rook/usage/storage-class/count/...` - Number of storage classes of a given type
    - `rook/usage/storage-class/count/total` - This is additionally useful in the case of a
      newly-added storage class type not recognized by an older Ceph telemetry version
    - `rook/usage/storage-class/count/rbd`
    - `rook/usage/storage-class/count/cephfs`
    - `rook/usage/storage-class/count/nfs`
    - `rook/usage/storage-class/count/bucket`

### CephCluster metrics
- `rook/cluster/storage/...` - Info about storage configuration
  - `rook/cluster/storage/device-set/...` - Info about storage class device sets
    - `rook/cluster/storage/device-set/count/...` - Number of device sets of given types
      - `rook/cluster/storage/device-set/count/total`
      - `rook/cluster/storage/device-set/count/portable`
      - `rook/cluster/storage/device-set/count/non-portable`
- `rook/cluster/mon/...` - Info about monitors and mon configuration
  - `rook/cluster/mon/count` - The desired mon count
  - `rook/cluster/mon/allow-multiple-per-node` - true/false if allowing multiple mons per node
    - 'true' shouldn't be used in production clusters, so this can give an idea of production count
  - `rook/cluster/mon/max-id` - The highest mon ID, which increases as mons fail over
  - `rook/cluster/mon/pvc/enabled` - true/false whether mons are on PVC
  - `rook/cluster/mon/stretch/enabled` - true/false if mons are in a stretch configuration
- `rook/cluster/network/...`
  - `rook/cluster/network/provider` - The network provider used for the cluster (default, host, multus)
- `rook/cluster/external-mode` - true/false if the cluster is in external mode

## Information Rook is interested in that is already collected by Ceph telemetry
- RBD pools (name is stripped), some config info, and the number of them.
- Ceph Filesystems and MDS info.
- RGW count, zonegroups, zones.
- RBD mirroring info.

## Proposed integration strategy
- Rook, with input from Ceph telemetry, will approve a version of this design doc and treat all
  noted metric names as requirements.
- Ceph will add all of the noted telemetry keys from the config-key database to its telemetry and
  backport to supported major Ceph versions.
  - Ceph code should handle the case where Rook has not set these fields.
- Rook will implement each metric (or related metric group) individually over time.

This strategy will allow Rook time to add telemetry items as it is able without rushing. Because the
telemetry fields will be approved all at once, it will also minimize the coordination that is
required between Ceph and Rook. The Ceph team will not need to create PRs one-to-one with Rook, and
we can limit version mismatch issues as the telemetry is being added.

Future updates will follow a similar pattern where new telemetry is suggested by updates to this
design doc in Rook, then batch-added by Ceph.

Rook will define all telemetry config-keys in a common file to easily understand from code what
telemetry is implemented by a given code version of Rook.

The below one-liner should list each individual metric in this design doc, which can help in
creating Ceph issue trackers for adding Rook telemetry features.
```console
grep -E -o -e '- `rook/.*[^\.]`' design/ceph/ceph-telemetry.md | grep -E -o -e 'rook/.*[^`]'
```

## Rejected metrics
Rejected metrics are included to capture the full discussion, and they can be revisited at any time
with new information or desires.

### Count of each CR Kind
Count of each type of CR: cluster, object, file, object store, mirror, bucket topic,
bucket notification, etc.

This was rejected for version one for a few reasons:
1. In most cases, the Ceph telemetry should be able to report this implicitly via its information.
  - Block pools, filesystems, and object stores can already be inferred easily
1. It would require a config-key for each CR sub-resource, and Rook has many.
1. This could be simpler if we had a json blob for the value, but the Ceph telemetry team has set a
   guideline not to do this.

We can revisit this on a case-by-case basis for specific CRs or features. For example, we may wish
to have ideas about COSI usage when that is available.

### Pod memory/CPU requests/limits
The memory/CPU requests/limits set on Ceph daemon types.

This was rejected for a few reasons:
1. Ceph telemetry already collects general information about OSDs and available space.
1. This would require config-keys for each Ceph daemon, for memory and CPU, and for requests and
   limits, which is a large matrix to provide config keys for.
1. This could be simpler if we had a json blob for the value, but the Ceph telemetry team has set a
   guideline not to do this.
1. This is further exacerbated because OSDs can have different configurations for different storage
   class device sets.

Unless we can provide good reasoning for why this particular metric is valuable, this is likely too
much work for too little benefit.

### Count of cluster-related PVCs/PVs
The number of PVCs/PVs of the different CSI types.

This was rejected primarily because it would require adding new get/list permissions to the Rook
operator which is antithetical to Rook's desires to keep permissions as minimal as possible.
