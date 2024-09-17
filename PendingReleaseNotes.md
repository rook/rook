# v1.16 Pending Release Notes

## Breaking Changes

- Removed support for Ceph Quincy (v17) since it has reached end of life

## Features

- Enable mirroring for CephBlockPoolRadosNamespaces (see [#14701](https://github.com/rook/rook/pull/14701)).
- Enable periodic monitoring for CephBlockPoolRadosNamespaces mirroring (see [#14896](https://github.com/rook/rook/pull/14896)).
- Allow migration of PVC based OSDs to enable or disable encryption (see [#14776](https://github.com/rook/rook/pull/14776)).
