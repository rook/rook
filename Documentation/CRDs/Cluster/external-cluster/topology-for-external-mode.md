# Topology-Based Provisioning

## Scenario
Applications like Kafka will have a deployment with multiple running instances. Each service instance will create a new claim and is expected to be located in a different zone. Since the application has its own redundant instances, there is no requirement for redundancy at the data layer. A storage class is created that will provision storage from replica 1 Ceph pools that are located in each of the separate zones.

## Configuration Flags

Add the required flags to the script: `create-external-cluster-resources.py`:

- `--topology-pools`: (optional) Comma-separated list of topology-constrained rbd pools

- `--topology-failure-domain-label`: (optional) K8s cluster failure domain label (example: zone, rack, or host) for the topology-pools that match the ceph domain

- `--topology-failure-domain-values`: (optional) Comma-separated list of the k8s cluster failure domain values corresponding to each of the pools in the `topology-pools` list

The import script will then create a new storage class named `ceph-rbd-topology`.

## Example Configuration

### Ceph cluster

Determine the names of the zones (or other failure domains) in the Ceph CRUSH map where each of the pools will have corresponding CRUSH rules.

Create a zone-specific CRUSH rule for each of the pools. For example, this is a CRUSH rule for `zone-a`:

```
$ ceph osd crush rule create-replicated <rule-name> <root> <failure-domain> <class>
 {
            "rule_id": 5,
            "rule_name": "rule_host-zone-a-hdd",
            "type": 1,
            "steps": [
                {
                    "op": "take",
                    "item": -10,
                    "item_name": "zone-a~hdd"
                },
                {
                    "op": "choose_firstn",
                    "num": 0,
                    "type": "osd"
                },
                {
                    "op": "emit"
                }
            ]
}
```

Create replica-1 pools based on each of the CRUSH rules from the previous step. Each pool must be created with a CRUSH rule to limit the pool to OSDs in a specific zone.

!!! note
   Disable the ceph warning for replica-1 pools: `ceph config set global mon_allow_pool_size_one true`

Determine the zones in the K8s cluster that correspond to each of the pools in the Ceph pool. The K8s nodes require labels as defined with the [OSD Topology labels](../ceph-cluster-crd.md#osd-topology). Some environments already have nodes labeled in zones. Set the topology labels on the nodes if not already present.

Set the flags of the external cluster configuration script based on the pools and failure domains.

--topology-pools=pool-a,pool-b,pool-c

--topology-failure-domain-label=zone

--topology-failure-domain-values=zone-a,zone-b,zone-c

Then run the python script to generate the settings which will be imported to the Rook cluster:
```
 python3 create-external-cluster-resources.py --rbd-data-pool-name replicapool --topology-pools pool-a,pool-b,pool-c --topology-failure-domain-label zone --topology-failure-domain-values zone-a,zone-b,zone-c
```

Output:
```
export ROOK_EXTERNAL_FSID=8f01d842-d4b2-11ee-b43c-0050568fb522
....
....
....
export TOPOLOGY_POOLS=pool-a,pool-b,pool-c
export TOPOLOGY_FAILURE_DOMAIN_LABEL=zone
export TOPOLOGY_FAILURE_DOMAIN_VALUES=zone-a,zone-b,zone-c
```

### Kubernetes Cluster

Check the external cluster is created and connected as per the installation steps.
Review the new storage class:
```
$ kubectl get sc ceph-rbd-topology -o yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  creationTimestamp: "2024-03-07T12:10:19Z"
  name: ceph-rbd-topology
  resourceVersion: "82502"
  uid: 68448a14-3a78-42c5-ac29-261b6c3404af
parameters:
  ...
  ...
  pool: replicapool
  topologyConstrainedPools: |
    [
     {"poolName":"pool-a",
      "domainSegments":[
        {"domainLabel":"zone","value":"zone-a"}]},
     {"poolName":"pool-b",
      "domainSegments":[
        {"domainLabel":"zone","value":"zone-b"}]},
     {"poolName":"pool-c",
      "domainSegments":[
        {"domainLabel":"zone","value":"zone-c"}]},
    ]
provisioner: rook-ceph.rbd.csi.ceph.com
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
```

#### Create a Topology-Based PVC

The topology-based storage class is ready to be consumed! Create a PVC from the `ceph-rbd-topology` storage class above, and watch the OSD usage to see how the data is spread only among the topology-based CRUSH buckets.
