# v1.11 Pending Release Notes

## Breaking Changes

- Removed support for MachineDisruptionBudgets, including settings removed from the CephCluster CR:
  - `manageMachineDisruptionBudgets`
  - `machineDisruptionBudgetNamespace`

## Features

- Added setting `requireMsgr2` on the CephCluster CR to allow clusters with a kernel of 5.11 or newer
  to fully communicate with msgr2 and disable the msgr1 port. This allows for more flexibility to enable
  msgr2 features such as encryption and compression on the wire.
- Change `pspEnable` default value to `false` in helm charts, and remove documentation for enabling PSP since the min supported K8s version is 1.21 where PSPs were deprecated.
- [Bucket notifications and topics](https://rook.io/docs/rook/latest/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications/)
  for object stores moved to stable from experimental.
- Introduce [Ceph exporter](https://github.com/rook/rook/blob/master/design/ceph/ceph-exporter.md) as the new source of metrics based on performance counters coming from every Ceph daemon.
- Added support to enable read affinity for RBD volumes. It leverages the [krbd map options](https://docs.ceph.com/en/latest/man/8/rbd/#kernel-rbd-krbd-options) to allow serving reads from an OSD in proximity to the client, according to OSD locations defined in the CRUSH map and topology labels on nodes.
- Update golang version to (v1.19)[https://github.com/rook/rook/pull/11692] and K8s version to (v1.26.1)[https://github.com/rook/rook/pull/11740]
