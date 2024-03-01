---
title: Rook
---

# Rook

Rook is an open source **cloud-native storage orchestrator**, providing the platform, framework, and support for
Ceph storage to natively integrate with cloud-native environments.

[Ceph](https://ceph.com/) is a distributed storage system that provides file, block and object storage and is deployed in large scale production clusters.

Rook automates deployment and management of Ceph to provide self-managing, self-scaling, and self-healing storage services.
The Rook operator does this by building on Kubernetes resources to deploy, configure, provision, scale, upgrade, and monitor Ceph.

The Ceph operator was declared stable in December 2018 in the Rook v0.9 release, providing a production storage platform for many years.
Rook is hosted by the [Cloud Native Computing Foundation](https://cncf.io) (CNCF) as a [graduated](https://www.cncf.io/announcements/2020/10/07/cloud-native-computing-foundation-announces-rook-graduation/) level project.

## Quick Start Guide

Starting Ceph in your cluster is as simple as a few `kubectl` commands.
See our [Quickstart](Getting-Started/quickstart.md) guide to get started with the Ceph operator!

## Designs

[Ceph](https://docs.ceph.com/en/latest/) is a highly scalable distributed storage solution for block storage, object storage, and shared filesystems with years of production deployments. See the [Ceph overview](Getting-Started/storage-architecture.md).

For detailed design documentation, see also the [design docs](https://github.com/rook/rook/tree/master/design).

## Need help? Be sure to join the Rook Slack

If you have any questions along the way, don't hesitate to ask in our [Slack channel](https://rook-io.slack.com). Sign up for the Rook Slack [here](https://slack.rook.io).
