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
