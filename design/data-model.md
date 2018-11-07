# Rook Data Model

```
# Operator
The operator manages multiple Rook storage clusters
The operator manages all CRDs for the Rook clusters
One instance of the operator is active
Multiple instances of the operator can be on standby in an HA configuration

# Storage Cluster
The cluster CRD defines desired settings for a storage cluster
All resources for a Rook cluster are created in the same Kubernetes namespace
A cluster has an odd number of mons that form quorum
A cluster has an osd per storage device
A cluster has zero or more pools
A cluster has zero or more block devices
A cluster has zero or more object stores
A cluster has zero or more shared file services

# Pool
The pool CRD defines desired settings for a pool
A pool is created with either replication or erasure coding
Replication can be 1 or more
Erasure coding requires k >= 2 and m >= 1, where k is data chunks and m is coding chunks
Erasure coding specifies a plugin (default=jerasure)
Erasure coding specifies an encoding algorithm (default=reed_sol_van)
A pool can set its failure domain using a CRUSH rule (default=host)

# Object Store
The object store CRD defines desired settings for an object store
An object store has a set of pools dedicated to its instance
Object store metadata pools can specify the same set of pool settings
The object store data pool can specify all pool settings
An object store has a unique set of authorized users
An object store has one or more stateless RGW pods for load balancing
An object store can specify an SSL certificate for secure connections
An object store can specify a port for RGW services (default=53390)
An object store represents a Ceph zone
An object store can be configured for replication from an object store in the same cluster or another cluster

# Shared File System
The file system CRD defines desired settings for a file system
A file system has one MDS service if not partitioned
A file system has multiple MDS services if partitioned
A file system has one metadata pool
A file system has one data pool
```
