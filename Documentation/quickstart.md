---
title: Quickstart
weight: 200
---

# Quickstart Guides

Welcome to Rook! We hope you have a great experience installing the Rook **cloud-native storage orchestrator** platform to enable highly available, durable storage
in your Kubernetes cluster.

If you have any questions along the way, please don't hesitate to ask us in our [Slack channel](https://rook-io.slack.com). You can sign up for our Slack [here](https://slack.rook.io).

Rook provides a growing number of storage providers to a Kubernetes cluster, each with its own operator to deploy and manage the resources for the storage provider.

**Follow these guides to get started with each provider**:

| Storage Provider               | Status      | Description                                                                                                                                                                                                          |
| ------------------------------ | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Ceph](ceph-quickstart.md)     | Stable / V1 | Ceph is a highly scalable distributed storage solution for block storage, object storage, and shared filesystems with years of production deployments.                                                               |
| [EdgeFS](edgefs-quickstart.md) | Stable / V1 | EdgeFS is high-performance and fault-tolerant decentralized data fabric with access to object, file, NoSQL and block. |
| [Cassandra](cassandra.md)      | Alpha       | Cassandra is a highly available NoSQL database featuring lightning fast performance, tunable consistency and massive scalability.                                                                                    |
| [CockroachDB](cockroachdb.md)  | Alpha       | CockroachDB is a cloud-native SQL database for building global, scalable cloud services that survive disasters.                                                                                                      |
| [NFS](nfs.md)                  | Alpha       | NFS allows remote hosts to mount filesystems over a network and interact with those filesystems as though they are mounted locally.                                                                                  |
| [YugabyteDB](yugabytedb.md)    | Alpha       | YugaByteDB is a high-performance, cloud-native distributed SQL database which can tolerate disk, node, zone and region failures automatically.                                                                       |
