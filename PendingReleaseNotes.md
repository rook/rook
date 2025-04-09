# v1.17 Pending Release Notes

## Breaking Changes

Object:

- Some ObjectBucketClaim options were added in Rook v1.16 that allowed more control over buckets.
    These controls allow users to self-serve their own S3 policies, which many administrators might
    consider a risk, depending on their environment. Rook has taken steps to ensure potentially risky
    configurations are disabled by default to ensure the safest off-the-shelf configurations.
    Administrators who wish to allow users to use the full range of OBC configurations must use the
    new `ROOK_OBC_ALLOW_ADDITIONAL_CONFIG_FIELDS` to enable users to set potentially risky options.
    See https://github.com/rook/rook/pull/15376 for more information.

- Add first-class credential management to CephObjectStoreUser. Existing S3 users provisioned via
    CephObjectStoreUser resources no longer allow multiple credentials to exist on underlying S3
    users, unless explicitly managed by Rook. Rook will purge all but one of the undeclared
    credentials. This could be a user observable regression for administrators who manually
    edited/rotated S3 user credentials for CephObjectStoreUsers. Affected users should make use of
    the new credential management feature instead.
    For more details, see [#15359](https://github.com/rook/rook/issues/15359).

- Kafka notifications configured via CephBucketTopic resources will now default
    to setting the Kafka authentication mechanism to `PLAIN`. Previously, no auth
    mechanism was specified by default.  It was possible to set the auth mechanism
    via `CephBucketTopic.spec.endpoint.kafka.opaqueData`.  However, setting
    `&mechanism=<auth type>` via `opaqueData` is no longer possible. If any auth
    mechanism other than `PLAIN` is in use, modification to `CephBucketTopic`
    resources is required.

## Features

- Support external mons for local Rook cluster (see [#14733](https://github.com/rook/rook/issues/14733)).
- Manage EndpointSlice resources containing monitor IPs to support DNS-based resolution for Ceph clients (see [#14986](https://github.com/rook/rook/issues/14986)).
