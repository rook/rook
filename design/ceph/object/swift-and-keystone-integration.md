---
title: swift-and-keystone
target-version: release-X.X
---

# Swift and Keystone Integration

## Summary

### Goals

The goal of this proposal is to allow configuring the Swift API and
the Keystone integration of Ceph RGW natively via the Object Store
CRD, which allows native integration of the Rook operated Ceph RGW into
OpenStack Clouds.

Both changes are bundled together as one proposal because they will
typically deployed together. It is unlikely to encounter a Swift
object store API outside an OpenStack cloud and in the context of an
OpenStack cloud the Swift API needs to be integrated with the Keystone
authentication service.

Any changes to the CRD must be future safe and cleanly allow extension
to further technologies (such as LDAP authentication).

### Non-Goals

* Support for Keystone API versions below v3. API version v2 has long
  been deprecated [has been removed in
  Queens](https://docs.openstack.org/keystone/latest/contributor/http-api.html)
  which was release in 2018 and is now in extended maintenance mode
  (which means it gets no more points releases and only sporadic bug
  fixes/security fixes).

* Authenticating Ceph RGW to Keystone via admin token – Only
  authentication via an OpenStack service account will be supported.

* Support for APIs beside S3 and Swift.

* Support for authentication technologies other than Keystone (e.g. LDAP)

* Exposing options that disable security features (e.g. TLS verification)

## Proposal details

The Object Store CRD will have to be extended to accommodate the new
settings.

### Keystone integration

A new section `auth:` is added to the Object Store CRD. To configure
the keystone integration:

```yaml
auth:
  keystone:
    url: https://keystone:5000/
    acceptedRoles: ["_member_", "service", "admin"]
    apiVersion: 3
    implicitTenants: swift
    tokenCacheSize: 1000
    revocationInterval: 1200
    serviceUserSecret: rgw-service-user
```

The credentials for the Keystone service account used by Ceph RGW are
supplied in a Secret that contains a mapping of OpenStack openrc
environment variables. Only password authentication to Keystone is
supported. Example:

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
* `OS_USER_DOMAIN_NAME` must equal `OS_PROJECT_DOMAIN_NAME`.
* All other openrc variables (e.g. API version and endpoint) are ignored.

### Swift integration

Swift is configured in the `gateway:` section of the Object Store CRD.

The currently unused `type:` argument may now take the values `swift`
or `s3`.

The following new settings are available to properly configure Swift:
```yaml
gateway:
  type: swift
  swiftAccountInUrl: true
  swiftUrlPrefix: /swifter
  swiftVersioningEnabled: false
```

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

## Drawbacks

* As shown in #4754 keystone can be integrated via config override so
  it is not strictly necessary to support configuring it via the
  Object Store CRD. Adding it to the CRD complicates things with
  minor gain.

## Alternatives

* There is no workable alternative to extending the CRD for Swift
  support.

* Keystone support could be configured via Rook config override as shown
  in #4754.

## Open Questions

### How to support multiple APIs in one Gateway

Having a single Ceph RGW deployment serve both S3 and Swift is both
possible and often desirable for interoperability reasons. The current
structure of the Object Store CRD makes it difficult to represent the
configuration for this cleanly.

Possible Solutions:

* Run a second object store resource. The question is then how to use
  the same pools (since the pools are derived from the resource name
  which must be unique)

* Simply allow a list in the type argument (but keep allowing strings
  for backwards compatibility). The drawback here is that the gateway
  config will then be cluttered by the config options pertaining to
  different API types.

* Break compatibility with the existing CRD and split up the
  `gateway:` configuration.

* Keep only the common things in `gateway:` and add an own config
  section for s3 and swift (those could be subsections of `gateway:`
  or of new section, e.g. `apis:`). The `type:` option seems to be currently
  ignored anyway. The config values are then merged from the
  `gateway:`, the `s3:` and the `swift:` sections, with the values in
  the specific sections taking precedence.
