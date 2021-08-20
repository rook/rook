---
title: Merging Rook and an existing ceph cluster
weight: 11600
indent: true
---

# Migration an existing Ceph Cluster into rook

Given an existing ceph cluster, we want to be able to migrate
it to a rook managed ceph cluster.

The following steps are planned describe how to
migrate it into a rook managed ceph cluster.

This document is **WORK IN PROGRESS**.

## Brainstorming

* The existing ceph cluster has ...
** an FSID that rook needs to learn
** Pools and possible rules
** various device classes

There might be different approaches for migrating

* RBD
* Objects storage
* CephFS

## Steps [WIP]

* Install rook as usual
* Inject fsid
  * `kubectl -n rook-ceph edit secret/rook-ceph-mon`
  * This seems to be easy, likely requires restarting pods
  * base64 without \n!
* Inject monmap (?)
** So that rook's mon connect to existing mon's
* Keyring(s)
  * Are likely stored in the monitors (?)
    * yes:
      https://ceph.io/en/news/blog/2015/ceph-using-monitor-keyvalue-store/
      says: "Ceph monitors make use of leveldb to store cluster maps, users and keys."
  * Might allow external access

### Monitors (mon) and Managers (mgr)


## Questions / problems to solve

* Phasing in rook with an even number of monitors
  * The existing clusters probably have an odd number
  * We can raise from 3 to 5 or from 5 to 7 mons by phasing in rook
* Is it possible to specify the fsid to use on bootstrapping rook?
  * If yes, this would probably safe a lot of work later
* Is it possible to specify a monmap to use on bootstrapping rook?
  * If yes, this would probably safe a lot of work later
* We need to map existing configurations to kubernetes objects
  * Can we write a script that does that?
* Where/how does rook decide whether on disk format is raw or LVM?
  * The OSD pod has an if for lvm/raw
* Why does the disaster recovery guide disable authentication?

## Strategies

* The K8S cluster will be created in addition to the existing
  mon/mgr/osd hosts
* Might want to choose the same ceph version
  * Upgrades are probably better in a 2nd step
* Rook will be bootstrapped onto it
* Having rook only add mon's and mgr's should be rather safe
  * mgr's stays as a standby
  * mon's add to the quorom, so an even number should be added
* A host serving OSDs (only) should be easy to convert
  * Set cluster to noout
  * Shutdown host
  * Replace OS with plain/new OS
  * Join the host using kubeadm join ... (or any other form)
  * rook should find the host / we add it manually
  * rook finds the disks (or we specify them manually) and creates OSD
    pods

## Best case scenario

In theory, the following steps could be enough:

* Update the fsid secret
* Delete mon pods -> will be restarted with correct fsid
* Get monmap from existing cluster
* Inject the monmap + a valid key
  * This will not contain the rook mons
  * However when the mon starts, it will (try to) join the cluster
  * It needs to have a key --
* Join the cluster
 * Do we need to update the rook configmap for tracking/including
   outside mons?
