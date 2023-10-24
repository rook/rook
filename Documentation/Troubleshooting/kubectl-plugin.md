---
title: kubectl Plugin
---

The Rook kubectl plugin is a tool to help troubleshoot your Rook cluster. Here are a few of the operations that the plugin will assist with:

- Health of the Rook pods
- Health of the Ceph cluster
- Create "debug" pods for mons and OSDs that are in need of special Ceph maintenance operations
- Restart the operator
- Purge an OSD
- Run any `ceph` command

See the [kubectl-rook-ceph documentation](https://github.com/rook/kubectl-rook-ceph) for more details.

## Installation

- Install [krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/)
- Install Rook plugin

  ```console
    kubectl krew install rook-ceph
  ```

## Ceph Commands

- Run any `ceph` command with `kubectl rook-ceph ceph <args>`. For example, get the Ceph status:

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

Reference: [Ceph Status](https://github.com/rook/kubectl-rook-ceph/blob/master/README.md#run-a-ceph-command)

## Debug Mode

Debug mode can be useful when a MON or OSD needs advanced maintenance operations that require the daemon to be stopped. Ceph tools such as `ceph-objectstore-tool`, `ceph-bluestore-tool`, or `ceph-monstore-tool` are commonly used in these scenarios. Debug mode will set up the MON or OSD so that these commands can be run.

- Start the debug pod for mon b

  ```console
    kubectl rook-ceph debug start rook-ceph-mon-b
  ```

- Stop the debug pod for mon b

  ```console
    kubectl rook-ceph debug stop rook-ceph-mon-b
  ```

Reference: [Debug Mode](https://github.com/rook/kubectl-rook-ceph/blob/master/README.md#debug-mode)
