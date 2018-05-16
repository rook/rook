# Decoupling Rook Agent from Backend Images

## Tl;dr

Rook operator should allow `rook` binary in its own container image while running backend storage (currently just Ceph) in a different image.

## Current Flow

Currently `rook` and all Ceph daemons and binaries are packaged in the same container image. 

The following illustrates the flow of starting `ceph-mon`:


| Seq  | Rook Operator  | Rook mon agent  | Ceph Daemon  |
|---|---|---|---|
| 1  | Create Replicaset to run `rook mon` command using the same container image   |   |   |
| 2  |   | `rook mon` exec `ceph-mon`  |   |
| 3  |   |   |  `ceph-mon` starts |


## Issues

Having rook and ceph in the same container image has the following issues:

- Unwanted ceph daemon restart during upgrade. Whenever rook binary is upgraded, a new image is created and the replicaset is refreshed. At this time, ceph daemons are restarted as well, even ceph binaries are the same. This tight coupling creates more disruptions when other storage backends are supported.

- Harder to use other ceph images. If customers choose to run ceph in a different image, e.g. `docker.io/ceph/daemon`, they have to make a new image to pick up `rook` binary.

## New Flow

| Seq  | Rook Operator  | Rook mon agent  | Ceph Daemon  |
|---|---|---|---|
| 1  | Create Replicaset to run `rook mon` command using the same container image in `init container` and use `docker.io/ceph/daemon` to run `ceph-mon` command in main container   |   |   |
| 2  |   | In init container, `rook mon` prepares for ceph mon but doesn't exec it. In main container, `ceph-mon` starts up using the directories prepared by `rook mon`  |   |
| 3  |   |   |  `ceph-mon` starts |

## Dependency

The new flow works for mon and mgr because the runtime options can be determined by operator. But for osd, runtime options such as osd id, data directory, etc are not known till `rook osd`  finishes. As a result, we need to wait for PR [one Pod per OSD](https://github.com/rook/rook/pull/1577) 

