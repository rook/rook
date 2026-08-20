---
title: CephX Keys and Rotation
---

!!! bug
    Ceph announced security vulnerability CVE-2025-30156 impacting all Ceph clusters, including
    Rook clusters. See [CVE-2025-30156 Resolution](#cve-2025-30156-resolution) below for more.

Ceph supports key rotation and a new, recommended AES256K key type beginning with the following versions:

- Squid - v19.2.6
- Tentacle - v20.2.4
- Umbrella - v21.2.0

Rook supports these in the following versions:

- v1.19.9
- v1.20.5

When following the documentation below, ensure the Rook cluster uses at least one of the above versions.

!!! important
    In order to use the new AES256K key type with Ceph CSI Persistent Volume Claims, the Linux
    kernels on Kubernetes hosts must support the new key type. Kernel versions 7.0 and up.
    FIPS users have an additional [note](#fips-enforcing).

Rook autodetects the correct [CephX authentication keys](https://docs.ceph.com/en/latest/dev/cephx/)
to use for new CephClusters. Rook is also able to rotate CephX authentication keys when requested.

When keys are rotated from AES to AES256K, Ceph daemons and clients will continue to use the old
type internally for two to three hours. This is normal. Ceph refers to these as rotating service
tokens.

## Key rotation overview

CephX authentication keys can be rotated when desired on a one-off basis. To provide this
capability, Rook utilizes an approximation of Kubernetes's resource generation. A one-time key
rotation is initiated by specifying `KeyGeneration` as the desired policy (the default policy is
`Disabled`) and also specifying a key generation higher than the current generation.

The keys Rook manages can be divided into two categories, below.

### Daemon keys

The term daemon is used loosely by Rook to mean both core Ceph daemons as well as tightly-integrated
gateways that act as Ceph clients. "Daemon" keys are used internally within a Ceph cluster, and
their rotation does not affect CSI volumes or connections to a Ceph cluster from outside.

Daemon key rotation is configured via the CephCluster `spec.security.cephx.daemon` config. This will
also rotate keys for several child resource daemons and gateways:

- CephFilesystem MDS daemons
- CephObjectStore RGW gateways
- CephNFS Ganesha gatways[*](#nfs-per-export-keys)
- CephNVMeOFGateway gateways

Rotation requires most Ceph daemons to restart, so this operation is best done at the same time Rook
is updated or at the same time the CephCluster `spec.cephVersion.image` is updated -- when daemons
will normally need to restart.

Rook automatically detects the best CephX key type for daemon keys. Do not set this unless required
to work around an issue.

### "Non-daemon" keys

Non-daemon keys may reasonably require user action beyond Rook API controls. All non-daemon keys act
as Ceph clients, but many are referred to by unique names distinguished from arbitrary CephClients.

Because these keys affect connections outside of Rook's control, their rotation is initiated
independently. This allows different Ceph clients to have different maintenance windows.

Below is a list of non-daemon keys along with the controlling config.

#### **CSI keys**

CephCluster CSI keys are controlled via CephCluster `spec.security.cephx.csi`.

Rotated CSI keys only take effect for new PVC mounts. For CSI alone, Rook is able to create a new
generation of keys while also keeping a number of prior key generations active. This is configured
using the `keepPriorKeyCountMax` option and prevents CSI provisioner and PVC downtime during and
after rotation.

AES256K CSI keys require Linux kernel support. Set the CSI `keyType: "aes"` until all Kubernetes
nodes run Linux kernel 7.0+. FIPS users have an additional [note](#fips-enforcing).

!!! tip
    Always set the desired `spec.security.cephx.csi.keyType` to guarantee the key type can be
    supported by kernel mounts.

#### **CephClient keys**

Each CephClient key is controlled via its own `spec.security.cephx` config.

CephClient resources are usually needed for custom applications to connect to the Rook Ceph cluster.
Such an application might use native Ceph binaries or libraries to perform Ceph operations, or might
use the Linux kernel to mount Ceph filesystems or RBD volumes.

When initiating rotation, use the `aes256k` key type when:

- Ceph binaries/libraries support the AES256K key type (Ceph v19.2.6+, v20.2.4+, v21+)
- Linux kernel mounts use kernel v7.0 or higher (FIPS users have an additional [note](#fips-enforcing))

Otherwise, use the `aes` key type.

!!! tip
    Always set the desired `spec.security.cephx.keyType` for CephClients to guarantee the key type
    can be supported by the client.

#### **RBD Mirror peer keys**

The CephCluster RBD mirror peer key is controlled via CephCluster `spec.security.cephx.rbdMirrorPeer`.

Each CephBlockPool that has mirroring configured will have a `peerToken` status that references the
CephCluster RBD mirror peer key.

The RBD Mirror peer will need to be specified as type `keyType: aes` if any peer clusters don't yet
support AES256K.

!!! tip
    Always set the desired `spec.security.cephx.rbdMirrorPeer.keyType` to guarantee the key type can
    be supported by peer clusters.

## Rotating keys

Enable key rotation for any key or set of keys by setting `keyRotationPolicy: KeyGeneration`. Key
rotation is disabled otherwise.

Initiate key rotation by setting `keyGeneration` to a positive integer value greater than the
current `keyGeneration` status associated with the key(s). Key rotation is also initiated when the
`keyType` spec is changed.

See supplementary documentation for
[CephX config](https://rook.io/docs/rook/latest/CRDs/specification/?h=cephx#ceph.rook.io/v1.CephxConfig)
as needed.

### Rotation example

Most key rotations are initiated from the CephCluster. For Rook clusters, the example below shows
how all keys can be rotated while keeping the `aes` key type for CSI and RBD mirror peers. This is a
common scenario. See [migrating CSI keys to a new key type](#migrating-csi-keys-to-a-new-key-type)
below for CSI key guidance.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: my-cluster
  namespace: rook-ceph # namespace:cluster
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v20.2.4
  security:
    cephx:
      daemon:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
      csi:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
        keepPriorKeyCountMax: 1  # keep one prior key also
        keyType: aes # keep the old aes key type when the host kernels do not yet support aes256k
      rbdMirrorPeer:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
        keyType: aes # keep the old aes key type when the peer does not yet support aes256k
  # ...
```

Once rotation is complete, the CephCluster status should look something like below. Each CephX key
type managed for the cluster is listed.

```yaml
status:
  # ...
  cephx:
    admin:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    cephExporter:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    crashCollector:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    csi:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
      priorKeyCount: 1
      keyType: aes
    mgr:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    mon:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    osd:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
    rbdMirrorPeer:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
      keyType: aes
```

Additionally, any CephFilesystem, CephObjectStore, CephNFS, or CephNVMeOFGateway will show the
status of rotation for their daemon keys:

```yaml
status:
  # ...
  cephx:
    daemon:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
```

If mirroring is enabled on a CephBlockPool, the following status will mirror the CephCluster's
`rbdMirrorPeer` status:

```yaml
status:
  # ...
  cephx:
    peerToken:
      keyCephVersion: 21.2.0-0
      keyGeneration: 2
```

!!! note
    When the admin key rotates, the toolbox pod may need to be restarted to refresh the keyring.
    The latest toolbox manifest will reload the keyring automatically after a few minutes delay.

## Key types

`keyType` can be specified during resource creation or during key rotation for any Rook resource to
tell Rook to create/rotate keys of a specific type.

Supported key types:

- `aes` - use this key type when clients or kernels do not support other key types
- `aes256k` - this is the latest key type which should be used whenever possible

### Health errors and warnings

Ceph may report a number of errors and/or warnings when older AES key types are in-use.

#### **Errors**

The following health errors are reported when critical Ceph daemons are not using the new AES256K
key type. Follow the [CVE-2025-30156 resolution](#cve-2025-30156-resolution) guide to resolve these
errors. If any of the following errors persist after upgrade and rotation, file a
[GitHub issue](https://github.com/rook/rook/issues/new/choose) or reach out on
[Slack](https://slack.rook.io/) for assistance.

- AUTH_INSECURE_SERVICE_KEY_TYPE
- AUTH_INSECURE_SERVICE_TICKETS

#### **Warnings**

The following health warnings are reported when non-core Ceph clients are not using the new AES256K
key type. Health warnings are listed below along with resolution guidance.

- AUTH_INSECURE_ROTATING_SERVICE_KEY_TYPE
    - Cleared 2-3 hours after core Ceph daemons [migrate to AES256K](#resolution-overview).
- AUTH_INSECURE_CLIENT_KEY_TYPE
    - Cleared once all keys are rotated to AES256K. This includes
        [CSI keys](#migrating-csi-keys-to-a-new-key-type),
        [RBD-Mirror keys](#rbd-mirror-peer-keys),
        [CephClient keys](#cephclient-keys),
        and [per-NFS-Export keys](#nfs-per-export-keys).
- AUTH_INSECURE_KEYS_ALLOWED
    - Cleared once [allowed ciphers](#allowed-ciphers) are restricted to AES256K
- AUTH_INSECURE_KEYS_CREATABLE
    - Cleared once [allowed ciphers](#allowed-ciphers) are restricted to AES256K

It is expected that Rook clusters will have constraints that prevent immediate resolution of all
warnings. Warnings that cannot be resolved immediately can be ignored safely. Rook also makes it
possible to [mute health warnings](../../CRDs/Cluster/ceph-cluster-crd.md#health-settings) to
avoid the related Ceph HEALTH_WARN status. Apply the CephCluster patch below to mute all warnings.

```yaml
spec:
  healthCheck:
    muteHealthWarning:
      AUTH_INSECURE_ROTATING_SERVICE_KEY_TYPE:
        policy: mute
      AUTH_INSECURE_CLIENT_KEY_TYPE:
        policy: mute
      AUTH_INSECURE_KEYS_ALLOWED:
        policy: mute
      AUTH_INSECURE_KEYS_CREATABLE:
        policy: mute
```

Replace `mute` values in the patch with `unmute`, and reapply after all keys are rotated to AES256K.
This will ensure any accidental AES keys raise visible warnings after full migration.

### Migrating CSI keys to a new key type

The [rotation example above](#rotation-example) shows how to rotate CSI keys while keeping the older
`aes` key type for all Linux kernel versions. This section explains how to follow up to migrate the
example's CSI keys to `aes256k` completely with minimal application downtime. At the end of this
migration, all CSI keys and all application PVCs will be utilizing new keys with the AES256K cipher.

1. First, double check that all host kernels support the AES256K keys (Linux kernel 7.0+).
    FIPS users have an additional [note](#fips-enforcing).
2. Initiate CSI key rotation using a patch like below. `keepPriorKeyCountMax: 1` ensures that the
    existing keys used for currently-mounted PVCs remain active.

    ```yaml
    spec:
      security:
        cephx:
          csi:
            keyRotationPolicy: KeyGeneration
            keyGeneration: 3 # modify as needed
            keepPriorKeyCountMax: 1  # keep in-use keys active
            keyType: aes256k
    ```

3. Wait for the rotation to be complete by watching `status.cephx.csi.keyGeneration`.
4. At this point, any new PVCs will be mounted using the new keys, but existing PVC mounts continue
    to use the old keys.
5. For each node in the Kubernetes cluster, cordon and drain the node, optionally reboot, and then
    uncordon the node. When Pods are rescheduled to the node, their new PVC mounts will use the
    latest CSI keys that Rook created.
6. Repeat step 5 until all nodes have been rehydrated.
7. As a final optional step, clean up old keys which are no longer in use by setting
    `keepPriorKeyCountMax: 0`.

### Migrating external cluster keys to a new key type

When Rook is configured to use an [external cluster](../../CRDs/Cluster/external-cluster/provider-export.md),
the `create-external-cluster-resources.py` script has flags available for assisting with key
rotation and for selecting the desired key type.

When running the script, add the below arguments for the cluster to rotate keys to the latest key
type. Don't forget to include other necessary flags for configuring RBD, CephFS, and/or RGW.

```console
python3 create-external-cluster-resources.py <other-flags> --cephx-key-rotate rotate --cephx-key-type aes256k
```

After the rotation, there will be a new set of keys at the latest version, and the old keys will
remain active to support existing PVCs. Import the new keys to Rook following external cluster
documentation.

Afterwards, PVC mounts can be migrated to the new keys following the node drain steps outlined in
the [CSI migration](#migrating-csi-keys-to-a-new-key-type) steps 4-7 above.

### Reverting back to an older key type

Support for AES256K keys is currently limited by client and Kernel support. If a CephX key is
rotated to the new type but the client/kernel does not support it, Rook allows reverting back to an
older key type. This process is as simple as specifying `keyType: aes` for the required component.
This will rotate the key again, and the new key will be of type `aes`.

### Allowed Ciphers

By default, Rook allows all key types to be created. This supports users who need to create older
key types to support an older Linux kernel or older Ceph clients. For clusters where all CephX keys
are using the latest `aes256k` key type, Ceph can be configured to restrict access to only clients
running the new key type and prevent creation of older key types. This serves as a security
hardening mechanism.

!!! warning
    Modifying this configuration can result in cluster unavailability if any core Ceph daemon key
    types do not match an allowed cipher. Use this configuration carefully.

For existing installs, first verify that all CephX keys use the newer key type. Use the command
below, and ensure all keys report `aes256k` in the output. If any keys report `aes`, do not proceed.

```console
ceph auth dump-keys --format=json-pretty
```

Once verified the older key type can be retired and restricted by setting the following CephCluster
`spec.security.cephx` config:

```yaml
spec:
  security:
    cephx:
      allowedCiphers:
        - aes256k
```

This can also be specified for new CephCluster installations where it's known that all clients and
kernels support AES256K keys.

!!! hint
    Use the CephCluster CephX security setting above for new Rook installs in environments with
    nodes running on Linux kernel 7.0+. FIPS users have an additional [note](#fips-enforcing).

#### Fixing cluster unavailability with allowedCiphers

If `allowedCiphers` is set to only `aes256k` while any Ceph daemon keys are using AES, Ceph will
fail with the below error. This will block Rook from updating the cluster.

```console
[errno 13] RADOS permission denied (error connecting to the cluster)
```

To resolve this issue, apply the following patch as a workaround.

```yaml
  security:
    cephx:
      allowedCiphers:
        - aes
        - aes256k
      daemon:
        keyType: aes # use this only as a workaround!
```

This will resolve the errors and allow Rook to proceed with reconciliation.

After all `rook-ceph-mon` Pods show the `--mon-auth-emergency-allowed-ciphers` CLI flag present and
Rook reconciliation continues without the error, remove `spec.security.cephx.daemon.keyType` from
the CephCluster. It has served its purpose as a workaround and should now be removed.

It is safest to leave `allowedCiphers` unmodified after this workaround.

Afterwards, review the allowed ciphers documentation above paying extra attention to the results of
`ceph auth dump-keys`. At least one Ceph daemon key is still using AES.

## Known issues

### NFS per-export keys

NFS-Ganesha's Ceph backend utilizes a separate CephX key for each NFS export. Rook is currently able
to rotate the CephX key used by the NFS-Ganesha daemon's main process, but Ceph has not implemented
a mechanism to rotate the per-NFS-export keys. Rook and Ceph development teams will continue to
focus on this issue.

### FIPS enforcing

When FIPS enforcing is enabled, the minimum Linux kernel version required for AES256K support is 7.2.

## CVE-2025-30156 resolution

Ceph announced security vulnerability CVE-2025-30156 impacting all Ceph clusters, including Rook
clusters.

All Rook users are advised to follow the [Quick Resolution Guide](#quick-resolution-guide) below as
soon as possible to resolve the vulnerability.

More background information on the CVE is present after the resolution guide for interested readers.

### Quick resolution guide

Follow these instructions to resolve CVE-2025–30156 as quickly as possible with minimal risk.

1. Upgrade to Rook v1.20.6 or higher (or v1.19.10 or higher)
2. Upgrade to Ceph v20.2.4 or higher (or v19.2.6 or higher)
3. Initiate key rotation for the keys you wish to rotate

Steps 2 and 3 can be done at once by applying the patch below to any CephCluster that hasn’t already
rotated CephX keys. PVCs will remain active throughout this process.

```yaml
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v1.20.4-20260818
  security:
    cephx:
      daemon:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
```

Update the patch to use Ceph v19.2.6-20260818 if needed for your environment.

Ceph health **warnings** will be present while any key remains using AES. These warnings can be
ignored or muted for now.

Ceph health **errors** will be present after the Ceph version is updated and while key rotation is
still unfinished. These errors should resolve after the CephCluster is done upgrading.
Refer to the [health errors and warnings](#health-errors-and-warnings) section for more details.

Following the [rotation example](#rotation-example), use the CephCluster `status.cephx` to double
check that keys are rotated. Remember that CSI and RBD-mirror peer keys will not be rotated.
CephFilesystem users should check its `status.cephx` as well.

**After this process is complete and verified, the CVE is resolved.** It will no longer be possible
to elevate the permissions of a CephX key. CephX authentication keys for core Ceph daemons, child
Ceph daemons (e.g., RGW), and Rook's admin key will have been rotated to the new AES256K cipher.

#### **Non-urgent follow-up tasks**

!!! Important
    **Do not rush to perform further steps.** Additional key rotations have prerequisites and
    caveats that are important to understand before proceeding. As long as the process above is
    completed, cluster security is not immediately vulnerable. Take your time.

Some Ceph health warnings (not errors) are likely to persist after this process.
[Warnings](#warnings) documentation explains how to interpret, manage, and (if desired) mute them.

Ceph developers recommend that users rotate remaining CephX client authentication keys to the latest
and most secure AES256K keys when possible to ensure their clusters have the best security.

For Rook users, review the remainder of this CephX documentation to understand how to rotate
authentication keys for CSI daemons, CephClients, and RBD-Mirror peers.

An optional, technical overview of the CVE follows, along with recommended
[mitigations](#mitigations) for users who are not able to resolve the CVE immediately.

### Background

Ceph uses a custom implementation of Kerberos for securing communication between Ceph daemons and
clients. This security implementation is called CephX, which (by design) most Rook users are
unlikely to be familiar with. Rook keeps CephX details abstracted from most end users to avoid
unnecessary complexity.

In order to understand the vulnerability and its resolution, we can provide a simplified view of how
Ceph authentication works. This explanation is sufficient for basic understanding of the
vulnerability and resolution but does not dive into technical nuances and caveats.

There are two sets of CephX keys in a Ceph cluster: authentication keys and service tickets.
Authentication keys are used by Ceph entities (daemons and clients) to establish identity when first
connecting to the Ceph cluster. Once an entity is authenticated, it is issued a series of encrypted
service tickets that define its authorizations for the four core Ceph daemons: monitors, managers,
OSDs, and filesystem MDses. Entities do not have keys to decrypt their service tickets, a second
layer of security.

When a component communicates with any Ceph daemon, it includes its relevant service tickets in
network packets. This allows core Ceph daemons to validate client entities in a decentralized
manner.

### Vulnerability overview

CVE-2025–30156 affects the previously mentioned service tickets. The existing AES service tickets do
not effectively implement integrity checks and can have their permissions modified without
detection, even without decrypting the AES-secured ticket.

An attacker who is able to (through other methods) gain access to a CephX key and masquerade as a
Ceph entity would be able to iteratively flip bits in the AES service ticket payload to attempt to
gain elevated Ceph permissions. With this workflow, an attacker would be able to obtain
admin-privileged permissions over the Ceph cluster in linear time.

### Resolution overview

To resolve this CVE, Ceph has introduced a new CephX key type for both authentication keys as well
as service tickets. The new key type cipher is called AES256K. In order to resolve the CVE, Ceph
must be upgraded, and CephX authentication keys for four core Ceph daemons must be migrated to the
new key type. In Rook terms, CephCluster monitors, managers, and OSDs; and CephFilesystem MDSes must
migrate.

Once these critical migrations are complete, all Ceph service tickets will use AES256K encryption.
Any other Ceph client entities that continue using older AES authentication keys will receive new
AES256K-encrypted service tickets. Thus, it will no longer be possible for any arbitrary CephX key
to be elevated to have admin permissions.

Rook and Ceph anticipate that most users will need to continue using older AES authentication keys
for non-core clients during a transition period in order to avoid storage service outages. This
transition period may last years for organizations with large or complex environments.

!!! Important
    CSI keys require Linux kernel version 7.0 or higher for AES256K authentication key support.
    Older kernels can still use AES authentication keys safely after Ceph is upgraded to issue
    AES256K service tickets. FIPS users have an additional [note](#fips-enforcing).


### Mitigations

This vulnerability is a good reminder of how important it is to ensure security at every layer, to
minimize the ability for any malicious actors to gain access where they shouldn’t. Below are some
mitigations to consider in all production environments, especially those where Ceph’s older AES keys
will be used:

- Avoid deploying privileged pods whenever possible.
- Drop `NET_RAW` permissions from pods whenever possible.
- When keys can’t yet be upgraded to AES256K, use Rook’s key rotation API to rotate AES keys regularly.
    Rotating keys during any Ceph or Rook version update is a good starting point.
- Use Kubernetes Network Policies. Rook’s experimental policies can be found
    [here](https://kubernetes.io/docs/concepts/services-networking/network-policies/).
- Consider enabling full over-the-wire Ceph network encryption via the Ceph option
    [`ms_client_mode`](https://docs.ceph.com/en/octopus/rados/configuration/msgr2/#connection-mode-configuration-options)
    Be advised of [performance effects](https://ceph.io/en/news/blog/2023/ceph-encryption-performance/).
- Ensure networking environments are resilient to ARP spoofing attempts.
- Use an immutable, container-optimized OS on Kubernetes nodes.
- Keep up-to-date on all security updates for all applications running in the Kubernetes cluster
    as well as all applications on the same network.
- Review recommendations for
    [Kubernetes Cluster Security](https://kubernetes.io/docs/tasks/administer-cluster/securing-a-cluster/).

There are more mitigations than can be reasonably covered in documentation. If community members
have more hardening advice, reach out on [Slack](https://slack.rook.io/)
or [GitHub Discussions](https://github.com/rook/rook/discussions).
