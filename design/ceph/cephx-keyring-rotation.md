# CephX Keyring Rotation

Feature request: https://github.com/rook/rook/issues/15904

## High-level strategy

From an end-user UX view, we can divide keys/keyrings into 2 categories:

### "daemon" keys
Keys for Ceph daemons that are purely used internally to the Ceph cluster.

Rook can rotate these keys automatically and transparently without risk of disrupting any
connections to the Ceph cluster beyond Rook's control.

e.g., Admin key(s), mon, mgr, osd, mds, rgw, rbdmirror's internal (non-peer) key

### "Non-daemon" keys
Any key whose rotation may reasonably require user action beyond Rook API controls.

Because these keys affect non-daemon connections, Rook must make sure the user can initiate rotation
during a maintenance window. It is imperative that the user can determine when rotation is finished,
so they know when to update the non-daemon application with the updated key.

e.g., CephClient keys, RBD/CephFS mirroring peer tokens, CSI keys

Because different types of key rotations may call for different administrator/user workflows, these
rotations should be able to be initiated independently. For example, a user may have an automated
workflow to update CephClient A's consumer, but CephClient B's consumer may be a manual process.
CephClient A and B should allow rotation separately.

CSI keys are a special case that best fits into the non-daemon case. Rotation of CSI keys will affect
Ceph-CSI's RBD and CephFS volume mounts associated with application pod PVCs. This rotation may
require administrators to reboot or drain/undrain all nodes with Ceph-CSI PVCs.

### Overlapping rotation

When CSI keys are updated, users must drain/undrain (or reboot) all nodes to transition PVC mounts
from using old key to new key.

Users with many nodes may not be able to update them all within the (2 * `auth_service_ticket_ttl`)
window. Some users with very large k8s clusters might only drain/reboot a portion of their nodes
during a maintenance window over a period of several days. (Service ticket TTL could be extended,
but extending it to several days would also have Ceph security implications that are best avoided
if possible)

In cases where the rotated key can't be picked up quickly, the alternative is for Rook to create a
new ceph client (user) with the new key without deleting the old user/key. Then, Rook updates the
CSI secrets to have the new user+key. This would begin the window in which users can drain/reboot
nodes. Any new PVC mounts would use the new key, and old mounts would continue operating happily
since the old user+key are not yet destroyed.

After the user is done with node updates, the user should have some way of indicating to Rook that
the old key is no longer needed, telling Rook it is safe to delete the old key. Autodetection might
be possible, but let's leave that for a future exercise.

For clarity, this design could be called "overlapping" rotation, and the design elsewhere in this
doc "in-place" rotation.

Overlapping rotation will be implemented for only CSI keys.

Overlapping rotation will not be implemented for the CephClient resource because each CephClient
represents a single client, which is a single client ID and its corresponding key. For users or
systems that propagate CephClient credentials, overlapping rotation can be accomplished by creating
a new CephClient, propagating the new client credentials as needed, then deleting the old CephClient
after completion.

In the future, overlapping rotation would be beneficial for the RBD/CephFS mirror peering keys.
Currently, the user name for RBD peering is hardcoded, making overlapping rotation impossible.

## User interfaces

### Key rotation base interface

```yaml
keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
keyGeneration: <int> # used with KeyGeneration
keepPriorKeyCount: <int> # only present to configure overlapping rotation
```

- `keyRotationPolicy` (string): select a key rotation policy:
    - `Disabled` (default when unset): don't rotate keys after initial creation
    - `WithCephVersionUpgrade`: rotate keys when the Ceph version updates
    - `KeyGeneration`: rotate when the `keyGeneration` input is greater than the current key generation

- `keyGeneration` (int): when `keyRotationPolicy==KeyGeneration`, set this to the desired key
    generation value. Ignored for other rotation policies. If this is set to greater than the
    current key generation, relevant keys will be rotated, and the generation value will be updated
    to this new value (generation values are not necessarily incremental, though that is the
    intended use case).
    If this is set to less than or equal to the current key generation, keys are not rotated.

API for overlapping rotation:

- `keepPriorKeyCount` (int): only available for components that use overlapping key rotation. This
    tells Rook how many prior keys to keep active. Generally, this would be set to `1` to allow for
    a migration period for applications. If desired, set this to `0` to delete prior keys after
    migration.

Alternative key rotation policy designs that were rejected:

Rook could use a string like `once` as an input, but Rook would have to record when `once` was first
observed so that it doesn't repeat rotations with every reconcile.

