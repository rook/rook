---
title: Krew Plugin
---

## Installation

* Install [Krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/)
* Install Krew rook-ceph plugin
  ```console
    kubectl krew install rook-ceph
  ```

## Working

Krew rook-ceph plugin will help in troubleshooting the cluster as by getting the stauts of the working cluster.

### Example:

* Get the ceph status:
  ```console
    kubectl rook-ceph ceph status
  ```

  Output:
  ```console
    cluster:
    id:     a1ac6554-4cc8-4c3b-a8a3-f17f5ec6f529
    health: HEALTH_OK

    services:
    mon: 3 daemons, quorum a,b,c (age 11m)
    mgr: a(active, since 10m)
    mds: 1/1 daemons up, 1 hot standby
    osd: 3 osds: 3 up (since 10m), 3 in (since 8d)

    data:
    volumes: 1/1 healthy
    pools:   6 pools, 137 pgs
    objects: 34 objects, 4.1 KiB
    usage:   58 MiB used, 59 GiB / 59 GiB avail
    pgs:     137 active+clean

    io:
    client:   1.2 KiB/s rd, 2 op/s rd, 0 op/s wr
  ```

Reference: [kubectl-rook-ceph](https://github.com/rook/kubectl-rook-ceph#kubectl-rook-ceph)
