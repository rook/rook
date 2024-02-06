# Recover RBD/CephFS RWO PVC in case of Node Loss

## Goal

Faster RBD/CephFS RWO recovery in case of node loss.

## Problem

For RBD RWO recovery:

When a node is lost where a pod is running with the RBD RWO volume is mounted, the volume cannot automatically be mounted on another node. If two clients are write to the same volume it could cause corruption. The node must be guaranteed to be down before the volume can be mounted on another node.

For CephFS recovery:

With the current design the node recovery will be faster for CephFS.

## Current Solution

For RBD RWO recovery:

We have a manual solution to the problem which involves forceful deletion of a pod so that forced detachment and attachment work is possible. The problem with the current solution is that even after the forced pod deletion it takes around 11 minutes for the volume to mount on the new node. Also there are still chances of data corruption if the old pod on the lost node comes back online, causing multiple writers and lead to data corruption if the [documentation](https://rook.github.io/docs/rook/v1.11/Troubleshooting/ceph-csi-common-issues/#node-loss) is not followed to manually block nodes.

For CephFS recovery:

Currently, CephFS recovery is slower in case of node loss.

## Proposed Solution

> Note: This solution requires minimum kubernetes version 1.26.0

The kubernetes feature [Non-Graceful Node Shutdown](https://kubernetes.io/blog/2022/12/16/kubernetes-1-26-non-graceful-node-shutdown-beta/) is available starting in Kubernetes 1.26 to help improve the volume recovery during node loss. When a node is lost, the admin is required to add the taint `out-of-service` manually to the node. After the node is tainted, Kubernetes will:

- Remove the volume attachment from the lost node
- Delete the old pod on the lost node
- Create a new pod on the new node
- Allow the volume to be attached to the new node

Once this taint is applied manually, Rook will create a [NetworkFence CR](https://github.com/csi-addons/kubernetes-csi-addons/blob/main/docs/networkfence.md). The [csi-addons operator](https://github.com/csi-addons/kubernetes-csi-addons) will then blocklist the node to prevent any ceph rbd/CephFS client on the lost node from writing any more data.

After the new pod is running on the new node and the old node which was lost comes back, Rook will delete the [NetworkFence CR](https://github.com/csi-addons/kubernetes-csi-addons/blob/main/docs/networkfence.md).

example of taint to be applied to lost node:

```console
kubectl taint nodes <node-name> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
# or
kubectl taint nodes <node-name> node.kubernetes.io/out-of-service=nodeshutdown:NoSchedule
```

> Note: This will be enabled by default in Rook if the NetworkFence CR is found, in the case for some reason user wants to disable this feature in Rook can edit the `rook-ceph-operator-config` configmap and update the `ROOK_WATCH_FOR_NODE_FAILURE: "false"`.

## How to get which IP to blocklist

There are multiple networking options available for example, Host Networking, Pod networking, Multus etc. This make it difficult to know which NodeIP address to blocklist.
For this we'll follow the following approach which will work for all networking options, except when connected to an external Ceph cluster.

1. Get the `volumesInUse` from the node which has the taint `out-of-service`.

2. List all the pv and compare the pv `spec.volumeHandle` with the node `volumesInUse` field `volumeHandle`
   1. Example:

      Below is sample Node volumeInUse field

    ```
      volumesInUse:
        - kubernetes.io/csi/rook-ceph.rbd.csi.ceph.com^0001-0009-Rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002

      # Note: The volumeInUse naming convention are `kubernetes.io/csi/ + CSI driver name + ^ + volumeHandle`
    ```

     and the following is pv `volumeInHandle`

     ```
      volumeHandle: 0001-0009-rook-ceph-0000000000000002-24862838-240d-4215-9183-abfc0e9e4002
    ```

3. For Ceph volumes on that node:

   1. If RBD PVC makes use of the rbd status API
      1. example:

    ```console
    $ rbd status <poolname>/<image_name>
    Watchers:
    watcher=172.21.12.201:0/4225036114 client.17881 cookie=18446462598732840961
    ```

   2. If CephFS PVC uses below CLI to clients connect to subvolume
      1. example:

      ```console
      $ ceph tell mds.* client ls
      ...
      ...
      ...
       "addr": {
                "type": "v1",
                "addr": "192.168.39.214:0",
                "nonce": 1301050887
       }
      ...
      ...
      ...
       ```

4. Get IPs from step 3 (in above example `172.21.12.201`)
5. blocklist the IP where the volumes are mounted.

Suggested change
Example of a NetworkFence CR that the Rook operator would create when a `node.kubernetes.io/out-of-service` taint is added on the node:

```yaml
apiVersion: csiaddons.openshift.io/v1alpha1
kind: NetworkFence
metadata:
  name: <name> # We will keep the name the same as the node name
  namespace: <ceph-cluster-namespace>
spec:
  driver: <driver-name> #  extract the driver name from the PV object
  fenceState: <fence-state> # For us it will be `Fenced`
  cidrs:
    - 172.21.12.201
  secret:
    name: <csi-rbd-provisioner-secret-name/csi-cephfs-provisioner-secret-name> # from pv object
    namespace: <ceph-cluster-namespace>
  parameters:
    clusterID: <clusterID> # from pv.spec.csi.volumeAttributes
```

## Bringing a node back online

Once the node is back online, the admin removes the taint.

1. Remove the taint

```console
kubectl taint nodes <node-name> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute-
# or
kubectl taint nodes <node-name> node.kubernetes.io/out-of-service=nodeshutdown:NoSchedule-
```

1. Rook will detect the taint is removed from the node, and immediately unfence the node by deleting the corresponding networkFence CR.

## Automation to taint a node

Rook will not automate tainting the node when they go offline. This is a decision the admin needs to make. But Rook will consider creating a sample script to watch for unavailable nodes and automatically taint the node based on how long node is offline. The admin can choose to enable the automated taints by running this example script.

## Open Design Questions

1. How to handle MultiCluster scenarios where two different node on different clusters have the same overlapping IP's.
