---
title: Upgrade
weight: 4920
indent: true
---

# EdgeFS Upgrades
This guide will walk you through the manual steps to upgrade the software in a Rook EdgeFS cluster
from one version to the next. Rook EdgeFS is a multi-cloud distributed software system and
therefore there are multiple components to individually upgrade in the sequence defined in this
guide. After each component is upgraded, it is important to verify that the cluster returns to a
healthy and fully functional state.

We welcome feedback and opening issues!

## Supported Versions
The supported version for this upgrade guide is **from a 1.0 release to a 1.x releases**.
Build-to-build upgrades are not guaranteed to work. This guide is to perform upgrades only between
the official releases.

Upgrades from Alpha to Beta not supported. However, please see migration procedure below.

## EdgeFS Migration
EdgeFS Operator provides a way of preserving data on disks or directories while moving to a
new version (like Alpha to Beta transitioning) or reconfiguring (like full re-start).

Example of migration from `v1alpha1` to `v1beta1`:

1. Delete all EdgeFS services in Kubernetes, e.g., `kubectl delete -f s3.yaml`
2. Delete EdgeFS cluster, e.g., `kubectl delete -f cluster.yaml`
3. Delete EdgeFS operator, e.g., `kubectl delete -f operator.yaml`
4. Edit operator.yaml to transition to a new version. This has to be done for each CustomResourceDefinition in the file.
5. Create EdgeFS operator, e.g., `kubectl create -f operator.yaml`
6. Edit cluster.yaml to transition to a new version. I.e. `edgefs.rook.io/v1alpha1` to `edgefs.rook.io/v1beta1`.
7. If you using devices, edit cluster.yaml and enable devicesResurrectMode "restore" and delete in-use discovery configmaps. This will preserve old cluster data.
8. Create EdgeFS cluster, e.g., `kubectl create -f cluster.yaml`
9. Login to mgr container and check system status, e.g., `efscli system status`
10. Edit EdgeFS services CRD files to transition to a new version. I.e. `edgefs.rook.io/v1alpha1` to `edgefs.rook.io/v1beta1`.
11. Deploy services CRDs, e.g., `kubectl create -f s3.yaml`

## EdgeFS Rolling Upgrade
This feature is coming soon in 1.1 release. Stay tuned!
