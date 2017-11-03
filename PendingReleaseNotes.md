# Major Themes

## Action Required

## Notable Features

- Rook Flexvolume
  - Introduces a new Rook plugin based on [Flexvolume](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md)
  - Integrates with Kubernetes' Volume Controller framework
  - Handles all volume attachment requests such as attaching, mounting and formatting volumes on behalf of Kubernetes
  - Simplifies the deployment by not requiring to have ceph-tools package installed
  - Allows block devices and filesystems to be consumed without any user secret management
  - Improves experience with fencing and volume locking
- Rook-Agents
  - Configured and deployed via Daemonset by Rook-operator
  - Installs the Rook Flexvolume plugin
  - Handles all storage operations required on the node, such as attaching devices, mounting volumes and formatting filesystem.
  - Performs node cleanups during Rook cluster teardown
- File system
  - File systems are defined by a CRD and handled by the Operator
  - Multiple file systems can be created, although still experimental in Ceph
  - Multiple data pools can be created
  - Multiple MDS instances can be created per file system
  - An MDS is started in standby mode for each active MDS
  - Shared filesystems are now supported by the Rook Flexvolume plugin
    - Improved and streamlined experience, now there are no manual steps to copy monitor information or secrets
    - Multiple shared filesystems can be created and consumed within the cluster
    - More information can be found in the [shared filesystems user guides](/Documentation/k8s-filesystem.md#consume-the-file-system)
- Object Store
  - Object Stores are defined by a CRD and handled by the Operator
  - Multiple object stores supported through Ceph realms
- OSDs
  - Bluestore is now the default backend store for OSDs when creating a new Rook cluster.
  - Bluestore can now be used on directories in addition to raw block devices that were already supported.
  - If an OSD loses its metadata and config but still has its data devices, the OSD will automatically regenerate the lost metadata to make the data available again.
- Pools
  - The failure domain for the CRUSH map can be specified on pools with the `failureDomain` property
  - Pools created by file systems or object stores are configurable with all options defined in the pool CRD

## Breaking Changes

- Rook Standalone
  - Standalone mode has been disabled and is no longer supported. A Kubernetes environment is required to run Rook.
- Rook-operator now deploys in `rook-system` namespace
  - If using the example manifest of [rook-operator.yaml](/cluster/examples/kubernetes/rook-operator.yaml), the rook-operator deployment is now changed to deploy in the `rook-system` namespace.
- Rook Flexvolume
  - Persistent volumes from previous releases were created using the RBD plugin. These should be deleted and recreated in order to use the new Flexvolume plugin.
- Pool CRD
  - `replication` renamed to `replicated`
  - `erasureCode` renamed to `erasureCoded`
- OSDs
  - OSD pods now require RBAC permissions to create/get/update/delete/list config maps.
  An upgraded operator will create the necessary service account, role, and role bindings to enable this.
- API
  - The API pod now uses RBAC permissions that are scoped only to the namespace it is running in.
  An upgraded operator will create the necessary service account, role, and role bindings to enable this.
- Filesystem
  - The Rook Flexvolume uses the `mds_namespace` option to specify a cephFS. This is only available on Kernel v4.7 or newer. On older kernel, if there are more than one filesystems in the cluster, the mount operation could be inconsistent. See this [doc](/Documentation/k8s-filesystem.md#kernel-version-requirement).

## Known Issues

- Rook Flexvolume
  - For Kubernetes cluster 1.7.x and older, Kubelet will need to be restarted in order to load the new flexvolume. This has been resolved in K8S 1.8. For more information about the requirements, refer to this [doc](/Documentation/kubernetes.md#restart-kubelet)
  - For Kubernetes cluster 1.6.x, the attacher/detacher controller needs to be disabled in order to load the new Flexvolume. This is caused by a [regression](https://github.com/kubernetes/features/blob/master/release-1.6/release-notes-draft.md#volume) on 1.6.x.  For more information about the requirements, refer to this [doc](Documentation/kubernetes.md#disable-attacher-detacher-controller)
  - For CoreOS and Rancher Kubernetes, the Flexvolume plugin dir will need to be specified to be different than the default. Refer to [Flexvolume configuration pre-reqs](/Documentation/k8s-pre-reqs.md#coreos-container-linux)

## Deprecations

- Rook Standalone
  - Standalone mode has been disabled and is no longer supported. A Kubernetes environment is required to run Rook.
- Rook API
  - Rook has a goal to integrate natively and deeply with container orchestrators such as Kubernetes and using extension points to manage and access the Rook storage cluster. More information can be found in the [tracking issue](https://github.com/rook/rook/issues/704#issuecomment-338738511).
- Rookctl
  - The `rookctl` client is being deprecated.  With the deeper and more native integration of Rook with Kubernetes, `kubectl` now provides a rich Rook management experience on its own.  For direct management of the Ceph storage cluster, the [Rook toolbox](/Documentation/toolbox.md) provides full access to the Ceph tools.
