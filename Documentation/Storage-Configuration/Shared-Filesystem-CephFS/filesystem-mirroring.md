---
title: Filesystem Mirroring
---

Ceph filesystem mirroring is a process of asynchronous replication of snapshots to a remote CephFS file system. Snapshots are synchronized by mirroring snapshot data followed by creating a snapshot with the same name (for a given directory on the remote file system) as the snapshot being synchronized. It is generally useful when planning for Disaster Recovery. Mirroring is for clusters that are geographically distributed and stretching a single cluster is not possible due to high latencies.

## Prerequisites

This guide assumes you have created a Rook cluster as explained in the main [quickstart guide](../../Getting-Started/quickstart.md)

## Create the Filesystem with Mirroring enabled

The following will enable mirroring on the filesystem:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: myfs
  namespace: rook-ceph
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPools:
    - name: replicated
      failureDomain: host
      replicated:
        size: 3
  preserveFilesystemOnDelete: true
  metadataServer:
    activeCount: 1
    activeStandby: true
  mirroring:
    enabled: true
    # list of Kubernetes Secrets containing the peer token
    # for more details see: https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#bootstrap-peers
    # Add the secret name if it already exists else specify the empty list here.
    peers:
      secretNames:
        #- secondary-cluster-peer
    # specify the schedule(s) on which snapshots should be taken
    # see the official syntax here https://docs.ceph.com/en/latest/cephfs/snap-schedule/#add-and-remove-schedules
    snapshotSchedules:
      - path: /
        interval: 24h # daily snapshots
        # The startTime should be mentioned in the format YYYY-MM-DDTHH:MM:SS
        # If startTime is not specified, then by default the start time is considered as midnight UTC.
        # see usage here https://docs.ceph.com/en/latest/cephfs/snap-schedule/#usage
        # startTime: 2022-07-15T11:55:00
    # manage retention policies
    # see syntax duration here https://docs.ceph.com/en/latest/cephfs/snap-schedule/#add-and-remove-retention-policies
    snapshotRetention:
      - path: /
        duration: "h 24"
```

## Create the cephfs-mirror daemon

Launch the `rook-ceph-fs-mirror` pod on the source storage cluster, which deploys the `cephfs-mirror` daemon in the cluster:  

```console
kubectl create -f deploy/examples/filesystem-mirror.yaml
```

Please refer to [Filesystem Mirror CRD](../../CRDs/Shared-Filesystem/ceph-fs-mirror-crd.md) for more information.

## Configuring mirroring peers

Once mirroring is enabled, Rook will by default create its own [bootstrap peer token](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/?#bootstrap-peers) so that it can be used by another cluster.
The bootstrap peer token can be found in a Kubernetes Secret. The name of the Secret is present in the Status field of the CephFilesystem CR:

```yaml
status:
  info:
    fsMirrorBootstrapPeerSecretName: fs-peer-token-myfs
```

This secret can then be fetched like so:

```console
# kubectl get secret -n rook-ceph fs-peer-token-myfs -o jsonpath='{.data.token}'|base64 -d
eyJmc2lkIjoiOTFlYWUwZGQtMDZiMS00ZDJjLTkxZjMtMTMxMWM5ZGYzODJiIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFEN1psOWZ3V1VGRHhBQWdmY0gyZi8xeUhYeGZDUTU5L1N0NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjEwLjEwMS4xOC4yMjM6MzMwMCx2MToxMC4xMDEuMTguMjIzOjY3ODldIn0=
```

### Import the token in the Destination cluster 

The decoded secret must be saved in a file before importing.

```console
# ceph fs snapshot mirror peer_bootstrap import <fs_name> <token file path>
```

See the CephFS mirror documentation on [how to add a bootstrap peer](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/).


Further refer to CephFS mirror documentation to [configure a directory for snapshot mirroring](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#mirroring-module-and-interface).

## Verify that the snapshots have synced

To check the `mirror daemon status`, please run the following command from the [toolbox](../../Troubleshooting/ceph-toolbox.md) pod. 

For example :

```console
# ceph fs snapshot mirror daemon status | jq
```

```json
[
  {
    "daemon_id": 906790,
    "filesystems": [
      {
        "filesystem_id": 1,
        "name": "myfs",
        "directory_count": 1,
        "peers": [
          {
            "uuid": "a24a3366-8130-4d55-aada-95fa9d3ff94d",
            "remote": {
              "client_name": "client.mirror",
              "cluster_name": "91046889-a6aa-4f74-9fb0-f7bb111666b4",
              "fs_name": "myfs"
            },
            "stats": {
              "failure_count": 0,
              "recovery_count": 0
            }
          }
        ]
      }
    ]
  }
]
```

Please refer to the `--admin-daemon` socket commands from the CephFS mirror documentation to verify [mirror status and peer synchronization status](https://docs.ceph.com/en/latest/dev/cephfs-mirroring/#mirror-daemon-status) and run the commands from the `rook-ceph-fs-mirror` pod:

```console
# kubectl -n rook-ceph exec -it deploy/rook-ceph-fs-mirror -- bash
```

Fetch the `ceph-client.fs-mirror` daemon admin socket file from the `/var/run/ceph` directory:

```console
# ls -lhsa /var/run/ceph/
```

```console
# ceph --admin-daemon /var/run/ceph/ceph-client.fs-mirror.1.93989418120648.asok fs mirror status myfs@1
```

```json
{
    "rados_inst": "X.X.X.X:0/2286593433",
    "peers": {
        "a24a3366-8130-4d55-aada-95fa9d3ff94d": {
            "remote": {
                "client_name": "client.mirror",
                "cluster_name": "91046889-a6aa-4f74-9fb0-f7bb111666b4",
                "fs_name": "myfs"
            }
        }
    },
    "snap_dirs": {
        "dir_count": 1
    }
}
```

For getting `peer synchronization status`:

```console
# ceph --admin-daemon /var/run/ceph/ceph-client.fs-mirror.1.93989418120648.asok fs mirror peer status myfs@1 a24a3366-8130-4d55-aada-95fa9d3ff94d
```

```json
{
    "/volumes/_nogroup/subvol-1": {
        "state": "idle",
        "last_synced_snap": {
            "id": 4,
            "name": "snap2"
        },
        "snaps_synced": 0,
        "snaps_deleted": 0,
        "snaps_renamed": 0
    }
}
```