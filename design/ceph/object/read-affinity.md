# Ceph RGW Read Affinity


## Overview
- RGW clients can read data from any replica without any prioritization. This may have cost implications due to cross-zone traffic distribution.
- In order to optimize network performance and reduce data transfer costs:
    - Client read requests should preferably be sent to the RGW daemon that is in the same zone as client. Refer Frontend Data Locality section below.
    - RGW daemon should send the read requests to the nearest possible OSD. Refer Backend Data Locality section below.


## Frontend Data Locality
- Users communicate with the Ceph Object Storage via K8s service that abstracts RGW daemons PODs.
- By default, the traffic is distributed evenly across all the available RGW daemon POD endpoints, regardless of their location relative to the users.
- `TrafficDistribution` can attempt to route traffic to the closest healthy endpoints.

### TrafficDistribution
- With `spec.trafficDistribution.PreferClose`, the endpoints within a zone will receive all the traffic for that zone. If there are no endpoints in a zone, traffic will be distributed to other zones.


### Implementation:

#### `API Changes`
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
        enableTrafficDistribution: true
```
Where:
    - `failureDoman`: is the RGW daemon failure domain. The RGW deployments will be scaled across the failureDomain to ensure that there is at least one RGW daemon pod running on each failure domain.
    - `enableTrafficDistribution`: If set to `true`, then k8's service created for the RGW daemons will have `spec.TrafficDistribution` set to `PerferClose`.

- If `failureDomain` is set to `zone`, then use zone specific deployments to create a separate RGW deployment for each zone.

####  Endpoint Overloading:
- `PreferClose` heuristic will attempt to route traffic to the closest healthy endpoints.
- Endpoints get can get overloaded due to insufficient numbers of endpoints or too many requests. In this case, the RGW daemons on that zone can be scaled using KEDA.


## Backend Data Locality
- RGW Read Affinity can reduce cross-zone traffic by keeping the data access within the respective data centers.
- For example, in a stretch cluster deployed in a multi-zone environment, the read affinity topology implementation helps to keep the traffic within the data center it originated from.
- RGW daemons have the ability to read data from an OSD in promixity to the RGW client, according to the OSD location defined in the CRUSH map and the topology labels on the nodes.
- Following Read Affinity options are available in ceph:
    - `Localize`: Read from OSD that is nearest to the RGW client daemon that has received a GET request.
    - `Balanced`: Read from a random OSD in the Placement Group's active set of OSDs.
    - `Default`: Read from the Primary OSD.
- `Localize` mode should be used in order to avoid cross-zone traffic.

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
    readAffinnity:
        type: localize
        failureDomain: zone
```
Where:
    - `type`: refers to the RGW read Affinity type. It can be `localize`, `balanced` or `default`. Use `Localize` reads to avoid cross-zone traffic.
    - `failureDoman`: is the RGW daemon failure domain. The RGW deployments will be scaled across the failureDomain to ensure that there is at least one RGW daemon pod running on each failure domain.

- Use Zone specific deployments to create a separate RGW deployment for each zone.

#### Set Crush Location
- Crush location should be set on the RGW daemon in order to determine the nearest OSD in `localize` mode.
- Crush location can be specificed by passing  `--crush-location=host=<hostName> zone=<zoneName> root default` argument in in `radosgw` command in the RGW daemon pod.
- Node name for the RGW is daemon is not known until the pod is scheduled. So its not easy to provide the node topolgy labels like  and use them in the `--crush-location` argument.
- There is an k8s KEP to use node topology labels as backward API. Until that is implmmented, we should be relying on some other workaround to pass the labels into the RGW daemon POD container.
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
