# Rook and External Ceph Clusters

Target version: 1.1

Rook was designed for storage consumption in the same Kubernetes cluster as the clients who are consuming the storage. However, this scenario is not always sufficient.

Another common scenario is when Ceph is running in an "external" cluster from the clients. There are a number of reasons for this scenario:
- Centralized Ceph management in a single cluster with multiple Kubernetes clusters that need to consume storage.
- Customers already have a Ceph cluster running not in a K8s environment, likely deployed with Ansible, ceph-deploy, or even manually. They should be able to consume this storage from Kubernetes.
- Fully independent storage for another level of isolation from their K8s compute nodes. This scenario can technically also be accomplished in a single Kubernetes cluster through labels, taints, and tolerations.

## Terminology

| | |
|---|---|
| **Local** Cluster  | The cluster where clients are running that have a need to connect to the Ceph storage. Must be a Kubernetes/OpenShift cluster.  |
| **External** Cluster  | The cluster where Ceph Mons, Mgr, OSDs, and MDS are running, which might have been deployed with Rook, Ansible, or any other method.  |

## Requirements

Requirements for clients in the local cluster to connect to the external cluster include:
- At least one mon endpoint where the connection to the cluster can be established
- Admin keyring for managing the cluster
- Network connectivity from a local cluster to the mons, mgr, osds, and mds of the external cluster:
  - mon: for the operator to watch the mons that are in quorum
  - mon/osd: for client access
  - mgr: for dashboard access
  - mds: for shared filesystem access

## Rook Ceph Operator

When the Rook operator is started, initially it is not aware of any clusters. When the admin creates the operator, they will want to configure the operator differently depending on if they want to configure a local Rook cluster, or an external cluster.

If external cluster management is required, the differences are:
- The Rook Discover DaemonSet would not be necessary. Its purpose is to detect local devices, which is only needed for OSD configuration.
   - Side note: If a local cluster, the discover DaemonSet could be delayed starting until the first cluster is started. There is no need for the discovery until the first cluster is created.
- The Security Context Constraints (SCC) would not require all the privileges of a local cluster. These privileges are only required by mon and/or osd daemon pods, which are not running in the local cluster.
  - `allowPrivilegedContainer`
  - `allowHostDirVolumePlugin`
  - `allowHostPID`
  - `allowHostIPC`
  - `allowHostPorts`

## CSI Driver

The CSI driver is agnostic of whether Ceph is running locally or externally. The core requirement of the CSI driver is the list of mons and the keyring with which to connect. This metadata is required whether the cluster is local or external. The Rook operator will need to keep this metadata updated throughout the lifetime of the CSI driver.

The CSI driver will be installed and configured by the Rook operator, similarly to any Rook cluster. The advantages of this approach instead of a standalone ceph-csi for external clusters include:
- Provide a consistent experience across any Kubernetes/OpenShift deployment
- Rook can install, configure, and update the Ceph CSI driver. Admins don't have to worry about the CSI driver.

Question: How would Rook behave in the case where the admin deployed ceph-csi standalone as well as Rook? It seems reasonable not to support this, although it's not clear if there would actually be conflicts between the two.

The flex driver would also be agnostic of the cluster for the same reasons, but we won’t need to worry about the flex driver going forward.

## Rook-Ceph Cluster CRD

In order for Rook to provide the storage to clients in the local cluster, the CephCluster CRD will be created in order for the operator to provide local management of the external cluster. There are several differences needed for the operator to be aware of an external cluster.

1. Before the CephCluster CRD is created, some metadata must be initialized in local configmaps/secrets to allow the local cluster to manage the external cluster.
   * mon endpoint(s) and admin keyring
1. The mon, mgr, and osd daemons will not be managed by the local Rook operator. These daemons must be created and managed by the external cluster.
1. The operator will make a "best effort" to keep the list of mons updated.
   * If the mons change in the external cluster, the list of mons must be updated in the local cluster.
   * The operator will need to query the Ceph status periodically (perhaps every minute). If there is a change to the mons, the operator will update the local configmaps/secrets.\
   * If the local operator fails to see changes to the external mons, perhaps because it is down, the mon list could become stale. In that case, the admin will need to update the list similarly to how it was initialized when the local cluster was first created.
   * The operator will update the cluster crd with the following status fields:
      - Timestamp of the last successful time querying the mons
      - Timestamp of the last attempt to query the mons
      - Success/Failure message indicating the result of the last check

