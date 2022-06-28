---
title: Configuration
---

For most any Ceph cluster, the user will want to--and may need to--change some Ceph
configurations. These changes often may be warranted in order to alter performance to meet SLAs or
to update default data resiliency settings.

!!! warning
    Modify Ceph settings carefully, and review the
    [Ceph configuration documentation](https://docs.ceph.com/docs/master/rados/configuration/) before
    making any changes.
    Changing the settings could result in unhealthy daemons or even data loss if
    used incorrectly.

## Required configurations

Rook and Ceph both strive to make configuration as easy as possible, but there are some
configuration options which users are well advised to consider for any production cluster.

### Default PG and PGP counts

The number of PGs and PGPs can be configured on a per-pool basis, but it is highly advised to set
default values that are appropriate for your Ceph cluster. Appropriate values depend on the number
of OSDs the user expects to have backing each pool. The Ceph [OSD and Pool config
docs](https://docs.ceph.com/docs/master/rados/operations/placement-groups/#a-preselection-of-pg-num)
provide detailed information about how to tune these parameters: `osd_pool_default_pg_num` and `osd_pool_default_pgp_num`.

Nautilus [introduced the PG auto-scaler mgr module](https://ceph.com/rados/new-in-nautilus-pg-merging-and-autotuning/)
capable of automatically managing PG and PGP values for pools. Please see
[Ceph New in Nautilus: PG merging and autotuning](https://ceph.io/rados/new-in-nautilus-pg-merging-and-autotuning/)
for more information about this module.

In Octopus (v15.2.x) and newer, module `pg_autoscaler` is enabled by default without the above-mentioned setting.

To disable this module, in the [CephCluster CR](../../CRDs/Cluster/ceph-cluster-crd.md#mgr-settings):

```yaml
spec:
  mgr:
    modules:
    - name: pg_autoscaler
      enabled: false
```

With that setting, the autoscaler will be enabled for all new pools. If you do not desire to have
the autoscaler enabled for all new pools, you will need to use the Rook toolbox to enable the module
and [enable the autoscaling](https://docs.ceph.com/docs/master/rados/operations/placement-groups/)
on individual pools.

The autoscaler is not enabled for the existing pools after enabling the module. So if you want to
enable the autoscaling for these existing pools, they must be configured from the toolbox.

## Specifying configuration options

### Toolbox + Ceph CLI

The most recommended way of configuring Ceph is to set Ceph's configuration directly. The first
method for doing so is to use Ceph's CLI from the Rook-Ceph toolbox pod. Using the toolbox pod is
detailed [here](../../Troubleshooting/ceph-toolbox.md). From the toolbox, the user can change Ceph configurations, enable
manager modules, create users and pools, and much more.

### Ceph Dashboard

The Ceph Dashboard, examined in more detail [here](../Monitoring/ceph-dashboard.md), is another way of setting
some of Ceph's configuration directly. Configuration by the Ceph dashboard is recommended with the
same priority as configuration via the Ceph CLI (above).

### Advanced configuration via ceph.conf override ConfigMap

Setting configs via Ceph's CLI requires that at least one mon be available for the configs to be
set, and setting configs via dashboard requires at least one mgr to be available. Ceph may also have
a small number of very advanced settings that aren't able to be modified easily via CLI or
dashboard. The **least** recommended method for configuring Ceph is intended as a last-resort
fallback in situations like these. This is covered in detail
[here](ceph-configuration.md#custom-cephconf-settings).
