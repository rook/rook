# Roadmap

This document defines a high level roadmap for Rook development. The dates below are subject to change but should give a general idea of what we are planning. We use the [milestone](https://github.com/rook/rook/milestones) feature in Github so look there for the most up-to-date and issue plan.

## Rook 0.7

- Ceph Block Storage (Cluster and Pool CRDs) declared beta
- Mon reliability (restarts, failing over too fast, ip changes, etc.)
- Durability of state (local storage support, config is regenerat-able)
- Run everywhere (rook-agent uses ceph-fuse, nbd-rbd / tcmu runner)
- Adding / removing nodes and disk drives (lifecycle issues, failures, etc.)
- Run with Least Privileged and possibly without privileged containers
- Shutdown / restart issues
- Improved data placement and pool configuration (CRUSH maps)
- Placement group balancer support (ceph-mgr module)
- Use new Prometheus ceph-mgr module
- Performance Testing for Block storage
- Make API and CLI Optional (plan to remove)
- Dynamic Volume Provisioning for CephFS

## Rook 0.8

- Operator runs HA
- Ceph Object storage declared Beta
- Logging (levels, granularity, etc.)
- Support ingress controller for RGW (SSL termination)
- Support for multi-region replication for RGW
- Object storage User CRD
- Automated Upgrade
- CRD validation, cleanup, progress, status (requires engagement with API sig)
- Volume Snapshotting (consider aligning with SIG-storage)
