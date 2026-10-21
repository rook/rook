# v1.22 Pending Release Notes

Notes for the v1.22.0 release, including breaking changes and new features.
Changes should *not* be included here if they are backported to previous releases.

## Breaking Changes


## Features

- Restricted external-cluster exports now give the healthchecker a
  consumer-specific CephX entity and scope its RBD permissions to the
  requested pool (and RADOS namespace, when specified). Existing broad
  healthchecker users are not modified in place; generate new restricted
  credentials for each consumer.
