---
title: Security
---

## User ID mapping
User ID mapping allows the NFS server to map connected NFS client IDs to a different user domain,
allowing NFS clients to be associated with a particular user in your organization. For example,
users stored in LDAP can be associated with NFS users and vice versa.

!!! attention
    This feature is experimental and may not support upgrades to future versions.

!!! attention
    Some configurations of this feature may break the ability to
    [mount NFS storage to pods via PVCs](./nfs-csi-driver.md#attaching-an-export-to-a-pod).
    The NFS CSI driver may not be able to mount exports for pods when ID mapping is configured.

### ID mapping via SSSD

[SSSD](https://sssd.io) is the System Security Services Daemon. It can be used to provide user ID
mapping from a  number of sources including LDAP, Active Directory, and FreeIPA. Currently, only
LDAP has been tested.

!!! attention
    The Ceph container image must have the `sssd-client` package installed to support SSSD. This
    package is included in `quay.io/ceph/ceph` in v17.2.4 and newer. For older Ceph versions you may
    build your own Ceph image which adds `RUN yum install sssd-client && yum clean all`.

#### **SSSD configuration**

SSSD requires a configuration file in order to configure its connection to the user ID mapping
system (e.g., LDAP). The file follows the `sssd.conf` format documented in its
[man pages](https://linux.die.net/man/5/sssd.conf).

Methods of providing the configuration file are documented in the
[NFS CRD security section](../../CRDs/ceph-nfs-crd.md#security).

The SSSD configuration file may be omitted from the CephNFS spec if desired. In this case, Rook will
not set the `/etc/sssd/sssd.conf` in any way. This allows you to manage the `sssd.conf` file
yourself however you wish. For example, you may build it into your custom Ceph container image, or
use the [Vault agent injector](https://www.vaultproject.io/docs/platform/k8s/injector) to securely
add the file via annotations on the CephNFS spec (passed to the NFS server pods).

A sample `sssd.conf` file is shown below.
```ini
[sssd]
# Only the nss service is required for the SSSD sidecar.
services = nss
domains = default
config_file_version = 2

[nss]
filter_users = root

[domain/default]
id_provider = ldap
ldap_uri = ldap://server.address
ldap_search_base = dc=company,dc=net
ldap_default_bind_dn = cn=admin,dc=company,dc=net
ldap_default_authtok_type = password
ldap_default_authtok = my-password
ldap_user_search_base = ou=users,dc=company,dc=net
ldap_group_search_base = ou=groups,dc=company,dc=net
ldap_access_filter = memberOf=cn=rook,ou=groups,dc=company,dc=net
# recommended options for speeding up LDAP lookups:
enumerate = false
ignore_group_members = true
```

Notes:
- The SSSD sidecar only requires the namespace switch (a.k.a. "nsswitch" or "nss"). We recommend
  enabling only the `nss` service to lower CPU usage.
- NFS-Ganesha does not require user enumeration. We recommend leaving this option unset or setting
  `enumerate = false` to speed up lookups and reduce RAM usage.
- NFS exports created via documented methods do not require listing all members of groups. We
  recommend setting `ignore_group_members = true` to speed up LDAP lookups. Only customized exports
  that set `manage_gids` need to consider this option.
