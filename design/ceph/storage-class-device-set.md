# Rook StorageClassDeviceSets

**Target version**: Rook 1.1

## Background

The primary motivation for this feature is to take advantage of the mobility of
storage in cloud-based environments by defining storage based on sets of devices
consumed as block-mode PVCs. In environments like AWS you can request remote
storage that can be dynamically provisioned and attached to any node in a given
Availability Zone (AZ). The design can also accommodate non-cloud environments
through the use of local PVs.

## StorageClassDeviceSet struct

```go
struct StorageClassDeviceSet {
    Name                 string // A unique identifier for the set
    Count                int // Number of devices in this set
    Resources            v1.ResourceRequirements // Requests/limits for the devices
    Placement            rook.Placement // Placement constraints for the devices
    Config               map[string]string // Provider-specific device configuration
    volumeClaimTemplates []v1.PersistentVolumeClaim // List of PVC templates for the underlying storage devices
}
```

A provider will be able to use the `StorageClassDeviceSet` struct to describe
the properties of a particular set of `StorageClassDevices`. In this design, the
notion of a "`StorageClassDevice`" is an abstract concept, separate from
underlying storage devices. There are three main aspects to this abstraction:

1. A `StorageClassDevice` is both storage and the software required to make the
storage available and manage it in the cluster. As such, the struct takes into
account the resources required to run the associated software, if any.
1. A single `StorageClassDevice` could be comprised of multiple underlying
storage devices, specified by having more than one item in the
`volumeClaimTemplates` field.
1. Since any storage devices that are part of a `StorageClassDevice` will be
represented by block-mode PVCs, they will need to be associated with a Pod so
that they can be attached to cluster nodes.

A `StorageClassDeviceSet` will have the following fields associated with it:

* **name**: A name for the set. **[required]**
* **count**: The number of devices in the set. **[required]**
* **resources**: The CPU and RAM requests/limits for the devices. Default is no
  resource requests.
* **placement**: The placement criteria for the devices. Default is no
  placement criteria.
* **config**: Granular device configuration. This is a generic
  `map[string]string` to allow for provider-specific configuration.
* **volumeClaimTemplates**: A list of PVC templates to use for provisioning the
  underlying storage devices.

An entry in `volumeClaimTemplates` must specify the following fields:
  * **resources.requests.storage**: The desired capacity for the underlying
  storage devices.
  * **storageClassName**: The StorageClass to provision PVCs from. Default would
  be to use the cluster-default StorageClass.

## Example Workflow: rook-ceph OSDs

The CephCluster CRD could be extended to include a new field:
`spec.storage.StorageClassDeviceSets`, which would be a list of one or more
`StorageClassDeviceSets`. If elements exist in this list, the CephCluster
controller would then create enough PVCs to match each `StorageClassDeviceSet`'s
`Count` field, attach them to individual OsdPrepare Jobs, then attach them to
OSD Pods once the Jobs are completed. For the initial implementation, only one
entry in `volumeClaimTemplates` would be supported, if only to tighten the scope
for an MVP.

The PVCs would be provisioned against a configured or default StorageClass. It
is recommended that the admin setup a StorageClass with `volumeBindingMode:
WaitForFirstConsumer` set.

If the admin wishes to control device placement, it will be up to them to make
sure the desired nodes are labeled properly to ensure the Kubernetes scheduler
will distribute the OSD Pods based on Placement criteria.

In keeping with current Rook-Ceph patterns, the **resources** and **placement**
for the OSDs specified in the `StorageClassDeviceSet` would override any
cluster-wide configurations for OSDs. Additionally, other conflicting
configurations parameters in the CephCluster CRD,such as `useAllDevices`, will
be ignored by device sets.

### OSD Deployment Behavior

While the current strategy of deploying OSD Pods as individual Kubernetes
Deployments, some changes to the deployment logic would need to change. The
workflow would look something like this:

1. Get all matching OSD PVCs
1. Create any missing OSD PVCs
1. Get all matching OSD Deployments
1. Check that all OSD Deployments are using valid OSD PVCs
    * If not, probably remove the OSD Deployment?
    * Remove any PVCs used by OSD Deployments from the list of PVCs to be
    worked on
