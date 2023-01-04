# v1.11 Pending Release Notes

## Breaking Changes

- Removed support for MachineDisruptionBudgets, including settings removed from the CephCluster CR:
  - `manageMachineDisruptionBudgets`
  - `machineDisruptionBudgetNamespace`

## Features

- Change `pspEnable` default value to `false` in helm charts.
- [Bucket notifications and topics](https://rook.io/docs/rook/latest/Storage-Configuration/Object-Storage-RGW/ceph-object-bucket-notifications/)
  for object stores moved to stable from experimental.
