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

## Features

- Support external mons for local Rook cluster (see [#14733](https://github.com/rook/rook/issues/14733)).
- Manage EndpointSlice resources containing monitor IPs to support DNS-based resolution for Ceph clients (see [#14986](https://github.com/rook/rook/issues/14986)).