The first bullet point above requires an extra manual configuration step by the cluster admin from what they need in a typical Rook cluster. The other items above will be handled automatically by the Rook operator. The extra step involves exporting metadata from the external cluster and importing it to the local cluster:
1. The admin creates a yaml file with the needed resources from the external cluster (ideally we would provide a helper script to help automate this task):
   * Save the mon list and admin keyring
1. Load the yaml file into the local cluster
   * `kubectl create -f <config.yaml>`

The CephCluster CRD will have a new property "external" to indicate whether the cluster is external. If true, the local operator will implement the described behavior.
Other CRDs such as CephBlockPool, CephFilesystem, and CephObjectStore do not need this property since they all belong to the cluster and will effectively
inherit the external property.

```yaml
kind: CephCluster
spec:
  external: true
```

The mgr modules, including the dashboard, would be running in the external cluster. Any configuration that happens through the dashboard would depend on the orchestration modules in that external cluster.

## Block Storage

With the rook-ceph cluster created, the CSI driver integration will cover the Block (RWO) storage and no additional management is needed.

### Pool

When a pool CRD is created in the local cluster, the operator will create the pool in the external cluster. The pool settings will only be applied the first
time the pool is created and should be skipped thereafter. The ownership and lifetime of the pool will belong to the external cluster.
The local cluster should not apply pool settings to overwrite the settings defined in the external cluster.

If the pool CRD is deleted from the local cluster, the pool will not be deleted in the external cluster.

## Filesystem (MDS)

A shared filesystem must only be created in the external cluster. Clients in the local cluster can connect to the MDS daemons in the external cluster.

The same instance of CephFS cannot have MDS daemons in different clusters. The MDS daemons must exist in the same cluster for a given filesystem.
When the CephFilesystem CRD is created in the local cluster, Rook will ignore the request and print an error to the log.

## Object Storage (RGW)

An object store can be created that will start RGW daemons in the local cluster.
When the CephObjectStore CRD is created in the local cluster, the local Rook operator does the following:
1. Create the metadata and data pools in the external cluster (if they don't exist yet)
1. Create a realm, zone, and zone group in the external cluster (if they don't exist yet)
1. Start the RGW daemon in the local cluster
1. Local s3 clients will connect to the local RGW endpoints

Question: Should we generate a unique name so an object store of the same name cannot be shared with the external cluster? Or should we allow
sharing of the object store between the two clusters if the CRD has the same name? If the admin wants to create independent object stores,
they could simply create them with unique CRD names.

Assuming the object store can be shared with the external cluster, similarly to pools, the owner of the object store is the external cluster.
If the local cluster attempts to change the pool settings such as replication, they will be ignored.

## Monitoring (prometheus)

Rook already creates and injects service monitoring configuration, consuming what the ceph-mgr prometheus exporter module generates.
This enables the capability of a Kubernetes cluster to gather metrics from the external cluster and feed them in Prometheus.

The idea is to allow Rook-Ceph to connect to an external ceph-mgr prometheus module exporter.

1. Enhance external cluster script:
    1. the script tries to discover the list of managers IP addresses
    2. if provided by the user, the list of ceph-mgr IPs in the script is accepted via the new `--prometheus-exporter-endpoint` flag

2. Add a new entry in the monitoring spec of the `CephCluster` CR:

```go
// ExternalMgrEndpoints point to existing Ceph prometheus exporter endpoints
ExternalMgrEndpoints []v1.EndpointAddress `json:"externalMgrEndpoints,omitempty"`
}
```

So the CephCluster CR will look like:

```yaml
monitoring:
  # requires Prometheus to be pre-installed
  enabled: true
  externalMgrEndpoints:
    - ip: "192.168.0.2"
    - ip: "192.168.0.3"
```

3. Configure monitoring as part of `configureExternalCephCluster()` method

4. Create a new metric Service

5. Create an Endpoint resource based out of the IP addresses either discovered or provided by the user in the script
