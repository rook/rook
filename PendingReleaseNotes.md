# Major Themes

## Action Required

## Known Issues

## Deprecations

## Notable Features

- File system
  - File systems are defined by a CRD and handled by the Operator
  - Multiple file systems can be created, although still experimental in Ceph
  - Multiple data pools can be created
  - Multiple MDS instances can be created per file system
  - An MDS is started in standby mode for each active MDS
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
- Introduces a new flexvolume plugin that will handle all volume attachment requests. This will simplify the deployment by not requiring to have ceph-tools package installed.
- For Kubernetes cluster 1.7.x and older, Kubelet will need to be restarted in order to load the new flexvolume. This has been resolve in K8S 1.8. For more information about the requirements, refer to this [doc](/Documentation/k8s-pre-reqs.md)
- Pool CRD
  - `replication` renamed to `replicated`
  - `erasureCode` renamed to `erasureCoded`
- OSDs
  - OSD pods now require RBAC permissions to create/get/update/delete/list config maps.
  An upgraded operator will create the necessary service account, cluster role, and cluster role bindings to enable this.