Rook could alternatively use a string like `always` and expect the user to unset `always` after they
see that rotation is complete, but this requires the user to monitor the rotation status and change
config in a clunky way that might risk manual errors.

Rook could use the Ceph version as the "older-than" selection, but that would only allow CSI key
rotations when the Ceph version changes. For a user already at the latest Ceph version, they
wouldn't have the option to rotate CSI keys on demand.

Rook could use a `DATETIME` string to initiate rotation for keys minted/rotated before a certain
time, but it is hard to track rotation time well when many keys are involved. Generation allows for
a more clear representation of actual state and provides a simple interface.

### Enabling daemon key rotations

Ceph developers have suggested rotation on every Ceph version update. This corresponds to a rotation
each time the administrator updates the CephCluster's Ceph image. This also aligns with when Rook
knows Ceph daemons are going to be restarted and when Rook can reasonably assume the administrator
has a maintenance window. Rotation at this periodicity will have no additional impact on cluster
connectivity or performance, so this periodicity will be used as the suggested periodicity option,
and one-time updates will be allowed as well.

For ease of configuration, the option for rotating daemon keys will be present on only the
CephCluster CR. Any child CRs (e.g., CephFilesystem) dependent on the CephCluster will inherit the
daemon key rotation config from the corresponding CephCluster. This allows administrators to enable
key rotation selectively for specific CephClusters while also keeping the UX simple.

```yaml
spec:
  # ...
  security:
    cephx:
      daemon:
        keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
        keyGeneration: <int> # used with KeyGeneration
      csi: {} # discussed more later
      # (room for future spec.security.cephx options)
    # (room for future spec.security options)
  # ...
```

List of Ceph daemons which have daemon keys that can be rotated automatically:

- mon - all share the same key
- mgr
- osd
- mds
- rgw
- nfs
- rbdmirror - daemon key will auto-rotate; related peer tokens will not auto-rotate
- cephfsmirror - same considerations as rbdmirror
- log-rotator
- crash-collector

The Ceph admin key Rook uses to run Ceph commands will also be rotated automatically. Care should be
taken to ensure admin key rotation doesn't block rotation of other keys.

Rook considered using an operator-level global config option
`ROOK_ROTATE_DAEMON_CEPHX_KEYS_OLDER_THAN`, but this does not allow controllers to get reconciliation
events when the config is modified. Associating the config with CephCluster allows controllers to
reconcile as needed if/when the user modifies the configuration(s).

### Enabling CSI key rotation

Ceph-CSI deployments (if managed by Rook) are managed in the operator namespace, but keys are
created on a per-CephCluster basis. Thus, a CephCluster configuration option (like above) is most
appropriate. CSI key rotations require manual administrator action to reboot or drain/undrain
nodes to remount PVCs with the new key, so Rook will avoid automatic rotation and only implement
one-time rotation options. To allow for an arbitrarily-long maintenance window for admins to perform
node actions, CSI will use overlapping rotation.

```yaml
spec:
  # ...
  security:
    cephx:
      daemon: {} # discussed above
      csi:
        keyRotationPolicy: Disabled | KeyGeneration
        keyGeneration: <int> # used with KeyGeneration
        keepPriorKeyCount: 1
      # (room for future spec.security.cephx options)
    # (room for future spec.security options)
  # ...
```

Note on `keyRotationPolicy` for CSI. `WithCephVersionUpgrade` will not be supported for CSI keys
unless we can validate that the keys can safely be rotated without the risk of affecting existing
PVC mount connectivity. Rook will return an error if this value is given.

### Enabling non-daemon key rotation

Ceph daemons that have keys used by non-Rook-controlled clients are also associated with
Custom Resources (CRs).

Full list of Rook CRs with non-daemon keys:

- CephClient - the created arbitrarily-usable client has a single associated key
- CephBlockPool - generates a mirror peer token for the pool (`peer`)
- CephFilesystem - generates a mirror peer token for the filesystem (`peer`)

Each CR will provide a key rotation mechanism as part of the primary API spec.

## Status reporting

For users to determine when Rook has successfully rotated keys, two pieces of information must be
reported:

Different Rook CRs will need to report status slightly differently (more below). The reused status
fields will be as follows:

```yaml
keyGeneration: <int>
keyCephVersion: "20.2.0" # e.g.
```

- `keyGeneration` (int): the CephX key generation for the most recently (successfully) reconciled
    resources. This status field is always updated, even when `keyRotationPolicy` is not
    `KeyGeneration`. When keys are first created, the generation is `1`. Generation `0` indicates
    that initial reconciliation (including key creation) has not finished, or keys existed prior to
    the implementation of the key rotation feature.

