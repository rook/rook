---
title: Stretch Storage Cluster
---

For environments that only have two failure domains available where data can be replicated, consider
the case where one failure domain is down and the data is still fully available in the
remaining failure domain. To support this scenario, Ceph has integrated support for "stretch" clusters.

Rook requires three zones. Two zones (A and B) will each run all types of Rook pods, which we call the "data" zones.
Two mons run in each of the two data zones, while two replicas of the data are in each zone for a total of four data replicas.
The third zone (arbiter) runs a single mon. No other Rook or Ceph daemons need to be run in the arbiter zone.

For this example, we assume the desired failure domain is a zone. Another failure domain can also be specified with a
known [topology node label](../../CRDs/Cluster/ceph-cluster-crd.md#osd-topology) which is already being used for OSD failure domains.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    # Five mons must be created for stretch mode
    count: 5
    allowMultiplePerNode: false
    stretchCluster:
      failureDomainLabel: topology.kubernetes.io/zone
      subFailureDomain: host
      zones:
      - name: a
        arbiter: true
      - name: b
      - name: c
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.4
    allowUnsupported: true
  # Either storageClassDeviceSets or the storage section can be specified for creating OSDs.
  # This example uses all devices for simplicity.
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter: ""
  # OSD placement is expected to include the non-arbiter zones
  placement:
    osd:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: topology.kubernetes.io/zone
              operator: In
              values:
              - b
              - c
```

For more details, see the [Stretch Cluster design doc](https://github.com/rook/rook/blob/master/design/ceph/ceph-stretch-cluster.md).
