# Major Themes

v1.8...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

- Flex driver is fully deprecated. If you are still using flex volumes, before upgrading to v1.8
  you will need to convert them to csi volumes. See the flex conversion tool.
- Min supported version of K8s is now 1.16. If running on an older version of K8s it is recommended
  to update to a newer version before updating to Rook v1.8.
- Directory structure of the YAML examples has changed. Files are now in `deploy/examples` and subdirectories.

### Ceph

## Features

- The Rook Operator does not use "tini" as an init process. Instead, it uses the "rook" and handles
  signals on its own.
- Rook adds a finalizer `ceph.rook.io/disaster-protection` to resources critical to the Ceph cluster
  (rook-ceph-mon secrets and configmap) so that the resources will not be accidentally deleted.
- Add support for [Kubernetes Authentication when using HashiCorp Vault Key Management Service](Documentation/ceph-kms.md##kubernetes-based-authentication).
- Bucket notification supported via CRDs
- The Rook Operator and the toolbox now run under the "rook" user and does not use "root" anymore.
- The Operator image now includes the `s5cmd` binary to interact with S3 gateways.
