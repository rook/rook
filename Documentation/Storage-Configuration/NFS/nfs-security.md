---
title: Security
---

Rook provides security for CephNFS server clusters through two high-level features:
[user ID mapping](#user-id-mapping) and [user authentication](#user-authentication).

!!! attention
    All features in this document are experimental and may not support upgrades to future versions.

!!! attention
    Some configurations of these features may break the ability to
    [mount NFS storage to pods via PVCs](./nfs-csi-driver.md#attaching-an-export-to-a-pod).
    The NFS CSI driver may not be able to mount exports for pods when ID mapping is configured.


## User ID mapping

User ID mapping allows the NFS server to map connected NFS client IDs to a different user domain,
allowing NFS clients to be associated with a particular user in your organization. For example,
users stored in LDAP can be associated with NFS users and vice versa.

### ID mapping via SSSD

[SSSD](https://sssd.io) is the System Security Services Daemon. It can be used to provide user ID
mapping from a  number of sources including LDAP, Active Directory, and FreeIPA. Currently, only
LDAP has been tested.

!!! attention
    The Ceph container image must have the `sssd-client` package installed to support SSSD. This
    package is included in `quay.io/ceph/ceph` in v17.2.4 and newer. For older Ceph versions you may
    build your own Ceph image which adds `RUN yum install sssd-client && yum clean all`.

#### SSSD configuration

SSSD requires a configuration file in order to configure its connection to the user ID mapping
system (e.g., LDAP). The file follows the `sssd.conf` format documented in its
[man pages](https://linux.die.net/man/5/sssd.conf).

Methods of providing the configuration file are documented in the
[NFS CRD security section](../../CRDs/ceph-nfs-crd.md#security).

Recommendations:

- The SSSD sidecar only requires the namespace switch (a.k.a. "nsswitch" or "nss"). We recommend
    enabling only the `nss` service to lower CPU usage.
- NFS-Ganesha does not require user enumeration. We recommend leaving this option unset or setting
    `enumerate = false` to speed up lookups and reduce RAM usage.
- NFS exports created via documented methods do not require listing all members of groups. We
    recommend setting `ignore_group_members = true` to speed up LDAP lookups. Only customized exports
    that set `manage_gids` need to consider this option.

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
ldap_uri = ldap://server-address.example.net
ldap_search_base = dc=example,dc=net
ldap_default_bind_dn = cn=admin,dc=example,dc=net
ldap_default_authtok_type = password
ldap_default_authtok = my-password
ldap_user_search_base = ou=users,dc=example,dc=net
ldap_group_search_base = ou=groups,dc=example,dc=net
ldap_access_filter = memberOf=cn=rook,ou=groups,dc=example,dc=net
# recommended options for speeding up LDAP lookups:
enumerate = false
ignore_group_members = true
```

The SSSD configuration file may be omitted from the CephNFS spec if desired. In this case, Rook will
not set `/etc/sssd/sssd.conf` in any way. This allows you to manage the `sssd.conf` file yourself
however you wish. For example, you may build it into your custom Ceph container image, or use the
[Vault agent injector](https://www.vaultproject.io/docs/platform/k8s/injector) to securely add the
file via annotations on the CephNFS spec (passed to the NFS server pods).


## User authentication

User authentication allows NFS clients and the Rook CephNFS servers to authenticate with each other
to ensure security.

### Authentication through Kerberos

Kerberos is the authentication mechanism natively supported by NFS-Ganesha. With NFSv4, individual
users are authenticated and not merely client machines.

#### Kerberos configuration

Kerberos authentication requires configuration files in order for the NFS-Ganesha server to
authenticate to the Kerberos server (KDC). The requirements are two-parted:

1. one or more kerberos configuration files that configures the connection to the Kerberos server.
    This file follows the `krb5.conf` format documented in its
    [man pages](https://linux.die.net/man/5/krb5.conf).
2. a keytab file that provides credentials for the
    [service principal](#nfs-service-principals) that NFS-Ganesha will use to authenticate with the
    Kerberos server.
3. a kerberos domain name which will be used to map kerberos credentials to uid/gid
    [domain name](#kerberos-domain-name) that NFS-Ganesha will use to authenticate with the

Methods of providing the configuration files are documented in the
[NFS CRD security section](../../CRDs/ceph-nfs-crd.md#security).

Recommendations:

- Rook configures Kerberos to log to stderr. We suggest removing logging sections from config files
    to avoid consuming unnecessary disk space from logging to files.

A sample Kerberos config file is shown below.

```ini
[libdefaults]
default_realm = EXAMPLE.NET

[realms]
EXAMPLE.NET = {
kdc = kdc.example.net:88
admin_server = kdc.example.net:749
}

[domain_realm]
.example.net = EXAMPLE.NET
example.net = EXAMPLE.NET
```

The Kerberos config files (`configFiles`) may be omitted from the Ceph NFS spec if desired. In this
case, Rook will not add any config files to `/etc/krb5.conf.rook/`, but it will still configure
Kerberos to load any config files it finds there. This allows you to manage these files yourself
however you wish.

Similarly, the keytab file (`keytabFile`) may be  omitted from the CephNFS spec if  desired. In this
case, Rook will not set `/etc/krb5.keytab` in any way. This allows you to manage the `krb5.keytab`
file yourself however you wish.

As an example for either of the above cases, you may build files into your custom Ceph container
image or use the [Vault agent injector](https://www.vaultproject.io/docs/platform/k8s/injector) to
securely add files via annotations on the CephNFS spec (passed to the NFS server pods).

#### NFS service principals

The Kerberos service principal used by Rook's CephNFS servers to authenticate with the Kerberos
server is built up from 3 components:

1. the configured from `spec.security.kerberos.principalName` that acts as the service name
2. the hostname of the server on which NFS-Ganesha is running which is in turn built up from the
    namespace and name of the CephNFS resource, joined by a hyphen. e.g., `rooknamespace-nfsname`
3. the realm as configured by the kerberos config file(s) from `spec.security.kerberos.configFiles`

The full service principal name is constructed as `<principalName>/<namespace>-<name>@<realm>`. For
ease of scaling up or down CephNFS clusters, this principal is used for all servers in the CephNFS
cluster.

Users must add this service principal to their Kerberos server configuration.

!!! example
    For a CephNFS named "fileshare" in the "business-unit" Kubernetes namespace that has a
    `principalName` of "sales-apac" and where the Kerberos realm is "EXAMPLE.NET", the full
    principal name will be `sales-apac/business-unit-fileshare@EXAMPLE.NET`.

!!! advanced
    `spec.security.kerberos.principalName` corresponds directly to NFS-Ganesha's
    NFS_KRB5:PrincipalName config. See the
    [NFS-Ganesha wiki](https://github.com/nfs-ganesha/nfs-ganesha/wiki/RPCSEC_GSS) for more details.

#### Kerberos domain name

The kerberos domain name is used to setup the domain name in /etc/idmapd.conf. This domain name is used
by idmap to map the kerberos credential to the user uid/gid. Without this configured, NFS-Ganesha will
be unable to map the Kerberos principal to an uid/gid and will instead use the configured
anonuid/anongid (default: -2) when accessing the local filesystem.