- `keyCephVersion` (string): the Ceph version that minted the currently-in-use keys.
    This must be the same string format as reported by `CephCluster.status.version.version` to allow
    them to be compared by users to determine when rotation is complete. E.g., `20.2.0`.
    An empty string indicates that the version is unknown, as expected in brownfield deployments.

For keys rotated `WithCephVersion`, the `status...keyCephVersion` can be compared to the Ceph
version known to be in the image being deployed. When status equals that in the image, rotation is
complete.

For keys rotated using `keyGeneration`, When `status...keyGeneration  >=  spec...keyGeneration`,
rotation is complete.

These statuses will be filled on all Rook resources when CephX keys are first created, even when key
rotation is not enabled. This will ensure that users can always know the Ceph version and generation
of minted keys -- or, by absence, show that the info is unknown.

For keys rotated via the overlapping mechanism, this status is also added:

- `priorKeyCount` (int): the number of prior keys currently kept active.

It is best if Rook is able to ensure old generation(s) of keys in Ceph's auth system are tracked
accurately. This is especially important if a bug occurs and Rook loses track of how many keys it
has generated for a component via resource statuses. This design doc recommends appending the
current `keyGeneration` to the Ceph auth client name to ensure  Rook can list keys and ensure only
the current generation of keys and desired number of previous generations of keys exists.

## Design details

In Rook, key rotation will be automated on a per-reconcile-controller basis. Wherever keys are
currently being created/deleted when Rook creates/deletes Ceph daemons, rotation will occur nearby.
This will ensure that key rotations can result in immediate daemon restart, allowing for appropriate
detection of key rotation errors before a cluster or child resource might be brought offline due to
unexpected errors.

The key rotation workflow will fit the following high-level process, adapted as necessary for each
Ceph daemon to ensure key rotation is applied:

1. Reconciliation of a Rook CR begins
2. The reconcile gets the current CR spec+status (Already part of all reconciles)
3. The reconcile runs a cmd-reporter job to detect the Ceph version present in the Ceph container
    image (`cephImageVersion`) that will be deployed (Already part of all reconciles)
4. For each daemon deployment, Rook gets the current CephX key for the daemon
    1. If an existing key doesn't exist, a new one is created, and...
        1. `keyGeneration` is taken to be `1`
        2. `keyCephVersion` is taken to be `cephImageVersion`
5. Rook checks the associated spec configs to see if rotation is enabled.
    1. For `KeyGeneration` single-rotation configs, Rook checks the appropriate
        `status...keyGeneration`. If `spec...keyGeneration` is greater, the key is rotated.
    2. For `CurrentCephVersion` configs, Rook checks the appropriate `status...keyCephVersion`.
        If `cephImageVersion` is greater, the key is rotated.
6. Rook updates the secret containing the daemon(s) keyring with the up-to-date content
    (Already part of reconciles)
7. If keys were created/rotated, Rook must make sure that daemons restart, including when a prior
    reconcile already updated the daemon deployment's Ceph image version. To guarantee rotation,
    Rook applies an annotation to the updated pod spec, that changes at each key secret update.
    This is not part of a user-facing API and will not be read by the operator to inform any
    operations. It's only purpose is to guarantee pod restart any time the key is rotated.
8. IFF keys were created/rotated, Rook updates the appropriate cephx key status(es)
    1. `status...keyGeneration` is taken to be `spec...keyGeneration`
    2. `status...keyCephVersion` is taken to be `cephImageVersion`

### CephCluster example

1. CephCluster daemons are all daemon, so the reconcile reads
    `spec.security.cephx.daemon` to determine if the reconcile should rotate.
2. When key rotation is indicated:
    1. Rotate the admin key. This is the first key created by Rook and will be the first rotated.
        This should be as easy as rotating it and updating the daemon reference. However, depending
        on how child reconciliations are updated with the admin key, it may be necessary to restart
        the Rook operator reconcile manager (all reconcile routines) after this rotation.
    2. Update the CephCluster status with the admin key info
    3. Rotate the mon key. Mon key rotation requires a special process:
        1. Update mon deployments first without rotating the key
        2. Rotate the key
        3. Restart the mon deployments to cause them to get the rotated key
    4. Update the CephCluster status with the mon key info
    5. For each mgr, rotate the key and update/restart the mgr daemon
    6. Update the CephCluster status with the mgr key info
    7. For each OSD, rotate the key
        1. OSDs don't have an associated keyring secret, so we must make sure all OSD deployments'
            "activate" init container gets the latest key and updates it for the "osd" pod
    8. Update the CephCluster status with the OSD key info

