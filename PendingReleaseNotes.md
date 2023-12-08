# v1.13 Pending Release Notes

## Breaking Changes

- Removed official support for Kubernetes v1.22
- Removed support for Ceph Pacific (v16)
- Support for the admission controller has been removed. See the
  [Rook upgrade guide](./Documentation/Upgrade/rook-upgrade.md#breaking-changes-in-v113) for more details.

## Features

- Added official support for Kubernetes v1.28
- Added experimental `cephConfig` to CephCluster to allow setting Ceph config options in the Ceph MON config store via the CRD
- CephCSI v3.10.0 is now the default CSI driver version.
  Refer to [Ceph-CSI v3.10.0 Release Notes](https://github.com/ceph/ceph-csi/releases/tag/v3.10.0) for more details.
