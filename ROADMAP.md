# Roadmap

This document defines a high level roadmap for Rook development. The dates below are subject to change but should give a general idea of what we are planning. We use the milestone feature in Github so look there for the most up-to-date and issue plan.

## Rook 0.5 (June 2017)

| Platform   | Block  | Object | File   |
|------------|--------|--------|--------|
| Kubernetes | Alpha  | Alpha  | Alpha  |
| Standalone | Alpha  | Alpha  | Alpha  |

 - Rook still in alpha
 - MON and OSD Reliability - survive restarts and pod failures
 - Backup of the cluster config and data required to recover it
 - Publish documentation to rook.io
 - RBD works on all Kubernetes deployments (without any host changes)
 - Upgrade to Ceph Luminous
 - Long-haul testing infra
 - TPRs for object store and file system
 - Initial Rook Upgrade support

## Rook 0.6 (August 2017)

| Platform   | Block   | Object | File   |
|------------|---------|--------|--------|
| Kubernetes | **Beta**| Alpha  | Alpha  |
| Standalone | Alpha   | Alpha  | Alpha  |

 - Rook Block store declared Beta on K8S
 - Kubernetes 1.7 Support
 - Ceph Luminous becomes default drop Kraken
 - Stateful Rook Upgrades supported
 - Improved data placement and pool configuration
 - Performance Testing infra

## Rook 0.7 (October 2017)

| Platform   | Block   | Object | File   |
|------------|---------|--------|--------|
| Kubernetes | **Stable**| **Beta**  | Alpha  |
| Standalone | Alpha   | Alpha  | Alpha  |

 - Rook Object Store declared Beta on K8S
 - Rook Block Store declared Stable on K8S

## Rook 0.8 (December 2017)

| Platform   | Block   | Object | File   |
|------------|---------|--------|--------|
| Kubernetes | **Stable**| **Stable**  | **Beta**  |
| Standalone | Alpha   | Alpha  | Alpha  |

 - Rook Object Storage declared Stable on K8S
 - Rook File System declared Beta on K8S

