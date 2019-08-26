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


We will do all our work in the EdgeFS example manifests directory.
```sh
cd $YOUR_ROOK_REPO/cluster/examples/kubernetes/edgefs/
```

Example of migration from existing `v1beta1` EdgeFS cluster to stable `v1` EdgeFS cluster:
For already existing EdgeFS manifests we should replace `v1beta1` version to `v1`.
We can use a few simple `sed` commands to do this for all manifests at once.

```sh
sed -i.bak -e "s/edgefs.rook.io\/v1beta1/edgefs.rook.io\/v1beta1/g" *.yaml
sed -i -e "s/edgefs.rook.io\/v1beta1/edgefs.rook.io\/v1/g" *.yaml
mkdir -p backups
mv *.bak backups/
```

Then we should add new `v1` version specification to existing EdgeFS' CRDs.
```sh
kubectl apply -f upgrade-from-v1beta1-create.yaml
```

And replace EdgeFS operator to new stable 'v1' version

```sh
kubectl -n rook-edgefs-system set image deploy/rook-edgefs-operator rook-edgefs-operator=rook/edgefs:v1.1.0
```

Then you could update your cluster's EdgeFS image for latest one as discribed below.

## EdgeFS Version Upgrade

### EdgeFS images
Official EddgeFS container images can be found on [Docker Hub](https://hub.docker.com/r/edgefs/edgefs/tags).

```sh
# Parameterize the environment
export ROOK_SYSTEM_NAMESPACE="rook-edgefs-system"
export CLUSTER_NAME="rook-edgefs"
```

The majority of the upgrade will be handled by the Rook operator. Begin the upgrade by changing the
EdgeFS image field in the cluster CRD (`spec:edgefsImageName`).
```sh
NEW_EDGEFS_IMAGE='edgefs/edgefs:1.2.50'
kubectl -n $CLUSTER_NAME patch Cluster $CLUSTER_NAME --type=merge \
  -p "{\"spec\": {\"edgefsImageName\": \"$NEW_EDGEFS_IMAGE\"}}"
```

or via console editor fix `edgefsImageName` property

```sh
kubectl edit -n $CLUSTER_NAME Cluster $CLUSTER_NAME
```

and save results.

#### 2. Wait for the pod updates to complete
As with upgrading Rook, you must now wait for the upgrade to complete. Determining when the EdgeFS
version has fully updated is rather simple.

```sh
kubectl -n $CLUSTER_NAME describe pods | grep "Image:" | sort | uniq
# This cluster is not yet finished:
#      Image:         edgefs/edgefs:1.2.31
#      Image:         edgefs/edgefs:1.2.50
#      Image:         edgefs/edgefs-restapi:1.2.31
#      Image:         edgefs/edgefs-ui:1.2.31
# This cluster is also finished(all versions are the same):
#      Image:         edgefs/edgefs:1.2.50
#      Image:         edgefs/edgefs-restapi:1.2.50
#      Image:         edgefs/edgefs-ui:1.2.50
```
#### 3. Verify the updated cluster

Access to  EdgeFS mgr pod and check EdgeFS system status

```sh
kubectl exec -it -n $CLUSTER_NAME rook-edgefs-mgr-xxxx-xxx -- toolbox
efscli system status -v 1
```

## EdgeFS Nodes update
Nodes can be added and removed over time by updating the Cluster CRD, for example with `kubectl edit Cluster -n rook-edgefs`.
This will bring up your default text editor and allow you to add and remove storage nodes from the cluster.
This feature is only available when `useAllNodes` has been set to `false` and `resurrect` mode is not used.

### 1. Add node example
#### a. Edit Cluster CRD `kubectl edit Cluster -n rook-edgefs`

#### b. Add new node section with desired configuration in storage section of Cluster CRD

Currently we adding new node `node3072ub16` with two drives `sdb` and `sdc` on it.

```yaml
    - config: null
      devices:
      - FullPath: ""
        config: null
        name: sdb
      - FullPath: ""
        config: null
        name: sdc
      name: node3072ub16
      resources: {}
```
#### c. Save CRD and operator will update all target nodes and related pods of the EdgeFS cluster.

#### d. Login to EdgeFS mgr toolbox and adjust FlexHash table to a new configuration using `efscli system fhtable` command.
