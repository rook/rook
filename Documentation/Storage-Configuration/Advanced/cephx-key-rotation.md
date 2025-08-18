---
title: CephX Key Rotation
---

!!! attention
    This feature is experimental.

Rook is able to rotate [CephX authentication keys](https://docs.ceph.com/en/latest/dev/cephx/) used
by Ceph daemons and clients.

For this experimental feature, some caveats should be noted:

- Only Ceph versions v19.2.3 and higher have the capabilities Rook requires for key rotation.
- Ceph Monitor (mon) keys cannot be rotated in Ceph v19 due to Ceph architecture limitations.

## Overview

CephX keys can be rotated when desired on a one-off basis. To provide this capability, Rook utilizes
an approximation of Kubernetes's resource generation. A one-time key rotation is initiated by
specifying `KeyGeneration` as the desired policy (the default policy is `Disabled`) and also
specify a key generation higher than the current generation.

CephX keys can be divided into two categories, below.

### Daemon keys

Daemon keys are used internally within a Ceph cluster, and their rotation does not affect CSI
volumes or connections to a Ceph cluster from outside.

Daemon key rotation is configured via the CephCluster `spec.security.cephx.daemon` config. This will
also rotate daemon keys for any CephFilesystem MDSes and CephObjectStore RGWs.

Rotation requires most Ceph daemons to restart, so this operation is best done at the same time the
CephCluster `spec.cephVersion.image` is updated -- when daemons will normally need to restart.

### "Non-daemon" keys

Non-daemon keys may reasonably require user action beyond Rook API controls.

Because these keys affect non-daemon connections, Rook allows users to initiate rotation
independently during their desired maintenance window.

Below is a list of non-daemon keys along with the controlling config.

- CephCluster CSI keys are controlled via CephCluster `spec.security.cephx.csi`
    - Rotated CSI keys only take effect for new PVCs. For CSI alone, Rook is able to create new keys
        while also keeping a number of prior key generations active. This is configured using the
        `keepPriorKeyCountMax` option.
- The CephCluster RBD mirror peer key is controlled via CephCluster `spec.security.cephx.rbdMirrorPeer`
    - Each CephBlockPool that has mirroring configured will have a `peerToken` status that
        references the CephCluster RBD mirror peer key
- Each CephClient key is controlled via its own `spec.security.cephx`

## Initiating key rotation

To begin experimenting with key rotation, check out
[CephX config](https://rook.io/docs/rook/latest/CRDs/specification/?h=cephx#ceph.rook.io/v1.CephxConfig)
options on Rook CRs.

### Example

Most key rotations are initiated from the CephCluster. An example spec that will rotate all CephX
keys for most new or upgraded Rook clusters is shown below.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: my-cluster
  namespace: rook-ceph # namespace:cluster
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v19.2.3
  security:
    cephx:
      daemon:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
      csi:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
        keepPriorKeyCountMax: 1  # keep one prior key also
      rbdMirrorPeer:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
  # ...
```

Once rotation is complete, CephCluster status should look something like below. Each CephX key
type managed for the cluster is listed.

```yaml
status:
  # ...
  cephx:
    cephExporter:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
    crashCollector:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
    csi:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
      priorKeyCount: 1
    mgr:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
    mon: {}  # reminder: mon key rotation is unsupported currently
    osd:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
    rbdMirrorPeer:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
```

Additionally, any CephFilesystem or CephObjectStore will show the status of rotation for their
daemons:

```yaml
status:
  # ...
  cephx:
    daemon:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
```

If mirroring is enabled on a CephBlockPool, the following status will mirror the CephCluster's
`rbdMirrorPeer` status:

```yaml
status:
  # ...
  cephx:
    peerToken:
      keyCephVersion: 19.2.3-0
      keyGeneration: 2
```