With separated daemon key info tracking, the status will look like so:

```yaml
spec:
  security:
    cephx:
      daemon: {}
      csi: {}
      rbdMirrorPeer: {}
status:
  # ...
  cephx:
    admin:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    mon:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    mgr:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    osd:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    rbdMirrorPeer: # cluster-level RBD mirror peer key (client.rbd-mirror-peer)
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    crashCollector:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    exporter:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    csi: {} # discussed more below, but some CSI keys and need to be in this status
```

Keeping track of each daemon type separately helps Rook ensure it won't re-rotate keys from earlier
in the reconcile if the reconcile needs to restart later in the process. For example, there is no
need to re-rotate admin/mon/mgr keys in the event the reconcile needs to restart in the middle of
OSD updates. Because OSD updates can take quite a while, this case is likely to occur across Rook's
large user base.

### CSI example

CSI keys are updated as part of a CephCluster reconcile.

For a CephCluster reconcile:

1. Ceph-CSI daemon keys (provisioners and node plugins) use `CephCluster.spec.security.cephx.csi`
    (not `...daemon`) to determine if the reconcile should rotate.
2. Assuming the rotation is indicated, rotate keys.
3. Update the CephCluster status with the CSI key info

    ```yaml
    kind: CephCluster
    # ...
    status:
      # ...
      cephx:
        # (status from CephCluster daemon rotations)
        csi:
          keyGeneration: 2 # e.g.
          keyCephVersion: "20.2.1" # e.g.
          priorKeyCount: 1 # e.g.
    ```

### CephBlockPool example

When mirroring is enabled on a CephBlockPools, it results in Rook creating an RBD mirror peering
token for the pool. The key housed within the token is hardcoded in Ceph to use the
`client.rbd-mirror-peer` user and key. Therefore, the singular RBD mirror key is rotated at the
CephCluster level (above), but Rook should still update CephBlockPool statuses to identify when the
peering token has been updated to use the latest peer token.

For token updates, the CephBlockPool controller should reconcile when the parent CephCluster's
`status.cephx.rbdMirrorPeer` is updated. When the CephBlockPool controller reconciles and creates
its bootstrap token, it should copy `CephCluster.status.cephx.rbdMirrorPeer` to its own
`status.cephx.peerToken`, which will indicate that the token has been updated with the latest key
after a CephCluster key rotation event.

```yaml
kind: CephBlockPool
# ...
status:
  # ...
  cephx:
    peerToken:
      keyGeneration: 2
      keyCephVersion: "20.2.2"
```

### CephFilesystem mirror example

Design for this should mirror `rbdMirrorPeer`/`peerToken` above, tailored for CephFS mirror design.

### CephClient example

CephClient stands out due to its simplicity. It will have one and exactly one key which can serve
any purpose (neither daemon key nor peer token). The interfaces are simplified to reflect this.

```yaml
kind: CephClient
# ...
spec:
  # ...
  security:
    cephx:
      keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
      keyGeneration: <int> # used with KeyGeneration
    # (room for future spec.security options)
  # ...
status:
  cephx:
    keyGeneration: 1 # e.g.
    keyCephVersion: "20.2.0" # e.g.
```

## Unused keys

There are a number of keys created by Ceph automatically that are unused. The Ceph team has
indicated that they will eventually modify Ceph to stop creating these keys. Before that change is
made, it is safe for Rook to delete them. Since there is no need to rotate unused keys, deletion is
best.

- client.bootstrap-rgw
- client.bootstrap-rbd
- client.bootstrap-mgr
- client.bootstrap-mds

## Dependencies

Ceph Tentacle (v20) provides a new `ceph auth rotate` command merged here:
https://github.com/ceph/ceph/pull/58121. Rook will rely on this command to rotate keys.

A few CephX technical details that are important for understanding CephX key rotation development
are summarized below. More details here: https://docs.ceph.com/en/latest/dev/cephx/

CephX keys minted by Rook are only used by Ceph for initial daemon connection. Internally, all Ceph
connections use "service" keys with laddered expiration times.

By default, Ceph service keys allow at least 2 hours from key rotation until the client must be
updated with the new key. There are 3 service keys with TTLs 1 hour, 2 hours, and 3 hours from the
time when service keys were last refreshed. Assuming the first TTL expiration is imminent, it is
still at least 2 hours until the 3-hour TTL expiration.

