# Major Themes

## Action Required

- Existing clusters that are running previous versions of Rook will need to be upgraded/migrated to be compatible with the `v0.8` operator and to begin using the new `rook.io/v1alpha2` and `ceph.rook.io/v1alpha1` CRD types.  Please follow the instructions in the [upgrade user guide](Documentation/upgrade.md) to successfully migrate your existing Rook cluster to the new release, as it has been updated with specific steps to help you upgrade to `v0.8`.

## Notable Features

- Rook is now architected to be a general cloud-native storage orchestrator, and can now support multiple types of storage and providers beyond Ceph.  More storage providers such as CockroachDB and Minio will be available in master builds soon.
- Ceph tools can be run [from any rook pod](Documentation/common-issues.md#ceph-tools).
- Output from stderr will be included in error messages returned from the `exec` of external tools
- Rook-Operator no longer creates the resources CRD's or TPR's at the runtime. Instead, those resources are provisioned during deployment via `helm` or `kubectl`.
- The 'rook' image is now based on the ceph-container project's 'daemon-base' image so that Rook no
  longer has to manage installs of Ceph in image.

## Breaking Changes

- Removed support for Kubernetes 1.6, including the legacy Third Party Resources (TPRs).
- Various paths and resources have changed to accommodate multiple backends:
  - Examples: The yaml files for creating a Ceph cluster can be found in `cluster/examples/kubernetes/ceph`. The yaml files that are backend-independent will still be found in the `cluster/examples/kubernetes` folder.
  - CRDs: The `apiVersion` of the Rook CRDs are now backend-specific, such as `ceph.rook.io/v1alpha1` instead of `rook.io/v1alpha1`.
  - Cluster CRD: The Ceph cluster CRD has had several properties restructured for consistency with other backend CRDs that will be coming soon. Rook will automatically upgrade the previous Ceph CRD versions to the new versions with all the compatible properties. When creating the cluster CRD based on the new `ceph.rook.io` apiVersion you will need to take note of the new settings structure.
  - Container images: The container images for Ceph and the toolbox are now `rook/ceph` and `rook/ceph-toolbox`.  The steps in the [upgrade user guide](Documentation/upgrade.md) will automatically start using these new images for your cluster.
  - Namespaces: The example namespaces are now backend-specific. Instead of `rook-system` and `rook`, you will see `rook-ceph-system` and `rook-ceph`.
  - Volume plugins: The dynamic provisioner and flex driver are now based on `ceph.rook.io` instead of `rook.io`
- Ceph container images now use CentOS 7 as a base

### Removal of the API service and rookctl tool

The [REST API service](https://github.com/rook/rook/issues/1122) has been removed. All cluster configuration is now accomplished through the
[CRDs](https://rook.io/docs/rook/master/crds.html) or with the Ceph tools in the [toolbox](https://rook.io/docs/rook/master/toolbox.html).

The tool `rookctl` has been removed from the toolbox pod. Cluster status and configuration can be queried and changed with the Ceph tools.
Here are some sample commands to help with your transition.

 `rookctl` Command | Replaced by | Description
 --- | --- | ---
`rookctl status` | `ceph status` | Query the status of the storage components
`rookctl block` | See the [Block storage](Documentation/block.md) and [direct Block](Documentation/direct-tools.md#block-storage-tools) config | Create, configure, mount, or unmount a block image
`rookctl filesystem` | See the [Filesystem](Documentation/filesystem.md) and [direct File](Documentation/direct-tools.md#shared-filesystem-tools) config | Create, configure, mount, or unmount a file system
`rookctl object` | See the [Object storage](Documentation/object.md) config | Create and configure object stores and object users

## Known Issues

## Deprecations

- Legacy CRD types in the `rook.io/v1alpha1` API group have been deprecated.  The types from
  `rook.io/v1alpha2` should now be used instead.
