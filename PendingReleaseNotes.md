# Major Themes

## Action Required

- Existing clusters that are running previous versions of Rook will need to be upgraded/migrated to be compatible with the `v0.8` operator and to begin using the new `rook.io/v1alpha2` and `ceph.rook.io/v1beta1` CRD types.  Please follow the instructions in the [upgrade user guide](Documentation/upgrade.md) to successfully migrate your existing Rook cluster to the new release, as it has been updated with specific steps to help you upgrade to `v0.8`.

## Notable Features

- Rook is now architected to be a general cloud-native storage orchestrator, and can now support multiple types of storage and providers beyond Ceph.
  - [CockroachDB](https://www.cockroachlabs.com/) is now supported by Rook with a new operator to deploy, configure and manage instances of this popular and resilient SQL database.  Databases can be automatically deployed by creating an instance of the new `cluster.cockroachdb.rook.io` custom resource. See the [CockroachDB user guide](Documentation/cockroachdb.md) to get started with CockroachDB.
  - [Minio](https://www.minio.io/) is also supported now with an operator to deploy and manage this popular high performance distributed object storage server.  To get started with Minio using the new `objectstore.minio.rook.io` custom resource, follow the steps in the [Minio user guide](Documentation/minio-object-store.md).
- The status of Rook is no longer published for the project as a whole.  Going forward, status will be published per storage provider or API group.  Full details can be found in the [project status section](./README.md#project-status) of the README.
  - [Ceph](https://ceph.com/) support has graduated to Beta.
- Ceph tools can be run [from any rook pod](Documentation/common-issues.md#ceph-tools).
- Output from stderr will be included in error messages returned from the `exec` of external tools.
- Rook-Operator no longer creates the resources CRD's or TPR's at the runtime. Instead, those resources are provisioned during deployment via `helm` or `kubectl`.
- The 'rook' image is now based on the ceph-container project's 'daemon-base' image so that Rook no
  longer has to manage installs of Ceph in image.
- Rook CRD code generation is now working with BSD (Mac) and GNU sed.
- The [Ceph dashboard](Documentation/ceph-dashboard.md) can be enabled by the cluster CRD.
- `monCount` has been renamed to `count`, which has been moved into the [`mon` spec](Documentation/ceph-cluster-crd.md#mon-settings). Additionally the default if unspecified or `0`, is now `3`.
- You can now toggle if multiple Ceph mons might be placed on one node with the `allowMultiplePerNode` option (default `false`) in the [`mon` spec](Documentation/ceph-cluster-crd.md#mon-settings).
- One OSD will run per pod to increase the reliability and maintainability of the OSDs. No longer will restarting an OSD pod mean that all OSDs on that node will go down. See the [design doc](design/dedicated-osd-pod.md).
- Added `nodeSelector` to Rook Ceph operator Helm chart.
- Ceph is updated to Luminous 12.2.7.
- Ceph OSDs will be automatically updated by the operator when there is a change to the operator version or when the OSD configuration changes. See the [OSD upgrade notes](Documentation/upgrade-patch.md#object-storage-daemons-osds).

## Breaking Changes

- Removed support for Kubernetes 1.6, including the legacy Third Party Resources (TPRs).
- Various paths and resources have changed to accommodate multiple backends:
  - Examples: The yaml files for creating a Ceph cluster can be found in `cluster/examples/kubernetes/ceph`. The yaml files that are backend-independent will still be found in the `cluster/examples/kubernetes` folder.
  - CRDs: The `apiVersion` of the Rook CRDs are now backend-specific, such as `ceph.rook.io/v1beta1` instead of `rook.io/v1alpha1`.
  - Cluster CRD: The Ceph cluster CRD has had several properties restructured for consistency with other backend CRDs that will be coming soon. Rook will automatically upgrade the previous Ceph CRD versions to the new versions with all the compatible properties. When creating the cluster CRD based on the new `ceph.rook.io` apiVersion you will need to take note of the new settings structure.
  - Container images: The container images for Ceph and the toolbox are now `rook/ceph` and `rook/ceph-toolbox`.  The steps in the [upgrade user guide](Documentation/upgrade.md) will automatically start using these new images for your cluster.
  - Namespaces: The example namespaces are now backend-specific. Instead of `rook-system` and `rook`, you will see `rook-ceph-system` and `rook-ceph`.
  - Volume plugins: The dynamic provisioner and flex driver are now based on `ceph.rook.io` instead of `rook.io`
- Ceph container images now use CentOS 7 as a base
- Minimal privileges are configured with a new cluster role for the operator and Ceph daemons, following the new [security design](design/security-model.md).
  - A role binding must be defined for each cluster to be managed by the operator.
- OSD pods are started by a deployment, instead of a daemonset or a replicaset. The new OSD pods will crash loop until the old daemonset or replicasets are removed.

### Removal of the API service and rookctl tool

The [REST API service](https://github.com/rook/rook/issues/1122) has been removed. All cluster configuration is now accomplished through the
[CRDs](https://rook.io/docs/rook/master/crds.html) or with the Ceph tools in the [toolbox](https://rook.io/docs/rook/master/toolbox.html).

The tool `rookctl` has been removed from the toolbox pod. Cluster status and configuration can be queried and changed with the Ceph tools.
Here are some sample commands to help with your transition.

| `rookctl` Command    | Replaced by                                                                                                                       | Description                                         |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `rookctl status`     | `ceph status`                                                                                                                     | Query the status of the storage components          |
| `rookctl block`      | See the [Block storage](Documentation/block.md) and [direct Block](Documentation/direct-tools.md#block-storage-tools) config      | Create, configure, mount, or unmount a block image  |
| `rookctl filesystem` | See the [Filesystem](Documentation/filesystem.md) and [direct File](Documentation/direct-tools.md#shared-filesystem-tools) config | Create, configure, mount, or unmount a file system  |
| `rookctl object`     | See the [Object storage](Documentation/object.md) config                                                                          | Create and configure object stores and object users |

## Known Issues

## Deprecations

- Legacy CRD types in the `rook.io/v1alpha1` API group have been deprecated.  The types from
  `rook.io/v1alpha2` should now be used instead.
- Legacy command flag `public-ipv4` in the ceph components have been deprecated, `public-ip` should now be used instead.
- Legacy command flag `private-ipv4` in the ceph components have been deprecated, `private-ip` should now be used instead.
