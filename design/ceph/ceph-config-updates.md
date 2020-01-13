# Updating Rook's Ceph configuration strategy (Jan 2019)
**Targeted for v1.0-v1.1**

## Background

Starting with Ceph Mimic, Ceph is able to store the vast majority of config options for all daemons
in the Ceph mons' key-value store. This ability to centrally manage 99% of its configuration
is Ceph's preferred way of managing config. This conveniently allows Ceph to fit better into the
containerized application space than before.

However, for it's own backwards compatibility, Ceph options set in the config file or via command
line flags will override the centrally-configured settings. To make the most of this functionality
within Ceph, it will be necessary to limit the configuration options specified by Rook in either
config files or on the command line to a minimum.


## Proposed new design

The end goal of this work will be to allow Ceph to centrally manage its own configuration as much as
possible. Or in other terms, Rook will specify the barest minimum of configuration options in the
config file or on the command line with a priority on clearing the config file.

### Minimal config file

All Ceph options in the config file can be set via the command line, so it is possible to remove the
need for having a `ceph.conf` in containers at all. This is preferred over a config file, as it is
possible to inspect the entire config a daemon is started with by looking at the pod description.

Some parts of the config file will have to be kept as long as Rook supports Ceph Luminous. See the
[supporting Luminous](#supporting-luminous) section below for more details.

### Minimal command line options

The minimal set of command line options is pared down to the settings which inform the daemon where
to find and/or store data.

Required flags:
- `--fsid` is not required but is set to ensure daemons will not connect to the wrong cluster
- `--mon-host` is required to find mons, and when a pod (re)starts it must have the latest
  information. This can be achieved by storing the most up-to-date mon host members in a Kubernetes
  ConfigMap and setting this value a container environment variable mapped to the ConfigMap value.
- `--{public,cluster}-addr` are not required for most daemons but ...
  - `--public-addr` and `--public-bind-addr` are necessary for mons
  - `--public-addr` and `--cluster-addr` may be needed for osds
- `--keyring` may be necessary to inform daemons where to find the keyring which must be mounted to
  a nonstandard directory by virtue of being sourced via a Kubernetes secret.
  - The keyring could copied from the secret mount location to Ceph's default location with an init
    container, and for some daemons, this may be necessary. This should not be done for the mons,
    however, as the mon keyrings also include the admin keyring, and persisting the admin key to
    disk should be avoided at all costs for security.

Notable non-required flags:
- `--{mon,osd,mgr,mds,rgw}-data-dir` settings exist for all daemons, but it is more desirable to use
  the `/var/lib/ceph/<daemon_type>/ceph-<daemon_id>` directory for daemon data within containers. If
  possible, mapping the `dataDirHostPath/<rook_daemon_data_dir>` path on hosts to this default
  location in the container is preferred.
  - Note that currently, `dataDirHostPath` is mapped directly to containers, meaning that each
    daemon container has access to other daemon containers' host-persisted data. Modifying Rook's
    behavior to only mount the individual daemon's data dir into the container as proposed here will
    be a small security improvement on the existing behavior.
- `--run-dir` exists for all daemons, but it is likewise more desirable to use the `/var/run/ceph`
  path in containers. Additionally, this directory stores only unix domain sockets, and it does not
  need to be persisted to the host. We propose to simply use the `/var/run/ceph` location in
  containers for runtime storage of the data.

### Additional configuration

Additional configuration which Rook sets up initially should be done by setting values in Ceph's
centrally-stored config. A large group of additional configurations can be configured at once via
the Ceph command `ceph config assimilage-conf`. Care should be taken to make sure that Rook does not
modify preexisting user-specified values.

In the initial version of this implementation, Rook will set these values on every operator restart.
This may result in user configs being overwritten but will ensure the user is not able to render
Rook accidentally unusable. In the future, means of determining whether a user has specified a value
or whether Rook has specified it is desired which may mean a feature addition to Ceph.

Existing global configs configured here:
- `mon allow pool delete = true`
- `fatal signal handlers = false` is configured here, but this could be a vestigial config from
  Rook's old days that can be removed (some more research needed)
- `log stderr prefix = "debug "` should be set for all daemons to differentiate logging from auditing
- `debug ...` configs

Removed configs:
- `mon_max_pg_per_osd = 1000` is a dangerous setting and should be removed regardless of whether
  this proposal is accepted
- `log file = /dev/stderr` is set by default to keep with container standards and kept here if the
  user needs to change this for debugging/testing
- `mon cluster log file = /dev/stderr` for `log file` reasons above
- `mon keyvaluedb = rocksdb` is not needed for Luminous+ clusters
- `filestore_omap_backend = rocksdb` is not needed for Luminous+
- `osd pg bits = 11` set (if needed) using config override for testing or play clusters
- `osd pgp bits = 11` set (if needed) using config override for testing or play clusters
- `osd pool default size = 1` set (if needed) using config override for testing or play clusters
- `osd pool default min size = 1` set (if needed) using config override for testing or play clusters
- `osd pool default pg num = 100` set (if needed) using config override for testing or play clusters
- `osd pool default pgp num = 100` set (if needed) using config override for testing or play clusters
- `rbd_default_features = 3` kubernetes should support Ceph's default RBD features after k8s v1.8

#### Additional configs via user override

Rook currently offers the option of a config override in a ConfigMap which users may modify after
the Ceph operator has started. We propose to keep the "spirit" of this functionality but change the
method of implementation, as the ConfigMap modification approach will be hard to integrate with the
final goal of eliminating the config file altogether. Instead, we propose to update the Ceph cluster
CRD to support setting and/or overriding values at the time of cluster creation. The proposed format
is below.

```yaml
apiVersion: ceph.rook.io/v2alpha1
kind: CephCluster
spec:
  # ...
  # For advanced users:
  # 'config' adds or overrides values in the Ceph config at operator start time and when the cluster
  # CRD is updated. Config changes are made in the mon's centralized config if it is available
  # (Mimic+) so that the user may override them temporarily via Ceph's command line. For Luminous,
  # the changes are set on the command line since the centralized config is not available, and
  # temporary overrides will not be possible.
  config:
    # Each key in the 'config' section represents a config file section. 'global' is likely to be
    # the only section which is modified; however, daemons can have their config overridden
    # explicitly if desired.
    # global will add/override config for all Ceph daemons
    global:
      # The below "osd_pool_default..." settings make the default pools created have no replication
      # and should be removed for production clusters, as this could impact data fault tolerance.
      osd_pool_default_size: 1
    # mon will add/override config for all mons
    mon:
      mon_cluster_log_file: "/dev/stderr"
    # osd.0 will add/override config only for the osd with ID 0 (zero)
    osd.0:
      debug_osd: 10
   # ...
```
**Note on the above:** all values under config are reported to Ceph as strings, but the yaml should
support integer values as well if at all possible

As stated in the example yaml, above, the 'config' section adds or overrides values in the Ceph
config whenever the Ceph operator starts and whenever the user updates the cluster CRD. Ceph
Luminous does not have a centralized config, so the overrides from this section will have to be set
on the command line. For Ceph Mimic and above, the mons have a centralized config which will be used
to set/override configs. Therefore, for Mimic+ clusters, the user may temporarily override values
set here, and those values will be reset to the `spec:config` values whenever the Ceph operator is
restarted or the cluster CRD is updated.

#### Additional configs for test/play environments

Test (especially integration tests) may need to specify `osd pool default size = 1` and
`osd pool default min size = 1` to support running clusters with only one osd. Test environments
would have a means of doing this fairly easily using the config override capability. These values
should not be set to these low values for production clusters, as they may allow admins to create
their own pools which are not fault tolerant accidentally.

There is an option to set these values automatically for clusters which run with only one osd or to
set this value for clusters with a number of osds less than the default programmatically within
Rook's operator; however, this adds an additional amount of code flow complexity which is
unnecessary except in the integration test environments or in minimal demo environments. A middle
ground proposed herein is to add `osd pool default {,min} size = 1` overrides to the example cluster
CRD so that users "just trying out Rook" still get today's easy experience but where they can be
easily removed for production clusters that should not run with potentially dangerous settings.

### Changes to Ceph mons

The current method of starting mons where `mon-a` has an initial member of `a`, `mon-b` initial
members `a b`, `mon-c` initial members `a b c`, etc. Has worked so far but could result in a race
condition. Mon cluster stability is important to Ceph, and it is critical for this PR that the mons'
centrally-stored config is stable, so we here note that this behavior should be fixed such that the
mon initial members are known before the first mon is bootstrapped to consider this proposal's work
completed. Practically speaking, this will merely require the mon services to be started and have
IP addresses before the mons are bootstrapped.

Additionally, generating the monmap during mon daemon initialization is unnecessary if `--mon-host`
is set for the `ceph-mon --mkfs` command.

Creating `/var/lib/ceph/mon-<ID>/data/kv_backend` is no longer necessary in Luminous and
can be removed.


## Planning changes

This proposal herein makes the suggestion that the changes be done with a new PR for each daemon
starting with the mons, as the mons are most affected. After the mons are done, the remaining 4
daemons can be done in parallel.

Once all 5 daemons are complete, there will likely be a need to refactor the codebase to remove any
vestigial remnants of the old config design which have been left. It will also be a good time to
look for any additional opportunities to reduce code duplication by teasing repeated patterns out
into shared modules.

Another option is to modify all 5 daemons such that support is focused on Luminous, and the final
clean-up stage could be a good time to introduce support for Mimic and its new centralized mon KV
all at once.

### Supporting Luminous

Luminous does not have the mon's centralized kv store for Ceph configs, so any config set in the mon
kv store should be set in the config file for Luminous, and users may override these values via
Rook's config override feature.

### Secondary considerations

The implementation of this work will naturally remove most of the need for Rook to modify Ceph
daemon configurations via its `config-init` code paths, so it will also be a good opportunity to
move all daemon logic into the operator process where possible.


## Appendix A - at-a-glance config changes compared to Rook's v0.9 Ceph config file
```
NEW LOCATION
----------------
REMOVED            [global]
FLAG               fsid                      = bd4e8c5b-80b8-47d5-9e39-460eccc09e62
REMOVED            run dir                   = /var/lib/rook/mon-c
FLAG AS NEEDED     mon initial members       = b c a
FLAG               mon host                  = 172.24.191.50:6790,172.24.97.67:6790,172.24.123.44:6790
MON KV             log file                  = /dev/stderr
MON KV             mon cluster log file      = /dev/stderr
FLAG AS NEEDED     public addr               = 172.24.97.67
FLAG AS NEEDED     cluster addr              = 172.16.2.122
FLAG AS NEEDED     public network            = not currently used
FLAG AS NEEDED     cluster network           = not currently used
REMOVED            mon keyvaluedb            = rocksdb
MON KV             mon_allow_pool_delete     = true
REMOVED            mon_max_pg_per_osd        = 1000
MON KV             debug default             = 0
MON KV             debug rados               = 0
MON KV             debug mon                 = 0
MON KV             debug osd                 = 0
MON KV             debug bluestore           = 0
MON KV             debug filestore           = 0
MON KV             debug journal             = 0
MON KV             debug leveldb             = 0
OVERRIDE           filestore_omap_backend    = rocksdb
OVERRIDE           osd pg bits               = 11
OVERRIDE           osd pgp bits              = 11
OVERRIDE           osd pool default size     = 1
OVERRIDE           osd pool default min size = 1
OVERRIDE           osd pool default pg num   = 100
OVERRIDE           osd pool default pgp num  = 100
REMOVED            rbd_default_features      = 3
MON KV / REMOVED?  fatal signal handlers     = false

REMOVED            [daemon.id]
FLAG AS NEEDED     keyring = /var/lib/rook/mon-c/data/keyring
```

New location key:
```
 - REMOVED        - removed entirely from the config
 - FLAG           - flag always set
 - FLAG AS NEEDED - set as a command line flag if/when it is needed
 - MON KV         - store in the mon's central config (except for Luminous)
 - OVERRIDE       - removed but will need to be added in override for some scenarios (test/play)
```
