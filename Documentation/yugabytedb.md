---
title: YugabyteDB
weight: 900
indent: true
---

# YugabyteDB operator Quikstart

YugaByte DB is a high-performance distributed SQL database (more information [here](https://docs.yugabyte.com/latest/introduction/)). Rook provides an operator that can create and manage YugabyteDB clusters.

## Prerequisites

Follow [these instructions](k8s-pre-reqs.md) to make your kubernetes cluster ready for `Rook`.

## TL;DR

You can create a simple YugabyteDB cluster with below commands. For more detailed instructions, please skip to the [Deploy Rook YugabyteDB Operator](#Deploy-Rook-YugabyteDB-Operator) section.

```console
cd cluster/examples/kubernetes/yugabytedb
kubectl create -f operator.yaml
kubectl create -f cluster.yaml
```

Use below commands to observe the created cluster.

```console
kubectl -n rook-yugabytedb-system get pods
```

## Deploy Rook YugabyteDB Operator

To begin with, deploy the Rook YugabyteDB operator, which can create/manage the YugabyteDB cluster. Use following commands to do the same.

```console
cd cluster/examples/kubernetes/yugabytedb
kubectl create -f operator.yaml
```

Observe the rook operator using below command.

```console
kubectl -n rook-yugabytedb-system get pods
```

## Create a simple YugabyteDB cluster

After the Rook YugabyteDB operator is up and running, you can create an object of the custom resource type `ybclusters.yugabytedb.rook.io`. A sample resource specs are present in `cluster.yaml`. You can also browse/modify the contents of `cluster.yaml` according to the configuration options available. Refer [YugabyteDB CRD documentation](yugabytedb-cluster-crd.md) for details on available configuration options.

To create a YugabyteDB cluster, run

```console
kubectl create -f cluster.yaml
```

Verify the created custom resource object using

```console
kubectl -n rook-yugabytedb get ybclusters.yugabytedb.rook.io
```

Check if the required replicas of Master & TServer are running, run the following command. Tally the Master & TServer pod count against the corresponding replica count you have in `cluster.yaml`. With no change to the replica count, you should see 3 pods each for Master & TServer.

```console
kubectl -n rook-yugabytedb get pods
```

## Troubleshooting

Skip this section, if the cluster is up & running. Continue to the [Access the Database](#access-the-database) section to access `ysql` api.

If the cluster does not come up, first run following command to take a look at operator logs.

```console
kubectl -n rook-yugabytedb-system logs -l app=rook-yugabytedb-operator
```

If everything is OK in the operator logs, check the YugabyteDB Master & TServer logs next.

```console
kubectl -n rook-yugabytedb logs -l app=yb-master-hello-ybdb-cluster
kubectl -n rook-yugabytedb logs -l app=yb-tserver-hello-ybdb-cluster
```

## Access the Database

After all the pods in YugabyteDB cluster are running, you can access the YugabyteDB's postgres compliant `ysql` api. Run following command to access it.

```console
kubectl -n rook-yugabytedb exec -it yb-tserver-hello-ybdb-cluster-0 /home/yugabyte/bin/ysqlsh -- -h yb-tserver-hello-ybdb-cluster-0  --echo-queries
```

Refer the [YugabyteDB documentation](https://docs.yugabyte.com/latest/quick-start/explore-ysql/#kubernetes) for more details on the `ysql` api.

You can also access the YugabyteDB dashboard using port-forwarding.

```console
kubectl port-forward -n rook-yugabytedb svc/yb-master-ui-hello-ybdb-cluster 7000:7000
```

> **NOTE**: You should now be able to navigate to `127.0.0.1:7000` to visualize your cluster.

## Cleanup

Run the commands below to clean up all resources created above.

> **NOTE**: This will destroy your database and delete all of its data.

```console
kubectl delete -f cluster.yaml
kubectl delete -f operator.yaml
```

Manually delete any Persistent Volumes that were created for this YugabyteDB cluster.
