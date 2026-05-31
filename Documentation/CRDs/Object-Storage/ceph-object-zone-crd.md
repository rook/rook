---
title: CephObjectZone CRD
---

Rook allows creation of zones in a ceph cluster for a [Ceph Object Multisite](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md)
configuration through a CRD. The following settings are available for Ceph object store zones.

## Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectZone
metadata:
  name: zone-a
  namespace: rook-ceph
spec:
  zoneGroup: zonegroup-a
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: osd
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  customEndpoints:
    - "http://rgw-a.fqdn"
  preservePoolsOnDelete: true
```

## Settings

### Metadata

* `name`: The name of the object zone to create
* `namespace`: The namespace of the Rook cluster where the object zone is created.

### Pools

The pools allow all of the settings defined in the Pool CRD spec. For more details, see the [Pool CRD](../Block-Storage/ceph-block-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

### Spec

* `zonegroup`: The object zonegroup in which the zone will be created. This matches the name of the object zone group CRD.
* `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
* `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.
* `customEndpoints`:  Specify the endpoint(s) that will accept multisite replication traffic for this zone. You may include the port in the definition if necessary. For example: "https://my-object-store.my-domain.net:443". By default, Rook will set this to the DNS name of the ClusterIP Service created for the CephObjectStore that corresponds to this zone.

    Most multisite configurations will not exist within the same Kubernetes cluster, meaning the default value will not be useful. In these cases, you will be required to create your own custom ingress resource for the CephObjectStore in order to make the zone available for replication. You must add the endpoint for your custom ingress resource to this list to allow the store to accept replication traffic.

    In the case of multiple stores (or multiple endpoints for a single store), you are not required to put all endpoints in this list. Only specify the endpoints that should be used for replication traffic.

    If you update `customEndpoints` to return to an empty list, you must the Rook operator to automatically add the CephObjectStore service endpoint to Ceph's internal configuration.

* `preservePoolsOnDelete`: If it is set to 'true' the pools used to support the CephObjectZone will remain when it is deleted. This is a security measure to avoid accidental loss of data. It is set to 'true' by default.

    It is better to check whether data synced with other peer zones before triggering the deletion to avoid accidental loss of data via steps mentioned [here](https://docs.ceph.com/en/latest/radosgw/multisite/#check-synchronization-status)

    When deleting a CephObjectZone, deletion will be blocked until all `CephObjectStores` belonging to the zone are removed.
