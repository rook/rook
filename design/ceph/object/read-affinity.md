# Ceph RGW Read Affinity


## Overview

- RGW clients can read data from any replica without any prioritization. This may have cost implications in geographyically distributed clsuter due to cross-zone traffic routing.
- In order to optimize network performance and reduce data transfer costs:
    - Client read requests should preferably be sent to the RGW daemon that is in the same zone as the client. Refer [Frontend Data Locality](/design/ceph/object/read-affinity.md/#frontend-data-locality) section below.
    - RGW daemon should send the read requests to the nearest possible OSD. Refer [Backend Data Locality](/design/ceph/object/read-affinity.md/#backend-data-locality) section below.


## Frontend Data Locality

- Users communicate with the Ceph Object Storage via K8s service that abstracts RGW daemons PODs.
- By default, the traffic is routed evenly across all the available RGW daemon POD endpoints, regardless of their location relative to the origin of the request.
- `TrafficDistribution` can attempt to route traffic to the closest healthy endpoint.

### TrafficDistribution

- With `spec.trafficDistribution.PreferClose`, the traffic will be routed to the endpoints within the zone from where the request had originated. If there are no endpoints in that zone then traffic will be routed to other zones.


### Implementation

#### API Changes

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  gateway:
    readAffinnity:
        type: localize
        failureDomain: zone
        service:
            trafficDistribution: PreferClose
```

- `failureDomain`: is the RGW daemon failure domain. The RGW deployments will be scaled across the failureDomain to ensure that there is at least one RGW daemon pod running on each failure domain.
- `service`: defines any additional configurations for the RGW service.
    - `trafficDistribution`: Sets the `TrafficDistribution` in the RGW k8s service. `PreferClose` is the only supported option.

### RGW Deployments
- One RGW deployment per topology domain would be created in order to scale these deployments independently.
- RGW deployment would be created with 1 replica by default.
- `spec.gateway.instances` will control the number of RGW deployments for the cephObjectStore resource. To use TrafficDistribution in a, say, 3 Zone cluster, the user should set the `spec.gateway.instances` to 3.
- The deployment in one locality can be scaled independent of the other localities, either manually or using HPA solution like KEDA.
- If a deployment is scaled then the operator should not override that value.


## Backend Data Locality
- RGW Read Affinity can reduce cross-zone traffic by keeping the data access within the respective data centers.
- For example, in a stretch cluster deployed in a multi-zone environment, the read affinity topology implementation helps to keep the traffic within the data center it originated from.
- RGW daemons have the ability to read data from an OSD in proximity to the RGW client, according to the OSD location defined in the CRUSH map and the topology labels on the nodes.
- Following Read Affinity options are available in ceph:
    - `Localize`: Read from OSD that is nearest to the RGW client daemon that has received a GET request.
    - `Balanced`: Read from a random OSD in the Placement Group's active set of OSDs.
    - `Default`: Read from the Primary OSD.
- `Localize` mode should be used in order to avoid cross-zone traffic routing.

### Implementation:

####  `API Changes`
```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  gateway:
    readAffinity:
        type: localize
        failureDomain: zone
```

- `type`: refers to the RGW read Affinity type. It can be `localize`, `balanced` or `default`. Use `Localize` reads to avoid cross-zone traffic.
- `failureDoman`: is the RGW daemon failure domain. The RGW deployments will be scaled across the failureDomain to ensure that there is at least one RGW daemon pod running on each failure domain.

- Use Zone specific deployments to create a separate RGW deployment for each zone.

#### Set Crush Location
- Crush location should be set on the RGW daemon in order to determine the nearest OSD in `localize` mode.
- Crush location can be specified by passing  `--crush-location=host=<hostName> zone=<zoneName> root default` argument in in `radosgw` command in the RGW daemon pod.
- Node name for the RGW is daemon is not known until the pod is scheduled. So its not easy to provide the node topolgy labels like  and use them in the `--crush-location` argument.
- There is a [KEP](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4742-node-topology-downward-api/README.md) to use node topology labels as downward API. Until that is implemented, we should be relying on some other workaround to pass the labels into the RGW daemon POD container.
- Some available workarounds:
    - `Option 1: topology labels as configmMap`.
        - All the node topology labels can be stored in config map as a json file.
        - ConfigMap can be mounted on the RGW deployment as a file.
        - `jq` utility can be used to parse this json file and set the node topolgy labels as environment variables.
        - The topology labels set as environment variables can be parsed and used in the `--crush-location` argument.
    - `Option 2: Canary pods for RGW daemons`
        - Similar to MONs, canary pods can be created for the RGW deployments to fetch the node details.
        - Delete the canary pods.
        - Create RGW deployments using the node details obtained from the canary pods.



# Risks and Mitigations:

- **Risk**: With `TrafficDistribution:PreferClose`, endpoints in certain locality can get overloaded if the originating traffic is skewed towards that locality.

  **Mitigation**:
    - Emphasize in the documentation that this feature can lead to overload within a locality.
    - Scale the overloaded RGW deployment.
    - Users should be able to opt-out of this feature by disabling TrafficDistribution and `localized` RGW ReadAffinity from the cephCluster spec.
