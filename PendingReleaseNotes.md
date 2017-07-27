The latest upstream contains the following changes since v0.4:

### Kubernetes
- Names of the deployments, services, daemonsets, pods, etc are named more consistently:
  - rook-ceph-mon, rook-ceph-osd, rook-ceph-mgr, rook-ceph-rgw, rook-ceph-mds
- [Node affinity and Tolerations](https://github.com/rook/rook/blob/master/Documentation/cluster-tpr.md#placement-configuration-settings) added to the Cluster TPR for api, mon, osd, mds, and rgw
- Each mon is managed with a replicaset
- Rook Operator [Helm chart](https://github.com/rook/rook/blob/master/demo/helm/rook-operator/README.md)
- A ConfigMap can be used to [override Ceph settings](https://github.com/rook/rook/blob/master/Documentation/advanced-configuration.md#custom-cephconf-settings) in the daemons

### Ceph
- Ceph Luminous is supported (will be the next LTS release soon)
- Kraken is no longer supported
- ceph-mgr is started as required by Luminous

### Tools
- The client binary `rook` was renamed to `rookctl`
- The damemon binary was renamed from `rookd` to `rook`
- The `rook-client` container is no longer built. Run the [toolbox container](https://github.com/rook/rook/blob/master/Documentation/toolbox.md) for access to the `rookctl` tool.
- `amd64`, `arm`, and `arm64` supported by the toolbox in addition to the daemons

### rook Container
- Based on Ubuntu instead of Alpine Linux
- Ceph tools are included
- No more embedded ceph (or any cgo) in the `rook` binary
- Rook daemons shell out to the Ceph tools for all configuration

### Build
- No more git submodules
- External repos are pulled into the build containers
- Faster incremental builds based on caching

### Test
- E2E Integration tests for Block, File, and Object
- Functional Tests for Block store
- Block store long haul testing