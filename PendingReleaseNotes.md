# Major Themes

## Action Required

## Notable Features
- Creation of storage pools through the custom resource definitions (CRDs) now allows users to optionally specify `deviceClass` property to enable
distribution of the data only across the specified device class. See [Ceph Block Pool CRD](Documentation/ceph-pool-crd.md#ceph-block-pool-crd) for
an example usage
- OwnerReferences are created with the fully qualified `apiVersion` such that the references will work properly on OpenShift.

### Ceph

- Rook can now be configured to read "region" and "zone" labels on Kubernetes nodes and use that information as part of the CRUSH location for the OSDs.
- Rgw pods have liveness probe enabled
- Rgw is now configured with the Beast backend as of the Nautilus release
- OSD: newly updated cluster from 0.9 to 1.0.3 and thus Ceph Nautilus will have their OSDs allowing new features for Nautilus
- Rgw instances have their own key and thus are properly reflected in the Ceph status
- The Rook Agent pods are now started when the CephCluster is created rather than immediately when the operator is started.
- Ceph CRUSH tunable are not enforced to "firefly" anymore, Ceph picks the right tunable for its own version, to read more about tunable [see the Ceph documentation](http://docs.ceph.com/docs/master/rados/operations/crush-map/#tunables)

## Breaking Changes

### <Storage Provider>

## Known Issues

### <Storage Provider>

## Deprecations

### Ceph

- For rgw, when deploying an object store with `object.yaml`, using `allNodes` is not supported anymore, a transition path has been implemented in the code though.
So if you were using `allNodes: true`, Rook will replace each daemonset with a deployment (one for one replacement) gradually.
This operation will be triggered on an update or when a new version of the operator is deployed.
Once complete, it is expected that you edit your object CR with `kubectl -n rook-ceph edit cephobjectstore.ceph.rook.io/my-store` and set `allNodes: false` and `instances` with the current number of rgw instances.

### <Storage Provider>
