# v1.12 Pending Release Notes

## Breaking Changes

## Features

- Automate the faster recovery of the RBD RWO volume from node loss utilizing kubernetes feature [Non-Graceful Node Shutdown](https://kubernetes.io/blog/2022/12/16/kubernetes-1-26-non-graceful-node-shutdown-beta/) by requiring manual tainting of the node with an 'out-of-service' taint once admin confirm that the node is down. This feature also prerequisites on the [CSI-add-ons](https://rook.github.io/docs/rook/latest/Storage-Configuration/Ceph-CSI/ceph-csi-drivers/?h=csiaddons#csi-addons-controller) for a network fencing CRD, enable `csi-addons` sidecar and the minimum required Kubernetes version is v1.26.
- integrating with ceph cosi driver support
