# CephX Keyring Rotation

Feature request: https://github.com/rook/rook/issues/15904

## High-level strategy

From an end-user UX view, we can divide keys/keyrings into 2 categories:

### "Local" keys
Keys that are purely used internally to the Ceph cluster.

Rook can rotate these keys automatically and transparently without risk of disrupting any
connections to the Ceph cluster beyond Rook's control.

e.g., Admin key(s), mon, mgr, osd, mds, rgw, rbdmirror's internal (non-peer) key

### "Non-local" keys
Any key whose rotation may reasonably require user action beyond Rook API controls.

Because these keys affect non-local connections, Rook must make sure the user can initiate rotation
during a maintenance window. It is imperative that the user can determine when rotation is finished,
so they know when to update the non-local application with the updated key.

e.g., CephClient keys, CephRBDMirror peer keys, CSI keys

Because different types of key rotations may call for different administrator/user workflows, these
rotations should be able to be initiated independently. For example, a user may have an automated
workflow to update CephClient A's consumer, but CephClient B's consumer may be a manual process.
CephClient A and B should allow rotation separately.

CSI keys are a special case that best fits into the non-local case. Rotation of CSI keys will affect
Ceph-CSI's RBD and CephFS volume mounts associated with application pod PVCs. This rotation may
require administrators to reboot or drain/undrain all nodes with Ceph-CSI PVCs.

## User interfaces

### Key rotation base interface

```yaml
keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
keyGeneration: <int> # used with KeyGeneration
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

### Enabling local key rotations

Ceph developers have suggested rotation on every Ceph version update. This corresponds to a rotation
each time the administrator updates the CephCluster's Ceph image. This also aligns with when Rook
knows Ceph daemons are going to be restarted and when Rook can reasonably assume the administrator
has a maintenance window. Rotation at this periodicity will have no additional impact on cluster
connectivity or performance, so this periodicity will be used as the suggested periodicity option,
and one-time updates will be allowed as well.

For ease of configuration, the option for rotating local keys will be present on only the
CephCluster CR. Any child CRs (e.g., CephFilesystem) dependent on the CephCluster will inherit the
local key rotation config from the corresponding CephCluster. This allows administrators to enable
key rotation selectively for specific CephClusters while also keeping the UX simple.

```yaml
spec:
  # ...
  security:
    cephx:
      local:
        keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
        keyGeneration: <int> # used with KeyGeneration
      csi: {} # discussed more later
      # (room for future spec.security.cephx options)
    # (room for future spec.security options)
  # ...
```

List of Ceph daemons which have local keys that can be rotated automatically:

- mon - all share the same key
- mgr
- osd
- mds
- rgw
- nfs
- rbdmirror - has 2 keys: one local (will auto-rotate) and one mirror peer (will not auto-rotate)
- cephfsmirror - same considerations as rbdmirror
- log-rotator
- crash-collector

The Ceph admin key Rook uses to run Ceph commands will also be rotated automatically. Care should be
taken to ensure admin key rotation doesn't block rotation of other keys.

Rook considered using an operator-level global config option
`ROOK_ROTATE_LOCAL_CEPHX_KEYS_OLDER_THAN`, but this does not allow controllers to get reconciliation
events when the config is modified. Associating the config with CephCluster allows controllers to
reconcile as needed if/when the user modifies the configuration(s).

### Enabling CSI key rotation

Ceph-CSI deployments (if managed by Rook) are managed in the operator namespace, but keys are
created on a per-CephCluster basis. Thus, a CephCluster configuration option (like above) is most
appropriate. CSI key rotations may require manual administrator action to reboot or drain/undrain
nodes to PVCs, so Rook must allow users to initiate one-time key rotations.

```yaml
spec:
  # ...
  security:
    cephx:
      local: {} # discussed above
      csi:
        keyRotationPolicy: Disabled | KeyGeneration
        keyGeneration: <int> # used with KeyGeneration
      # (room for future spec.security.cephx options)
    # (room for future spec.security options)
  # ...
