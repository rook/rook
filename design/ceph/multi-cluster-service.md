# Multi-cluster Service support for clusters with overlapping networks

## Use case

As a rook-ceph user, I should be able to setup mirroring across multiple clusters with overlapping networks in order to protect my data in case of any disaster.

## Summary

Sometimes users wish to connect two Kubernetes clusters into a single logical cluster. Often, both clusters may have a standardized install with overlapping CIDRs (Service CIDR and/or Pod network CIDR). The Kubernetes "sig-multicluster" community SIG defines a Multi-Cluster Services (MCS) API for providing cross-cluster connectivity from pods to remote services using global IPs.

In order to support Kubernetes clusters connected by an MCS API-compatible application, Rook needs to use "clusterset" IPs instead of the Services' cluster IPs.

## Prerequisite

Peer clusters should be connected using an MCS API compatible application. For example [Submariner Globalnet](https://submariner.io/getting-started/quickstart/)

## Service of OSD pods

For scenarios like RBD mirroring, peers need direct access to Ceph OSDs. Each OSD will have to have a standard ClusterIP Service created for it to allow this. The OSD Service will be created only when multi-cluster networking support is enabled.

The reference implementation used for development of this feature is [Submariner](https://submariner.io/getting-started/quickstart/). In the reference implementation, it is important for Services to be of `type` ClusterIP. Headless Services don't have internal routing between OSDs local to a cluster or any other Ceph daemons local to the cluster.

## API Changes

Provide an option to enable `multiClusterService` in the `cephCluster` CR

``` yaml
spec:
    network:
        ### Enable multiClusterService to export the Services between peer clusters
        multiClusterService:
          enabled: true
```

## Service Export

- `ServiceExport` CR is used to specify which services should be exposed across all the clusters in the cluster set.
- The exported service becomes accessible as `<service>.<ns>.svc.clusterset.local`.
- Create ServiceExport resource for each mon and OSD service.
```yaml
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ServiceExport
  name: <name>
  namespace: rook-ceph
```
- Here, the ServiceExport resource name should be the name of the service that should be exported.

## Implementation Details

- For each mon and OSDs service:
    - Create a corresponding `ServiceExport` resource.
    - Verify that status of the `ServiceExport` should be `true`. Sample Status:
    ```yaml
    Status:
    Conditions:
        Last Transition Time:  2020-12-01T12:35:32Z
        Message:               Awaiting sync of the ServiceImport to the broker
        Reason:                AwaitingSync
        Status:                False
        Type:                  Valid
        Last Transition Time:  2020-12-01T12:35:32Z
        Message:               Service was successfully synced to the broker
        Reason:
        Status:                True
        Type:                  Valid
    ```
    - Obtain the global IP for the exported service by issuing a DNS query to `<service>.<ns>.svc.clusterset.local`.
    - Use this IP in the `--public-addr` flag when creating the mon or OSD deployment.
    - The OSD will bind to the POD IP with the flag `--public-bind-addr`
    - Ensure that mon endpoints configMap has the global IPs.

## Upgrade
- The mons don't work on updating the IPs after they're already running. The monmap remembers the mon Public IP, so if it changes, they will see it as an error state and not respond on the new one.
- If user enables `multiClusterService` on an existing cluster where mons are already using the Cluster IP of the kubernetes service, then the operator should failover each mon to start a new mon.