1. Run an OsdPrepare Job on all unused and uninitialized PVCs
    * This would be one Job per PVC
1. Create an OSD Deployment for each unused but initialized PVC
    * Deploy OSD with `ceph-volume` if available.
       * If PV is not backed by LV, create a LV in this PV.
       * If PV is backed by LV, use this PV as is.

### Additional considerations for local storage

This design can also be applied to non-cloud environments. To take advantage of
this, the admin should deploy the
[sig-storage-local-static-provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner)
to create local PVs for the desired local storage devices and then follow the
recommended directions for [local
PVs](https://kubernetes.io/blog/2019/04/04/kubernetes-1.14-local-persistent-volumes-ga/).

### Possible Implementation: DriveGroups

Creating real-world OSDs is complex:

* Some configurations deploy multiple OSDs on a single drive
* Some configurations are using more than one drive for a single OSD.
* Deployments often look similar on multiple hosts.
* There are some advanced configurations possible, like encrypted drives.

All of these setups are valid real-world configurations that need to be
supported.

The Ceph project defines a data structure that allows defining groups of drives
to be provisioned in a specific way by ceph-volume: Drive Groups. Drive Groups
were originally designed to be ephemeral, but it turns out that orchestrators
like DeepSea store them permanently in order to have a source of truth when
(re-)provisioning OSDs. Also, Drive Groups were originally designed to be host
specific. But the concept of hosts is not really required for the Drive Group
data structure make sense, as they only select a subset of a set of aviailble
drives.

DeepSea has a documentation of [some example drive
groups](https://github.com/SUSE/DeepSea/wiki/Drive-Groups#example-drive-group-files).
A complete specification is documented in the [ceph
documentation](http://docs.ceph.com/docs/master/mgr/orchestrator_modules/#orchestrator.DriveGroupSpec).


#### DeviceSet vs DriveGroup

A DeviceSet to provision 8 OSDs on 8 drives could look like:

```yaml
name: "my_device_set"
count: 8
```

The Drive Group would look like so:

```yaml
host_pattern: "hostname1"
data_devices:
  count: 8
```

A Drive Group with 8 OSDs using a shared fast drive could look similar to this:

```yaml
host_pattern: "hostname1"
data_devices:
  count: 8
  model: MC-55-44-XZ
db_devices:
  model: NVME
db_slots: 8
```
#### ResourceRequirements and Placement

Drive Groups don't yet provide orchestrator specific extensions, like resource
requirements or placement specs, but that could be added trivially. Also a name
could be added to Drive Groups.

### OSD Configuration Examples

Given the complexity of this design, here are a few examples to showcase
possible configurations for OSD `StorageClassDeviceSets`.

#### Example 1: AWS cross-AZ

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  storage:
    ...
    storageClassDeviceSets:
    - name: cluster1-set1
      count: 3
      resources:
        requests:
          cpu: 2
          memory: 4Gi
      placement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
              topologyKey: "failure-domain.beta.kubernetes.io/zone"
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: gp2-ebs
```

In this example, `podAntiAffinity` is used to spread the OSD Pods across as many
AZs as possible. In addition, all Pods would have to be given the label
`rook.io/cluster=cluster1` to denote they belong to this cluster, such that the
scheduler will know to try and not schedule multiple Pods with that label on the
same nodes if possible. The CPU and memory requests would allow the scheduler to
know if a given node can support running an OSD process.

It should be noted, in the case where the only nodes that can run a new OSD Pod
are nodes with OSD Pods already on them, one of those nodes would be
selected. In addition, EBS volumes may not cross between AZs once created, so a
given Pod is guaranteed to always be limited to the AZ it was created in.

#### Example 2: Single AZ

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  resources:
    osd:
      requests:
        cpu: 2
        memory: 4Gi
  storage:
    ...
    storageClassDeviceSet:
    - name: cluster1-set1
      count: 3
      placement:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: "failure-domain.beta.kubernetes.io/zone"
                operator: In
                values:
                - us-west-1a
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: gp2-ebs
    - name: cluster1-set2
      count: 3
      placement:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: "failure-domain.beta.kubernetes.io/zone"
                operator: In
                values:
                - us-west-1b
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: gp2-ebs
    - name: cluster1-set3
      count: 3
      placement:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: "failure-domain.beta.kubernetes.io/zone"
                operator: In
                values:
                - us-west-1c
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: gp2-ebs
```

In this example, we've added a `nodeAffinity` to the `placement` that restricts
all OSD Pods to a specific AZ. This case is only really useful if you specify
multiple `StorageClassDeviceSets` for different AZs, so that has been done here.
We also specify a top-level `resources` definition, since we want that to be the
same for all OSDs in the device sets.

#### Example 3: Different resource needs

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  storage:
    ...
    storageClassDeviceSets:
    - name: cluster1-set1
      count: 3
      resources:
        requests:
          cpu: 2
          memory: 4Gi
      placement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
              topologyKey: "failure-domain.beta.kubernetes.io/zone"
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: gp2-ebs
    - name: cluster1-set2
      count: 3
      resources:
        requests:
          cpu: 2
          memory: 8Gi
      placement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
              topologyKey: "failure-domain.beta.kubernetes.io/zone"
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 10Ti
          storageClassName: gp2-ebs
```

In this example, we have two `StorageClassDeviceSets` with different capacities
for the devices in each set. The devices with larger capacities would require a
greater amount of memory to operate the OSD Pods, so that is reflected in the
`resources` field.

#### Example 4: Simple local storage

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  storage:
    ...
    storageClassDeviceSet:
    - name: cluster1-set1
      count: 3
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 1
          storageClassName: cluster1-local-storage
```

In this example, we expect there to be nodes that are configured with one local
storage device each, but they would not be specified in the `nodes` list. Prior
to this, the admin would have had to deploy the local-storage-provisioner,
created local PVs for each of the devices, and created a StorageClass to allow
binding of the PVs to PVCs. At this point, the same workflow would be the same
as the cloud use case, where you simply specify a count of devices, and a
template with a StorageClass. Two notes here:

1. The count of devices would need to match the number of existing devices to
consume them all.
1. The capacity for each device is irrelevant, since we will simply consume the
entire storage device and get that capacity regardless of what is set for the
PVC.

#### Example 5: Multiple devices per OSD

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  storage:
    ...
    storageClassDeviceSet:
    - name: cluster1-set1
      count: 3
      config:
        metadataDevice: "/dev/rook/device1"
      volumeClaimTemplates:
      - metadata:
          name: osd-data
        spec:
          resources:
            requests:
              storage: 1
          storageClassName: cluster1-hdd-storage
      - metadata:
          name: osd-metadata
        spec:
          resources:
            requests:
              storage: 1
          storageClassName: cluster1-nvme-storage
```

In this example, we are using NVMe devices to store OSD metadata while having
HDDs store the actual data. We do this by creating two StorageClasses, one for
the NVMe devices and one for the HDDs. Then, if we assume our implementation
will always provide the block devices in a deterministic manner, we specify the
location of the NVMe devices (as seen in the container) as the `metadataDevice`
in the OSD config. We can guarantee that a given OSD Pod will always select two
devices that are on the same node if we configure `volumeBindingMode:
WaitForFirstConsumer` in the StorageClasses, as that allows us to offload that
logic to the Kubernetes scheduler. Finally, we also provide a `name` field for
each device set, which can be used to identify which set a given PVC belongs to.

#### Example 6: Additional OSD configuration

```yaml
type: CephCluster
name: cluster1
...
spec:
  ...
  storage:
    ...
    storageClassDeviceSet:
    - name: cluster1-set1
      count: 3
      config:
        osdsPerDevice: "3"
      volumeClaimTemplates:
      - spec:
          resources:
            requests:
              storage: 5Ti
          storageClassName: cluster1-local-storage
```

In this example, we show how we can provide additional OSD configuration in the
`StorageClassDeviceSet`. The `config` field is just a `map[string]string` type,
so anything can go in this field.