```

Note on `keyRotationPolicy` for CSI. `WithCephVersionUpgrade` will not be supported for CSI keys
unless we can validate that the keys can safely be rotated without the risk of affecting existing
PVC mount connectivity. Rook will return an error if this value is given.

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

### Enabling non-local key rotation

Ceph daemons that have keys used by non-Rook-controlled clients are also associated with
Custom Resources (CRs).

Full list of Rook CRs with non-local keys:

- CephClient - the created arbitrarily-usable client has a single associated key
- CephRBDMirror - mirror daemon has a mirror peer key (`peer`)
- CephFilesystemMirror - mirror daemon has a mirror peer key (`peer`)

Each CR will provide a key rotation mechanism as part of the primary API spec.

```yaml
kind: CephRBDMirror | CephFilesystemMirror
# ...
spec:
  # ...
  security:
    cephx:
      peer: # this distinction is not necessary but makes clear that it doesn't control local keys
        keyRotationPolicy: Disabled | WithCephVersionUpgrade | KeyGeneration
        keyGeneration: <int> # used with KeyGeneration
    # (room for future spec.security options)
  # ...
```

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

These statuses will be filled on all Rook resources that require cephx keys, even when key rotation
is not enabled. This will ensure that users can always know the Ceph version and generation of
minted keys -- or, by absence, show that the info is unknown.

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
    Rook applies the annotation `cephxKeyTimestamp` to the updated pod spec, with the
    value of `time.Now()`. This is not part of a user-facing API and will not be read by the
    operator to inform any operations (thus it is not given a `rook.io` key suffix). It's only
    purpose is to guarantee pod restart any time the key is rotated.
8. IFF keys were created/rotated, Rook updates the appropriate cephx key status(es)
    1. `status...keyGeneration` is taken to be `spec...keyGeneration`
    2. `status...keyCephVersion` is taken to be `cephImageVersion`

### CephCluster example

1. CephCluster daemons are all local, so the reconcile reads
    `spec.security.cephx.local` to determine if the reconcile should rotate.
2. When key rotation is indicated:
    1. Rotate the admin key. This is the first key created by Rook and will be the first rotated.
        This should be as easy as rotating it and updating the local reference. However, depending
        on how child reconciliations are updated with the admin key, it may be necessary to restart
        the Rook operator reconcile manager (all reconcile routines) after this rotation.
    2. <!-- TBD --> Update the CephCluster status with the admin key info (with internal retry)
    3. Rotate the mon key. Mon key rotation requires a special process:
        1. Update mon deployments first without rotating the key
        2. Rotate the key
        3. Restart the mon deployments to cause them to get the rotated key
    4. <!-- TBD --> Update the CephCluster status with the mon key info (with internal retry)
    5. For each mgr, rotate the key and update/restart the mgr daemon
    6. <!-- TBD --> Update the CephCluster status with the mgr key info (with internal retry)
    7. For each OSD, rotate the key
        1. OSDs don't have an associated keyring secret, so we must make sure all OSD deployments'
            "activate" init container gets the latest key and updates it for the "osd" pod
    8. <!-- TBD --> Update the CephCluster status with the OSD key info (with internal retry)

With separated daemon key info tracking, the status will look like so:

```yaml
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
    csi: {} # discussed more below, but some CSI keys and need to be in this status
```

<!-- TODO: DISCUSS -->

I believe we can track all CephCluster key rotations as one and rely on operator logs and error
events for debugging, but we should still separate out CSI rotations since those can be initiated
separately.

```yaml
status:
  # ...
  cephx:
    local:
      keyGeneration: 3 # e.g.
      keyCephVersion: "20.2.2" # e.g.
    csi: {} # discussed more below, but some CSI keys and need to be in this status
