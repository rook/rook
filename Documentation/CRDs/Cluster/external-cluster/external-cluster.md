---
title: External Storage Cluster
---

An external cluster is a Ceph configuration that is managed outside of the local K8s cluster. The external cluster could be managed by cephadm, or it could be another Rook cluster that is configured to allow the access (usually configured with host networking).

In external mode, Rook will provide the configuration for the CSI driver and other basic resources that allows your applications to connect to Ceph in the external cluster.

## External configuration

* Provider cluster: The cluster providing the data, usually configured by [cephadm](https://docs.ceph.com/en/pacific/cephadm/#cephadm)

* Consumer cluster: The K8s cluster that will be consuming the external provider cluster

## Prerequisites

Create the desired types of storage in the provider Ceph cluster:

* [RBD pools](https://docs.ceph.com/en/latest/rados/operations/pools/#create-a-pool)
* [CephFS filesystem](https://docs.ceph.com/en/latest/cephfs/createfs/)

## Connect the external Ceph Provider cluster to the Rook consumer cluster

1) [Export config from the Provider Ceph cluster](provider-export.md). Configuration must be exported by the Ceph admin, such as a Ceph keyring and mon endpoints, that allows connection to the Ceph cluster.

2) [Import config to the Rook consumer cluster](consumer-import.md). The configuration exported from the Ceph cluster is imported to Rook to provide the needed connection details.

## Advance Options

* [NFS storage](advance-external.md#nfs-storage)

* [Exporting Rook to another cluster](advance-external.md#exporting-rook-to-another-cluster)

* [Run consumer Rook cluster with Admin privileges](advance-external.md#admin-privileges)

* [Connect to an External Object Store](advance-external.md#connect-to-an-external-object-store)

## Upgrades

* [Upgrade external cluster](upgrade-external.md#upgrade-external-cluster)

* [Utilize new features in upgrade](upgrade-external.md#upgrade-cluster-to-utilize-new-feature)
