---
title: CockroachDB
weight: 500
indent: true
---
{% include_relative branch.liquid %}

# CockroachDB Quickstart

CockroachDB is a cloud-native SQL database for building global, scalable cloud services that survive disasters.
Rook provides an operator to deploy and manage CockroachDB clusters.

## Prerequisites

A Kubernetes cluster is necessary to run the Rook CockroachDB operator.
To make sure you have a Kubernetes cluster that is ready for `Rook`, you can [follow these instructions](k8s-pre-reqs.md).

## Deploy CockroachDB Operator

First deploy the Rook CockroachDB operator using the following commands:

```console
git clone --single-branch --branch {{ branchName }} https://github.com/rook/rook.git
cd cluster/examples/kubernetes/cockroachdb
kubectl create -f operator.yaml
```

You can check if the operator is up and running with:

```console
 kubectl -n rook-cockroachdb-system get pod
```

## Create and Initialize CockroachDB Cluster

Now that the operator is running, we can create an instance of a CockroachDB cluster by creating an instance of the `cluster.cockroachdb.rook.io` resource.
Some of that resource's values are configurable, so feel free to browse `cluster.yaml` and tweak the settings to your liking.
Full details for all the configuration options can be found in the [CockroachDB Cluster CRD documentation](cockroachdb-cluster-crd.md).

When you are ready to create a CockroachDB cluster, simply run:

```console
kubectl create -f cluster.yaml
```

We can verify that a Kubernetes object has been created that represents our new CockroachDB cluster with the command below.
This is important because it shows that Rook has successfully extended Kubernetes to make CockroachDB clusters a first class citizen in the Kubernetes cloud-native environment.

```console
kubectl -n rook-cockroachdb get clusters.cockroachdb.rook.io
```

To check if all the desired replicas are running, you should see the same number of entries from the following command as the replica count that was specified in `cluster.yaml`:

```console
kubectl -n rook-cockroachdb get pod -l app=rook-cockroachdb
```

## Accessing the Database

To use the `cockroach sql` client to connect to the database cluster, run the following command in its entirety:

```console
kubectl -n rook-cockroachdb-system exec -it $(kubectl -n rook-cockroachdb-system get pod -l app=rook-cockroachdb-operator -o jsonpath='{.items[0].metadata.name}') -- /cockroach/cockroach sql --insecure --host=cockroachdb-public.rook-cockroachdb
```

This will land you in a prompt where you can begin to run SQL commands directly on the database cluster.

**Example**:

```console
root@cockroachdb-public.rook-cockroachdb:26257/> show databases;
+----------+
| Database |
+----------+
| system   |
| test     |
+----------+
(2 rows)

Time: 2.105065ms
```

## Example App

If you want to run an example application to exercise your new CockroachDB cluster, there is a load generator application in the same directory as the operator and cluster resource files.
The load generator will start writing random key-value pairs to the database cluster, verifying that the cluster is functional and can handle reads and writes.

The rate at which the load generator writes data is configurable, so feel free to tweak the values in `loadgen-kv.yaml`.
Setting `--max-rate=0` will enable the load generator to go as fast as it can, putting a large amount of load onto your database cluster.

To run the load generator example app, simply run:

```console
kubectl create -f loadgen-kv.yaml
```

You can check on the progress and statistics of the load generator by running:

```console
 kubectl -n rook-cockroachdb logs -l app=loadgen
```

To connect to the database and view the data that the load generator has written, run the following command:

```console
kubectl -n rook-cockroachdb-system exec -it $(kubectl -n rook-cockroachdb-system get pod -l app=rook-cockroachdb-operator -o jsonpath='{.items[0].metadata.name}') -- /cockroach/cockroach sql --insecure --host=cockroachdb-public.rook-cockroachdb -d test -e 'select * from kv'
```

## Clean up

To clean up all resources associated with this walk-through, you can run the commands below.

> **NOTE**: that this will destroy your database and delete all of its associated data.

```console
kubectl delete -f loadgen-kv.yaml
kubectl delete -f cluster.yaml
kubectl delete -f operator.yaml
```

## Troubleshooting

If the cluster does not come up, the first step would be to examine the operator's logs:

```console
kubectl -n rook-cockroachdb-system logs -l app=rook-cockroachdb-operator
```

If everything looks OK in the operator logs, you can also look in the logs for one of the CockroachDB instances:

```console
kubectl -n rook-cockroachdb logs rook-cockroachdb-0
```