```

### CSI example

This is complicated, and I need some input. My understanding is that CSI keys are are always updated
as part of a CephCluster, CephRBDMirror, or CephFilesystemMirror reconcile.
<!-- TODO: is the above true?
    are any keys created in the CSI reconcile logic that's separate from CephCluster or other reconcile routines? -->

For a CephCluster reconcile:

1. Ceph-CSI daemons use a separate config from Ceph local, so the reconcile reads
    `CephCluster.spec.security.cephx.csi` to determine if the reconcile should rotate.
2. Assuming the rotation is indicated, rotate keys.
3. Update the CephCluster status with the CSI key info (with internal retry)

    ```yaml
    kind: CephCluster
    # ...
    status:
      # ...
      cephx:
        # (status from CephCluster local rotations)
        csi:
          keyGeneration: 2 # e.g.
          keyCephVersion: "20.2.1" # e.g.
    ```

`(with internal retry)` this is an important detail we should plan for IFF the CSI reconciliation
routine may be trying to update this value in parallel with CephCluster reconciliation. If there is
no parallelism, the internal retry can (should) be removed from the design.

If CSI also requires CephRBDMirror and CephFilesystemMirror reconciles to rotate keys, Rook must
ensure those reconcile watchers enqueue for `CephCluster.spec.security.cephx.csi` updates. Other
reconcile watchers can ignore `...cephx.csi` updates.

### CephFilesystem example

Rook creates at least 2 MDSes for each CephFilesystem. This is a simple but non-trivial example of
key rotation and an example showing rotation for a non-CephCluster resource.

1. Because MDSes are local daemons, the reconcile reads `CephCluster.spec.security.cephx.local` key
    rotation configs to determine if the reconcile should rotate.
2. When key rotation is indicated, it will rotate keys, first for MDS.a, then MDS.b, etc.
3. Once reconciliation is nearing completion, CephFilesystem status is updated, as so:

    ```yaml
    status:
      # ...
      cephx:
        local: # don't need this distinction, but keeps consistent status API and allows future change
          keyGeneration: 2 # e.g.
          keyCephVersion: "20.2.0" # e.g.
    ```

### CephRbdMirror example
CephRbdMirror keys have both local and non-local keys. For local keys, rotation follows the
    methods explained for CephFilesystem above.

For the non-local (peer) key:

1. Reconciliation reads `spec.security.cephx.peer` configs to determine if reconcile should rotate.
2. When key rotation is indicated, the peer key is rotated.
3. Once reconciliation is nearing completion, the status is updated.

Example showing input spec and output status, for clarity:

```yaml
# ...
spec:
  # ...
  security:
    cephx:
      peer:
        keyRotationPolicy: KeyGeneration
        keyGeneration: 2
  # ...
status:
  # ...
  cephx:
    local:
      keyGeneration: 3
      keyCephVersion: "20.2.2"
    peer:
      # before rotation
      keyGeneration: 1
      keyCephVersion: "20.2.0"
      ## after rotation
      # keyGeneration: 2
      # keyCephVersion: "20.2.2"
```

### CephClient example

CephClient stands out due to its simplicity. It will have one and exactly one key which can serve
any purpose (it is not local nor peer). The interfaces are simplified to reflect this.

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

## Dependencies

Ceph Tentacle (v20) provides a new `ceph auth rotate` command merged here:
https://github.com/ceph/ceph/pull/58121. Rook will rely on this command to rotate keys.

A few CephX technical details that are important for understanding CephX key rotation development
are summarized below. More details here: https://docs.ceph.com/en/latest/dev/cephx/

CephX keys minted by Rook are only used by Ceph for initial daemon connection. Internally, all Ceph
connections use "service" keys with laddered expiration times.

By default, Ceph service keys allow at least 2 hours from key rotation until the client must be
updated with the new key. There are 3 service keys with TTLs 1 hour, 2 hours, and 3 hours from the
time when service keys were last refreshed. Assuming the the first TTL expiration is imminent, it is
still at least 2 hours until the 3-hour TTL expiration.

Ceph does not allow keyrings to contain multiple keys for a given client/daemon. When a key is
rotated, the old key is removed and new key replaces it with no ability to have a 2-valid-keys
transition period.

Ceph's `auth_service_ticket_ttl` and `auth_mon_ticket_ttl` config options allow users to
shorten/lengthen that time as desired/needed. Note that this isn't immediate: it will take an
unknown amount of time for internal service keys to update.

Ceph config `debug_auth=30` provides maximum CephX debug logs, as a development aid.
