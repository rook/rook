# Managing Compute Resources for rook
## Overview
Resource Constraints allow the rook components to be in specific Kubernetes Quality of Service (QoS) classes. For this the components started by rook need to have resource requests and/or limits set depending on which class the component(s) should be in.

Ceph has recommendations for CPU and memory for each component. The recommendations can be found here: http://docs.ceph.com/docs/master/start/hardware-recommendations/

## Dangers of Resource Constraints
If limits are set too low, this is especially a danger on the memory side. When a Pod runs out of memory because of the limit it is OOM killed, there is a potential of data loss.
The CPU limits would merely limit the "throughput" of the component when reaching it's limit.

## Application of Resource Constraints
The resource constraints are defined in the rook Cluster, Filesystem and RGW CRDs.
The default is to not set resource requirements, this translates to the `qosClasss: BestEffort` (`qosClasss` will be later on explained in [Automatic Algorithm](#automatic-algorithm)).

### Automatic Algorithm
The user is able to enable and disable the automatic resource algorithm as he wants.

This algorithm has a global "governance" class for the Quality of Service (QoS) class to be aimed for.
The key to choose the QoS class is named `qosClasss` and in the `resources` specification.
The QoS classes are:
* `BestEffort` - No `requests` and `limits` are set (for testing).
* `Burstable` - Only `requests` requirements are set.
* `Guaranteed` - `requests` and `limits` are set to be equal.

Additionally to allow the user to simply tune up/down the values without needing to set them a key named `multiplier` in the `resources` specification. This value is used as a multiplier for the calculated values.

#### Special Case: OSD
The OSDs are a special case that need to be covered by the algorithm.
The algorithm needs to take the count of stores (devices and supplied directories) in account to calculate the correct amount of CPU and especially memory usage.

### User defined Override
User defined values **always** overrule automatic calculated values by rook.
The precedence of values is:
1. User defined values
2. (For OSD) per node
3. Global values

**Example**: If you specify a global value for memory for OSDs but set a memory value for a specific node, every OSD except the specific OSD on the node, will get the global value set.

#### Global User Resource Specification
A Kubernetes resource requirement object looks like this:
```yaml
requests:
  cpu: "2"
  memory: "1Gi"
limits:
  cpu: "3"
  memory: "2Gi"
```

The key in the CRDs to set resource requirements is named `resources`.
The following components will allow a resource requirement to be set for:
* `api`
* `agent`
* `mgr`
* `mon`
* `osd`

The `mds` and `rgw` components are configured through the CRD that creates them.
The `mds` are created through the `Filesystem` CRD and the `rgw` through the `ObjectStore` CRD.
There will be a `resources` section added to their respective specification to allow user defined requests/limits.

#### Special Case: OSD
To allow per node/OSD configuration of resource constraints, a key is added to the `storage.nodes` item.
It is named `resources` and contain a resource requirement object (see above).
The `rook-ceph-agent` may be utilized in some way to detect how many stores are used.

### Example
#### Cluster CRD
The requests/limits configured are as an example and not to be used in production.
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  hostNetwork: false
  mon:
    count: 3
    allowMultiplePerNode: false
  placement:
  resources:
    api:
      requests:
          cpu: "500m"
          memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
    mgr:
      requests:
          cpu: "500m"
          memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
    mon:
      requests:
          cpu: "500m"
          memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
    osd:
      requests:
          cpu: "500m"
          memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
  storage:
    useAllNodes: true
    useAllDevices: false
    deviceFilter:
    metadataDevice:
    location:
    storeConfig:
      storeType: bluestore
    nodes:
    - name: "172.17.4.101"
      directories:
      - path: "/rook/storage-dir"
      # resources for the OSD Pod
      resources:
        requests:
          memory: "512Mi"
    - name: "172.17.4.201"
      devices:
      - name: "sdb"
      - name: "sdc"
      storeConfig:
        storeType: bluestore
      # resources for the OSD Pod
      resources:
        requests:
          memory: "512Mi"
          cpu: "1"
        limits:
          cpu: "2"
```
