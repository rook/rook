---
target-version: release-1.9
---

# Swift and Keystone Integration

## Summary

### Goals

The goal of this proposal is to allow configuring the Swift API and
the Keystone integration of Ceph RGW natively via the Object Store
CRD, which allows native integration of the Rook operated Ceph RGW into
OpenStack clouds.

Both changes are bundled together as one proposal because they will
typically deployed together. It is unlikely to encounter a Swift
object store API outside an OpenStack cloud and in the context of an
OpenStack cloud the Swift API needs to be integrated with the Keystone
authentication service.

The Keystone integration must support the current [OpenStack Identity
API version 3](https://docs.openstack.org/api-ref/identity/v3/).

It must be possible to serve S3 and Swift for the same object store
pool.

It must be possible to [obtain S3 credentials via OpenStack](
https://docs.ceph.com/en/octopus/radosgw/keystone/#keystone-integration-with-the-s3-api).

Any changes to the CRD must be future safe and cleanly allow extension
to further technologies (such as LDAP authentication).

### Non-Goals

* Support for OpenStack Identity API versions below v3. API version v2 has long
  been deprecated and [has been removed in the "queens" version of
  Keystone](https://docs.openstack.org/keystone/xena/contributor/http-api.html)
  which was released in 2018 and is now in extended maintenance mode
  (which means it gets no more points releases and only sporadic bug
  fixes/security fixes).

* Authenticating Ceph RGW to Keystone via admin token (a.k.a. shared secret).
  This is a deliberate choice as [admin tokens should not be used in production environments](
  https://docs.openstack.org/keystone/rocky/admin/identity-bootstrap.html#using-a-shared-secret).

* Support for APIs beside S3 and Swift.

* Interaction of Swift with OBCs is out of scope of this document.  If
  you need to access a bucket created by OBC via Swift you need to
  create a separate `cephobjectstoreuser`, configure its access rights
  to the bucket and use those credentials.

* Support for Kubernetes Container Object Storage (COSI)

* Support for authentication technologies other than Keystone (e.g. LDAP)

* Exposing options that disable security features (e.g. TLS verification)


## Proposal details

The Object Store CRD will have to be extended to accommodate the new
settings.

### Keystone integration

A new optional section `auth.keystone` is added to the Object Store
CRD to configure the keystone integration:

```yaml
auth:
  keystone:
    url: https://keystone:5000/                     [1, 2]
    acceptedRoles: ["_member_", "service", "admin"] [1, 2]
    implicitTenants: swift                          [1]
    tokenCacheSize: 1000                            [1]
    revocationInterval: 1200                        [1]
    serviceUserSecretName: rgw-service-user         [3, 2]
```
Annotations:
* `[1]` These options map directly to [RGW configuration
  options](https://docs.ceph.com/en/octopus/radosgw/config-ref/#keystone-settings),
  the corresponding RGW option is formed by prefixing it with
  `rgw_keystone_` and replacing upper case letters by their lower case
  letter preceded by an underscore. E.g. `tokenCacheSize` maps to
  `rgw_keystone_token_cache_size`.
* `[2]` These settings are required in the `keystone` section if
  present.
* `[3]` The name of the secret containing the credentials for the
  service user account used by RGW. It has to be in the same namespace
  as the object store resource.

The `rgw_keystone_api_version` option is not exposed to the user, as
only version 3 of the OpenStack Identity API is supported for now. If
a newer version of the Openstack Identity should be released at some
point, it will be easy to extend the CR to accommodate it.

The certificate to verify the Keystone endpoint can't be explicitly
configured in Ceph RGW. Instead, the system configuration of the pod
running RGW is used. You can add to the system certificate store via
the `gateway.caBundleRef` setting of the object store resource.

The credentials for the Keystone service account used by Ceph RGW are
supplied in a Secret that contains a mapping of OpenStack [openrc
environment variables](https://docs.openstack.org/python-openstackclient/xena/cli/man/openstack.html#environment-variables).
`password` is the only authentication type that is supported, this is
a limitation of RGW which does not support other Keystone authentication
types. Example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rgw-service-user
data:
  OS_PASSWORD: "horse staples battery correct"
  OS_USERNAME: "ceph-rgw"
  OS_PROJECT_NAME: "admin"
  OS_USER_DOMAIN_NAME: "Default"
  OS_PROJECT_DOMAIN_NAME: "Default"
  OS_AUTH_TYPE: "password"
```

This format is chosen because it is a natural and interoperable way
that keystone credentials are represented and for the supported auth
type it maps naturally to the Ceph RGW configuration.

The following constraints must be fulfilled by the secret:
* `OS_AUTH_TYPE` must be `password` or omitted.
* `OS_USER_DOMAIN_NAME` must equal `OS_PROJECT_DOMAIN_NAME`. This is a
  restriction of Ceph RGW, which does not support configuring separate
  domains for the user and project.
* All openrc variables not in the example above are ignored. The API
  version (`OS_IDENTITY_API_VERSION`) is assumed to be `3` and
  Keystone endpoint `OS_AUTH_URL` is taken from the
  `keystone.apiVersion` configuration in the object store resource.

The mapping to
[RGW configuration options](https://docs.ceph.com/en/octopus/radosgw/config-ref/#keystone-settings)
is done as follows:
* `OS_USERNAME` -> `rgw_keystone_admin_user`
* `OS_PROJECT_NAME` -> `rgw_keystone_admin_project`
* `OS_PROJECT_DOMAIN_NAME`, `OS_USER_DOMAIN_NAME` -> `rgw_keystone_admin_domain`
* `OS_PASSWORD` -> `rgw_keystone_admin_password`

### Swift integration

The currently ignored `gateway.type` option is deprecated and from now on
explicitly ignored by rook.

The other `gateway` settings are kept as they are: They do not directly
relate to Swift or S3 but are common configuration of RGW.

The Swift API is enabled and configured via a new `protocols` section:
```yaml
protocols:
  swift:                      [1]
    accountInUrl: true        [4]
    urlPrefix: /example       [4]
    versioningEnabled: false  [4]
  s3:
    enabled: false            [2]
    authUseKeystone: true     [3]
```
Annotations:
* `[1]` Swift will be enabled, if `protocols.swift` is present.
* `[2]` This defaults to `true` (even if `protocols.s3` is not present
  in the CRD). This maintains backwards compatibility – by default S3
  is enabled.
* `[3]` This option maps directly to the `rgw_s3_auth_use_keystone` option.
  Enabling it allows generating S3 credentials via an OpenStack API call, see the
  [docs](https://docs.ceph.com/en/octopus/radosgw/keystone/#keystone-integration-with-the-s3-api).
   If not given, the defaults of the corresponding RGW option apply.
* `[4]` These options map directly to [RGW configuration
  options](https://docs.ceph.com/en/octopus/radosgw/config-ref/#swift-settings),
  the corresponding RGW option is formed by prefixing it with
  `rgw_swift_` and replacing upper case letters by their lower case
  letter preceded by an underscore. E.g. `urlPrefix` maps to
  `rgw_swift_url_prefix`. They are optional. If not given, the defaults
  of the corresponding RGW option apply.

Access to the Swift API is granted by creating a subuser of an RGW
user. While commonly access is granted via projects
mapped from Keystone, explicit creation of subusers is supported by
extending the `cephobjectstoreuser` resource with a new optional section
`spec.subUsers`:
```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: my-display-name
  quotas:
    maxBuckets: 100
    maxSize: 10G
    maxObjects: 10000
  capabilities:
    user: "*"
    bucket: "*"
  subUsers:
  - name: swift  [1]
    access: full [2]
```

Annotations:
* `[1]` This is the name of the subuser without the `username:` prefix
  (see below for more explanation). The `name` must be unique within
  the `subUsers` list.

* `[2]` The possible values are: `read`, `write`, `readwrite`,
  `full`. These values take their meanings from the possible values of
  the `--access-level` option of `radosgw-admin subuser create`, as
  documented in the [radosgw admin guide](
  https://docs.ceph.com/en/octopus/radosgw/admin/#create-a-subuser).

* `name` and `access` are required for each item in `subUsers`.

When changing the subuser-configuration in the CR, this is reflected
on the RGW side:
* Subsers are deleted and created to match the list of subusers in the
  resource.
* If the access level for an existing user is changed no new
  credentials are created, but the existing credentials are kept.
* If a subuser is deleted the corresponding credential secret is
  deleted as well.
* Changing only the order of the subuser list does not trigger a
  reconcile.

The subusers are not mapped to a separate CR for the
following reasons:

* The full subuser names are prefixed with the username like
  `my-user:subusername`, so being unique within the CR guarantees
  global uniqueness.

  Unlike `radosgw-admin` the subuser name in the CRD must be provided
  *without the prefix* (radosgw-admin allows both, to omit or include
  the prefix).

* The subuser abstraction is very simple – only a name and an access
  level can be configured, so a separate resource would not be
  appropriate complexity wise.

Like for the S3 access keys for the users, the swift keys created for the
sub-users will be automatically injected into Secret objects. The
credentials for the subusers are mapped to separate secrets, in the
case of the example the following secret will be created:
```yaml
apiVersion:
kind: Secret
metadata:
  name: rook-ceph-object-subuser-my-store-my-user-swift [1]
  namespace: rook-ceph
data:
  SWIFT_USER: my-user:swift                             [2]
  SWIFT_SECRET_KEY: $KEY                                [3]
  SWIFT_AUTH_ENDPOINT: https://rgw.example:6000/auth    [4]
```
Annotations:
* `[1]` The name is constructed by joining the following elements with dashes
  (compare [the corresponding name of the secret for the object store users](
  https://github.com/rook/rook/blob/376ca62f8ad07540d9ddffe9dc0ee53f4ac35e29/pkg/operator/ceph/object/user/controller.go#L416)):
      - the literal `rook-ceph-object-subuser`
      - the name of the object store resource
      - the name of the user
      - the name of the subuser (without `username:`-prefix)
* `[2]` The full name of the subuser (including the `username:`-prefix).
* `[3]` The generated swift access secret.
* `[4]` The API endpoint for [swift auth](https://docs.ceph.com/en/octopus/radosgw/swift/auth/#auth-get).

### Risks and Mitigation

As long as the Object Store CRD changes are well thought out the
overall risk is minimal.

* If Swift is enabled by accident this could lead to an increased and
  unexpected attack surface, especially since the authentication and
  authorization for Swift and S3 may differ depending on how Ceph RGW
  is configured. This is mitigated by requiring explicit configuration
  to enable Swift.

* A misconfigured Keystone integration may allow users to gain access
  to objects they should not be authorized to access (e.g. if the
  `rgw keystone accepted roles` setting is too broad). This will be
  mitigated by proper documentation to make the operator aware of the
  risk.

* Ceph RGW allows to disable TLS verification when querying Keystone,
  we deliberately choose not to expose this config option to the user.

* The mapping from store, username and subuser-name to the name of the
  secret with the credentials is not injective. This means that the
  subusers of two different users may map to the same secret
  (e.g. `user:a-b` and `user-a:b`).

  This is potentially a vector for leaks of credentials to
  unauthorized entities. A simple workaround is to avoid dashes in the
  names of users and subuser managed by the CephObjectStoreUser CR.

  Documenting the problem is deemed sufficient since a similar
  problem already exists for the secret created for the users (in that
  case for users from different object stores, e.g. the secret for
  user `foo` in `my-store` collides with the one for user `store-foo`
  in `my`).

## Drawbacks

* As shown in [#4754](https://github.com/rook/rook/issues/4754)
  keystone can be integrated via config override so
  it is not strictly necessary to support configuring it via the
  Object Store CRD. Adding it to the CRD complicates things with
  minor gain.

## Alternatives

* There is no workable alternative to extending the CRD for Swift
  support.

* Keystone support could be configured via Rook config override as shown
  in [#4754](https://github.com/rook/rook/issues/4754).

## Open Questions
