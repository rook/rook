---
title: Upgrade
weight: 5100
indent: true
---

# Cassandra Operator Upgrades

This guide will walk you through the manual steps to upgrade the software in Cassandra Operator from one version to the next. The cassandra operator is made up of two parts:

1. The `Operator` binary that runs as a standalone application, watches the Cassandra Cluster CRD and makes administrative decisions.
1. A sidecar that runs alongside each member of a Cassandra Cluster. We will call this component `Sidecar`.

Both components should be updated. This is a very manual process at the moment, but it should be automated soon in the future, once the Cassandra Operator reaches the beta stage.

## Considerations

With this upgrade guide, there are a few notes to consider:

* **WARNING**: Upgrading a Rook cluster is not without risk. There may be unexpected issues or
  obstacles that damage the integrity and health of your storage cluster, including data loss. Only
  proceed with this guide if you are comfortable with that. It is recommended that you backup your data before proceeding.
* **WARNING**: The current process to upgrade REQUIRES the cluster to be unavailable for the time of the upgrade.

## Prerequisites

* If you are upgrading from v0.9.2 to a later version, the mount point of the PVC for each member has changed because it was wrong. Please follow the [migration instructions for upgrading from v0.9.2](#before-upgrading-from-v092).
* Before starting the procedure, ensure that your Cassandra Clusters are in a healthy state. You can check a Cassandra Cluster's health by using `kubectl describe clusters.cassandra.rook.io $NAME -n $NAMESPACE` and ensuring that for each rack in the Status, `readyMembers` equals `members`.

## Procedure

1. Because each version of the `Operator` is designed to work with the same version of the `Sidecar`, they must be upgraded together. In order to avoid mixing versions between the `Operator` and `Sidecar`, we first delete every Cassandra Cluster CRD in our Kubernetes cluster, after first backing up their manifests. This will not delete your data because the PVCs will be retained even if the Cassandra Cluster object is deleted. Example:

```console
# Assumes cluster rook-cassandra in namespace rook-cassandra
NAME=rook-cassandra
NAMESPACE=rook-cassandra

kubectl get clusters.cassandra.rook.io $NAME -n $NAMESPACE -o yaml > $NAME.yaml
kubectl delete clusters.cassandra.rook.io $NAME -n $NAMESPACE
```

2. After that, we upgrade the version of the `Operator`. To achieve that, we patch the StatefulSet running the `Operator`:

```console
# Assumes Operator is running in StatefulSet rook-cassandra-operator
# in namespace rook-cassandra-system

kubectl set image sts/rook-cassandra-operator rook-cassandra-operator=rook/cassandra:v0.9.x -n rook-cassandra-system
```

After patching, ensure that the operator pods are running successfully:

```console
kubectl get pods -n rook-cassandra-system
```

3. Recreate the manifests previously deleted:

```console
kubectl apply -f $NAME.yaml
```

The `Operator` will pick up the newly created Cassandra Clusters and recreate them with the correct version of the sidecar.

## Before Upgrading from v0.9.2

Do the following before proceeding:

* For each member of each cluster:

```console
POD=rook-cassandra-us-east-1-us-east-1a-0
NAMESPACE=rook-cassandra

# Change /var/lib/cassandra to /var/lib/scylla for a scylla cluster
kubectl exec $POD -n $NAMESPACE -- /bin/bash

> mkdir /var/lib/cassandra/data/data
> shopt -s extglob
> mv !(/var/lib/cassandra/data) /var/lib/cassandra/data/data
```

After that continue with [the upgrade procedure](#procedure).