Ceph does not allow keyrings to contain multiple keys for a given client/daemon. When a key is
rotated, the old key is removed and new key replaces it with no ability to have a 2-valid-keys
transition period.

Ceph's `auth_service_ticket_ttl` and `auth_mon_ticket_ttl` config options allow users to
shorten/lengthen that time as desired/needed. Note that this isn't immediate: it will take an
unknown amount of time for internal service keys to update.

Ceph config `debug_auth=30` provides maximum CephX debug logs, as a development aid.

## Ceph admin key rotation

Rotation of the Ceph admin key is a risky process. Rook must ensure that admin key rotation cannot
brick a Rook cluster. As such, process details is designed in full here.

Today, the `rook-ceph-mon` secret contains the authoritative `client.admin` keyring
(the "primary admin keyring").

While `client.admin` may be able to rotate its own key, the process of rotating the key and updating
the secret could not be made atomic. If the reconcile or operator were to fail between rotation and
secret update, there could be no way to recover the cluster.

To ensure CephCluster reconciliation can be recovered in the event of failures, Rook will create a
new, temporary admin user `client.admin-rotator` whose sole purpose is to rotate the primary admin
keyring. It will be created to rotate the primary admin keyring and removed after rotation.

While `client.admin-rotator` exists, it must be stored also. To avoid the complexity and risk of
adding a temporary field to the `rook-ceph-mon` secret for storing the admin rotator keyring, a new
`rook-ceph-admin-rotator-keyring` secret will be used.

### Rotation procedure

First, we establish a rotation procedure. Because this procedure is risky, the process takes extra
caution to verify that updates to the secret are properly stored. There is no room for error.

1.  as `client.admin`: run `ceph auth get-or-create client.admin-rotator` w/ admin permissions
2.  create/update the `rook-ceph-admin-keyring` secret with the `client.admin-rotator` keyring
3.  **ROTATE**: as `client.admin-rotator`: run `ceph auth ls` to ensure it has permissions
4.  as `client.admin-rotator`: run `ceph auth rotate client.admin`
5.  as `client.admin` run `ceph auth ls` to ensure it has permissions
6.  update the on-disk ceph config file with new `client.admin` keyring
7.  update the `rook-ceph-mon` secret with new `client.admin` keyring
8.  as `client.admin`: run `ceph auth rm client.admin-rotator`
9.  **CLEANUP** delete the `rook-ceph-admin-keyring` secret
10. update `CephCluster.status.cephx.admin` with updated CephX status

### In post-mon-startup actions

Rotation of the admin key should happen after mons are updated. This is important if Ceph is being
upgraded and the 'current' Ceph version doesn't support the `auth rotate` command, but the 'new'
Ceph version does support it. (This consideration also exists for rotating the `mon.` key)

1. use `CephCluster.spec.security.cephx.daemon` to determine if rotation is indicated
2. if indicated, perform the rotation procedure from start to finish

### In CephCluster startup

If rotation fails or the operator restarts after the **ROTATE** step, the CephCluster reconcile will
begin again. In the new reconcile, the secret may have the wrong info in `data.keyring`. In that
case, the CephCluster reconcile would be unable to take any admin actions, effectively bricking the
reconcile. Therefore, the CephCluster reconciler must be able to recover from any interrupted admin
key rotation and must do so before any other Ceph admin actions.

1. read the secret (same as today)
2. load `client.admin` from the secret `data.keyring` (same as today)
3. if secret `data.rotatorKeyring` is present, prior rotation failed somewhere - recovery needed
    1. as `client.admin`: run `ceph auth ls`
        1. if success **and** `client.admin-rotator` is not present in output, final cleanup failed
            1. GOTO: **CLEANUP**
        2. otherwise (even if `auth ls` failed), rotation failed somewhere between **ROTATE** and **CLEANUP**
            1. load `client.admin-rotator` from the secret `data.rotatorKeyring`
            2. GOTO: **ROTATE**
4. continue CephCluster reconcile (same as today)

## Prior art

Kubernetes documentation explains how to encrypt data at rest in the cluster and how keys are
rotated in this document:
https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/#rotating-a-decryption-key

In that document, users are in charge of generating keys. Because Rook is in charge of generating
CephX keys, the k8s design does not translate to this feature.

IBM Credential Rotator Operator (https://github.com/IBM/credential-rotator-operator) automatically
rotates keys for an application and restarts the application pod(s) afterwards. The input that
initiates rotation is not easy to understand, but the status shows `PreviousResourceKeyID`. Since
Ceph keys don't have an ID associated with rotation, this seems similar to Rook tracking
"key version" metadata on its own for key rotation.
