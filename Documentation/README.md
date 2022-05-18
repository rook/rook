---
title: Rook
---

# Rook

Rook is an open source **cloud-native storage orchestrator**, providing the platform, framework, and support for a diverse set of storage solutions to natively integrate with cloud-native environments.

Rook turns storage software into self-managing, self-scaling, and self-healing storage services. It does this by automating deployment, bootstrapping, configuration, provisioning, scaling, upgrading, migration, disaster recovery, monitoring, and resource management. Rook uses the facilities provided by the underlying cloud-native container management, scheduling and orchestration platform to perform its duties.

Rook integrates deeply into cloud native environments leveraging extension points and providing a seamless experience for scheduling, lifecycle management, resource management, security, monitoring, and user experience.

The Ceph operator was declared stable in December 2018 in the Rook v0.9 release, providing a production storage platform for several years already.

## Quick Start Guide

Starting Ceph in your cluster is as simple as a few `kubectl` commands.
See our [Quickstart](quickstart.md) guide to get started with the Ceph operator!

## Designs

[Ceph](https://docs.ceph.com/en/latest/) is a highly scalable distributed storage solution for block storage, object storage, and shared filesystems with years of production deployments. See the [Ceph overview](storage-architecture.md).

For detailed design documentation, see also the [design docs](https://github.com/rook/rook/tree/master/design).

## Need help? Be sure to join the Rook Slack

If you have any questions along the way, please don't hesitate to ask us in our [Slack channel](https://rook-io.slack.com). You can sign up for our Slack [here](https://slack.rook.io).
