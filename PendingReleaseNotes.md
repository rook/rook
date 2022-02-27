# v1.9 Pending Release Notes

## Breaking Changes

*  The mds liveness and startup probes are now configured by the filesystem CR instead of the cluster CR. To apply the mds probes, they need to be specified in the filesystem CR. See the [filesystem CR doc](Documentation/ceph-filesystem-crd.md#metadata-server-settings) for more details. 
Pr: https://github.com/rook/rook/pull/9550

## Features

### Ceph
- Prometheus Rule alerts can be customized by user preference.
