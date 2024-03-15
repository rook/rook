---
title: Network Providers
---

Rook deploys CephClusters using Kubernetes' software-defined networks by default. This is simple for
users, provides necessary connectivity, and has good node-level security. However, this comes at the
expense of additional latency, and the storage network must contend with Kubernetes applications for
network bandwidth. It also means that Kubernetes applications coexist on the same network as Ceph
daemons and can reach the Ceph cluster easily via network scanning.

Rook allows selecting alternative network providers to address some of these downsides, sometimes at
the expense of others. Selecting alternative network providers is an advanced topic.

!!! Note
    This is an advanced networking topic.
    See also the [CephCluster general networking settings.](./ceph-cluster-crd.md#network-configuration-settings)

## Ceph Networking Fundamentals

Ceph daemons can operate on up to two distinct networks: public, and cluster.

Ceph daemons always use the public network. The public network is used for client communications
with the Ceph cluster (reads/writes). Rook configures this as the Kubernetes pod network by default.
Ceph-CSI uses this network for PVCs.

The cluster network is optional and is used to isolate internal Ceph replication traffic. This
includes additional copies of data replicated between OSDs during client reads/writes. This also
includes OSD data recovery (re-replication) when OSDs or nodes go offline. If the cluster network is
unspecified, the public network is used for this traffic instead.

Refer to [Ceph networking documentation](https://docs.ceph.com/docs/master/rados/configuration/network-config-ref/)
for deeper explanations of any topics.

## Specifying Ceph Network Selections
`network.addressRanges`

This configuration is always optional but is important to understand.

Some Rook network providers allow specifying the public and network interfaces that Ceph will use
for data traffic. Use `addressRanges` to specify this. For example:

```yaml
  network:
    provider: host
    addressRanges:
      public:
        - "192.168.100.0/24"
        - "192.168.101.0/24"
      cluster:
        - "192.168.200.0/24"
```

This `public` and `cluster` translate directly to Ceph's `public_network` and `host_network`
configurations.

The default network provider cannot make use of these configurations.

Ceph public and cluster network configurations are allowed to change, but this should be done with
great care. When updating underlying networks or Ceph network settings, Rook assumes that the
current network configuration used by Ceph daemons will continue to operate as intended. Network
changes are not applied to Ceph daemon pods (like OSDs and MDSes) until the pod is restarted. When
making network changes, ensure that restarted pods will not lose connectivity to existing pods, and
vice versa.

## Host Networking
`network.provider: host`

Host networking allows the Ceph cluster to use network interfaces on Kubernetes hosts for
communication. This eliminates latency from the software-defined pod network, but it provides no
host-level security isolation.

Ceph daemons will use any network available on the host for communication. To restrict Ceph to using
only a specific specific host interfaces or networks, use `addressRanges` to select the network
CIDRs Ceph will bind to on the host.

If the host networking setting is changed in a cluster where mons are already running, the existing mons will
remain running with the same network settings with which they were created. To complete the conversion
to or from host networking after you update this setting, you will need to
[failover the mons](../../Storage-Configuration/Advanced/ceph-mon-health.md#failing-over-a-monitor)
in order to have mons on the desired network configuration.

## CSI Host Networking

Host networking for CSI pods is controlled independently from CephCluster networking. CSI can be
deployed with host networking or pod networking. CSI uses host networking by default, which is the
recommended configuration. CSI can be forced to use pod networking by setting the operator config
`CSI_ENABLE_HOST_NETWORK: "false"`.

When CSI uses pod networking (`"false"` value), it is critical that `csi-rbdplugin`,
`csi-cephfsplugin`, and `csi-nfsplugin` pods are not deleted or updated without following a special
process outlined below. If one of these pods is deleted, it will cause all existing PVCs on the
pod's node to hang permanently until all application pods are restarted.

The process for updating CSI plugin pods is to follow the following steps on each Kubernetes node
sequentially:
1. `cordon` and `drain` the node
2. When the node is drained, delete the plugin pod on the node (optionally, the node can be rebooted)
3. `uncordon` the node
4. Proceed to the next node when pods on the node rerun and stabilize

For modifications, see [Modifying CSI Networking](#modifying-csi-networking).

## Multus
`network.provider: multus`

Rook supports using Multus NetworkAttachmentDefinitions for Ceph public and cluster networks.

This allows Rook to attach any CNI to Ceph as a public and/or cluster network. This provides strong
isolation between Kubernetes applications and Ceph cluster daemons.

While any CNI may be used, the intent is to allow use of CNIs which allow Ceph to be connected to
specific host interfaces. This improves latency and bandwidth while preserving host-level network
isolation.

### Multus Prerequisites

These prerequisites apply when:
- CephCluster `network.selector['public']` is specified, AND
- Operator config `CSI_ENABLE_HOST_NETWORK` is `"true"` (or unspecified), AND
- Operator config `CSI_DISABLE_HOLDER_PODS` is `"true"`

If any of the above do not apply, these prerequisites can be skipped.

In order for host network-enabled Ceph-CSI to communicate with a Multus-enabled CephCluster, some
setup is required for Kubernetes hosts.

These prerequisites require an understanding of how Multus networks are configured and how Rook uses
them. Following sections will help clarify questions that may arise here.

Two basic requirements must be met:

1. Kubernetes hosts must be able to route successfully to the Multus public network.
2. Pods on the Multus public network must be able to route successfully to Kubernetes hosts.

These two requirements can be broken down further as follows:

1. For routing Kubernetes hosts to the Multus public network, each host must ensure the following:
   1. the host must have an interface connected to the Multus public network (the "public-network-interface").
   2. the "public-network-interface" must have an IP address.
   3. a route must exist to direct traffic destined for pods on the Multus public network through
      the "public-network-interface".
2. For routing pods on the Multus public network to Kubernetes hosts, the public
   NetworkAttachementDefinition must be configured to ensure the following:
  1. The definition must have its IP Address Management (IPAM) configured to route traffic destined
     for nodes through the network.
3. To ensure routing between the two networks works properly, no IP address assigned to a node can
   overlap with any IP address assigned to a pod on the Multus public network.

These requirements require careful planning, but some methods are able to meet these requirements
more easily than others. [Examples are provided after the full document](#multus-examples) to help
understand and implement these requirements.

!!! Tip
    Keep in mind that there are often ten or more Rook/Ceph pods per host. The pod address space may
    need to be an order of magnitude larger (or more) than the host address space to allow the
    storage cluster to grow in the future.

If these prerequisites are not achievable, the remaining option is to set the Rook operator config
`CSI_ENABLE_HOST_NETWORK: "false"` as documented in the [CSI Host Networking](#csi-host-networking)
section.

### Multus Configuration

Refer to [Multus documentation](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md)
for details about how to set up and select Multus networks.

Rook will attempt to auto-discover the network CIDRs for selected public and/or cluster networks.
This process is not guaranteed to succeed. Furthermore, this process will get a new network lease
for each CephCluster reconcile. Specify `addressRanges` manually if the auto-detection process
fails or if the selected network configuration cannot automatically recycle released network leases.

Only OSD pods will have both public and cluster networks attached (if specified). The rest of the
Ceph component pods and CSI pods will only have the public network attached. The Rook operator will
not have any networks attached; it proxies Ceph commands via a sidecar container in the mgr pod.

A NetworkAttachmentDefinition must exist before it can be used by Multus for a Ceph network. A
recommended definition will look like the following:

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ceph-multus-net
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "192.168.200.0/24"
      }
    }'
```

* Note that the example above does not specify information that would route pods on the network to
  Kubernetes hosts.
* Ensure that `master` matches the network interface on hosts that you want to use.
  It must be the same across all hosts.
* CNI type [macvlan](https://www.cni.dev/plugins/current/main/macvlan/) is highly recommended.
  It has less CPU and memory overhead compared to traditional Linux `bridge` configurations.
* IPAM type [whereabouts](https://github.com/k8snetworkplumbingwg/whereabouts) is recommended
  because it ensures each pod gets an IP address unique within the Kubernetes cluster. No DHCP
  server is required. If a DHCP server is present on the network, ensure the IP range does not
  overlap with the DHCP server's range.

NetworkAttachmentDefinitions are selected for the desired Ceph network using `selectors`. Selector
values should include the namespace in which the NAD is present. `public` and `cluster` may be
selected independently. If `public` is left unspecified, Rook will configure Ceph to use the
Kubernetes pod network for Ceph client traffic.

Consider the example below which selects a hypothetical Kubernetes-wide Multus network in the
default namespace for Ceph's public network and selects a Ceph-specific network in the `rook-ceph`
namespace for Ceph's cluster network. The commented-out portion shows an example of how address
ranges could be manually specified for the networks if needed.

```yaml
  network:
    provider: multus
    selectors:
      public: default/kube-multus-net
      cluster: rook-ceph/ceph-multus-net
    # addressRanges:
    #   public:
    #     - "192.168.100.0/24"
    #     - "192.168.101.0/24"
    #   cluster:
    #     - "192.168.200.0/24"
```

### Validating Multus configuration

We **highly** recommend validating your Multus configuration before you install Rook. A tool exists
to facilitate validating the Multus configuration. After installing the Rook operator and before
installing any Custom Resources, run the tool from the operator pod.

The tool's CLI is designed to be as helpful as possible. Get help text for the multus validation
tool like so:
```console
kubectl --namespace rook-ceph exec -it deploy/rook-ceph-operator -- rook multus validation run --help
```

Then, update the args in the
[multus-validation](https://github.com/rook/rook/blob/master/deploy/examples/multus-validation.yaml)
job template. Minimally, add the NAD names(s) for public and/or cluster as needed and and then,
create the job to validate the Multus configuration.

If the tool fails, it will suggest what things may be preventing Multus networks from working
properly, and it will request the logs and outputs that will help debug issues.

Check the logs of the pod created by the job to know the status of the validation test.

### Known limitations with Multus

Daemons leveraging Kubernetes service IPs (Monitors, Managers, Rados Gateways) are not listening on
the NAD specified in the `selectors`. Instead the daemon listens on the default network, however the
NAD is attached to the container, allowing the daemon to communicate with the rest of the cluster.
There is work in progress to fix this issue in the
[multus-service](https://github.com/k8snetworkplumbingwg/multus-service) repository. At the time of
writing it's unclear when this will be supported.

### Multus examples

#### Macvlan, Whereabouts, Node Dynamic IPs

The network plan for this cluster will be as follows:
- The underlying network supporting the public network will be attached to hosts at `eth0`
- Macvlan will be used to attach pods to `eth0`
- Pods and nodes will have separate IP ranges
- Nodes will get the IP range 192.168.252.0/22 (this allows up to 1024 hosts)
- Nodes will have IPs assigned dynamically via DHCP
  (DHCP configuration is outside the scope of this document)
- Pods will get the IP range 192.168.0.0/18 (this allows up to 16,384 Rook/Ceph pods)
- Whereabouts will be used to assign IPs to the Multus public network

Node configuration must allow nodes to route to pods on the Multus public network.

Because pods will be connecting via Macvlan, and because Macvlan does not allow hosts and pods to
route between each other, the host must also be connected via Macvlan.

Because the host IP range is different from the pod IP range, a route must be added to include the
pod range.

Such a configuration should be equivalent to the following:

```console
ip link add public-shim link eth0 type macvlan mode bridge
ip link set public-shim up
dhclient public-shim # gets IP in range 192.168.252.0/22
ip route add 192.168.0.0/18 dev public-shim
```

The NetworkAttachmentDefinition for the public network would look like the following, using
Whereabouts' `exclude` option to simplify the `range` request.

The Whereabouts `routes[].dst` option
([is not well documented](https://gist.github.com/dougbtv/b41c759e9b9aee6a3fe210f09da8e835))
but allows adding routing pods to hosts via the Multus public network.

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: public-net
  # namespace: rook-ceph # (optional) operator namespace
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "1192.168.0.0/18",
        "routes": [
          {"dst": "192.168.252.0/22"}
        ]
      }
    }'
```

#### Macvlan, Whereabouts, Node Static IPs

The network plan for this cluster will be as follows:
- The underlying network supporting the public network will be attached to hosts at `eth0`
- Macvlan will be used to attach pods to `eth0`
- Pods and nodes will share the IP range 192.168.0.0/16
- Nodes will get the IP range 192.168.252.0/22 (this allows up to 1024 hosts)
- Pods will get the remainder of the ranges (192.168.0.1 to 192.168.251.255)
- Whereabouts will be used to assign IPs to the Multus public network
- Nodes will have IPs assigned statically via PXE configs
  (PXE configuration and static IP management is outside the scope of this document)

PXE configuration for the nodes must apply a configuration to nodes to allow nodes to route to pods
on the Multus public network.

Because pods will be connecting via Macvlan, and because Macvlan does not allow hosts and pods to
route between each other, the host must also be connected via Macvlan.

Because the host IP range is a subset of the whole range, a route must be added to include the whole
range.

Such a configuration should be equivalent to the following:

```console
ip link add public-shim link eth0 type macvlan mode bridge
ip addr add 192.168.252.${STATIC_IP}/22 dev public-shim
ip link set public-shim up
ip route add 192.168.0.0/16 dev public-shim
```

The NetworkAttachmentDefinition for the public network would look like the following, using
Whereabouts' `exclude` option to simplify the `range` request. The Whereabouts `routes[].dst` option
ensures pods route to hosts via the Multus public network.

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: public-net
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "192.168.0.0/16",
        "exclude": [
           "192.168.252.0/22"
        ],
        "routes": [
          {"dst": "192.168.252.0/22"}
        ]
      }
    }'
```

#### Macvlan, DHCP

The network plan for this cluster will be as follows:
- The underlying network supporting the public network will be attached to hosts at `eth0`
- Macvlan will be used to attach pods to `eth0`
- Pods and nodes will share the IP range 192.168.0.0/16
- DHCP will be used to ensure nodes and pods get unique IP addresses

Node configuration must allow nodes to route to pods on the Multus public network.

Because pods will be connecting via Macvlan, and because Macvlan does not allow hosts and pods to
route between each other, the host must also be connected via Macvlan.

Such a configuration should be equivalent to the following:

```console
ip link add public-shim link eth0 type macvlan mode bridge
ip link set public-shim up
dhclient public-shim # gets IP in range 192.168.0.0/16
```

The NetworkAttachmentDefinition for the public network would look like the following.

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: public-net
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "dhcp",
      }
    }'
```

## Modifying CSI networking

### Disabling Holder Pods with Multus and CSI Host Networking

This migration section applies in the following scenarios:
- CephCluster `network.provider` is `"multus"`, AND
- Operator config `CSI_DISABLE_HOLDER_PODS` is changed to `"true"`, AND
- Operator config `CSI_ENABLE_HOST_NETWORK` is (or is modified to be) `"true"`

If the scenario does not apply, skip ahead to the
[Disabling Holder Pods](#disabling-holder-pods) section below.

**Step 1**
Before setting `CSI_ENABLE_HOST_NETWORK: "true"` and `CSI_DISABLE_HOLDER_PODS: "true"`, thoroughly
read through the [Multus Prerequisites section](#multus-prerequisites). Use the prerequisites
section to develop a plan for modifying host configurations as well as the public
NetworkAttachmentDefinition.

Once the plan is developed, execute the plan by following the steps below.

**Step 2**
First, modify the public NetworkAttachmentDefinition as needed. For example, it may be necessary to
add the `routes` directive to the Whereabouts IPAM configuration as in
[this example](#macvlan-whereabouts-node-static-ips).

**Step 3**
Next, modify the host configurations in the host configuration system. The host configuration system
may be something like PXE, ignition config, cloud-init, Ansible, or any other such system. A node
reboot is likely necessary to apply configuration updates, but wait until the next step to reboot
nodes.

**Step 4**
After the NetworkAttachmentDefinition is modified, OSD pods must be restarted. It is easiest to
complete this requirement at the same time nodes are being rebooted to apply configuration updates.

For each node in the Kubernetes cluster:
1. `cordon` and `drain` the node
2. Wait for all pods to drain
3. Reboot the node, ensuring the new host configuration will be applied
4. `uncordon` and `undrain` the node
5. Wait for the node to be rehydrated and stable
6. Proceed to the next node

By following this process, host configurations will be updated, and OSDs are also automatically
restarted as part of the `drain` and `undrain` process on each node.

OSDs can be restarted manually if node configuration updates do not require reboot.

**Step 5**
Once all nodes are running the new configuration and all OSDs have been restarted, check that the
new node and NetworkAttachmentDefinition configurations are compatible. To do so, verify that each
node can `ping` OSD pods via the public network.

Use the [toolbox](../../Troubleshooting/ceph-toolbox.md) or the
[kubectl plugin](../../Troubleshooting/kubectl-plugin.md) to list OSD IPs.

The example below uses
the kubectl plugin, and the OSD public network is 192.168.20.0/24.
```console
$ kubectl rook-ceph ceph osd dump | grep 'osd\.'
osd.0 up   in  weight 1 up_from 7 up_thru 0 down_at 0 last_clean_interval [0,0) [v2:192.168.20.19:6800/213587265,v1:192.168.20.19:6801/213587265] [v2:192.168.30.1:6800/213587265,v1:192.168.30.1:6801/213587265] exists,up 7ebbc19a-d45a-4b12-8fef-0f9423a59e78
osd.1 up   in  weight 1 up_from 24 up_thru 24 down_at 20 last_clean_interval [8,23) [v2:192.168.20.20:6800/3144257456,v1:192.168.20.20:6801/3144257456] [v2:192.168.30.2:6804/3145257456,v1:192.168.30.2:6805/3145257456] exists,up 146b27da-d605-4138-9748-65603ed0dfa5
osd.2 up   in  weight 1 up_from 21 up_thru 0 down_at 20 last_clean_interval [18,20) [v2:192.168.20.21:6800/1809748134,v1:192.168.20.21:6801/1809748134] [v2:192.168.30.3:6804/1810748134,v1:192.168.30.3:6805/1810748134] exists,up ff3d6592-634e-46fd-a0e4-4fe9fafc0386
```

Now check that each node (NODE) can reach OSDs over the public network:
```console
$ ssh user@NODE
$ user@NODE $> ping -c3 192.168.20.19
# [truncated, successful output]
$ user@NODE $> ping -c3 192.168.20.20
# [truncated, successful output]
$ user@NODE $> ping -c3 192.168.20.21
# [truncated, successful output]
```

If any node does not get a successful `ping` to a running OSD, it is not safe to proceed. A problem
may arise here for many reasons. Some reasons include: the host may not be properly attached to the
Multus public network, the public NetworkAttachmentDefinition may not be properly configured to
route back to the host, the host may have a firewall rule blocking the connection in either
direction, or the network switch may have a firewall rule blocking the connection. Diagnose and fix
the issue, then return to **Step 1**.

**Step 6**
If the above check succeeds for all nodes, proceed with the
[Disabling Holder Pods](#disabling-holder-pods) steps below.

### Disabling Holder Pods

This migration section applies when `CSI_DISABLE_HOLDER_PODS` is changed to `"true"`.

**Step 1**
If any CephClusters have Multus enabled (`network.provider: "multus"`), follow the
[Disabling Holder Pods with Multus and CSI Host Networking](#disabling-holder-pods-with-multus-and-csi-host-networking)
steps above before continuing.

**Step 2**
Begin by setting `CSI_DISABLE_HOLDER_PODS: "true"` (and `CSI_ENABLE_HOST_NETWORK: "true"` if desired).

After this, `csi-*plugin-*` pods will restart, and `csi-*plugin-holder-*` pods will remain running.

**Step 3**
Check that CSI pods are using the correct host networking configuration using the example below as
guidance (in the example, `CSI_ENABLE_HOST_NETWORK` is `"true"`):
```console
$ kubectl -n rook-ceph get -o yaml daemonsets.apps csi-rbdplugin | grep -i hostnetwork
      hostNetwork: true
$ kubectl -n rook-ceph get -o yaml daemonsets.apps csi-cephfsplugin | grep -i hostnetwork
      hostNetwork: true
$ kubectl -n rook-ceph get -o yaml daemonsets.apps csi-nfsplugin | grep -i hostnetwork
      hostNetwork: true
```

**Step 4**
At this stage, PVCs for running applications are still using the holder pods. These PVCs must be
migrated from the holder to the new network. Follow the below process to do so.

For each node in the Kubernetes cluster:
1. `cordon` and `drain` the node
2. Wait for all pods to drain
3. Delete all `csi-*plugin-holder*` pods on the node (a new holder will take it's place)
4. `uncordon` and `undrain` the node
5. Wait for the node to be rehydrated and stable
6. Proceed to the next node

**Step 5**
After this process is done for all Kubernetes nodes, it is safe to delete the `csi-*plugin-holder*`
daemonsets.

Delete the holder daemonsets using the example below as guidance:
```console
$ kubectl -n rook-ceph get daemonset -o name | grep plugin-holder
daemonset.apps/csi-cephfsplugin-holder-my-cluster
daemonset.apps/csi-rbdplugin-holder-my-cluster

$ kubectl -n rook-ceph delete daemonset.apps/csi-cephfsplugin-holder-my-cluster
daemonset.apps "csi-cephfsplugin-holder-my-cluster" deleted

$ kubectl -n rook-ceph delete daemonset.apps/csi-rbdplugin-holder-my-cluster
daemonset.apps "csi-rbdplugin-holder-my-cluster" deleted
```

**Step 6**
The migration is now complete! Congratulations!

### Applying CSI Networking

This migration section applies in the following scenario:
- `CSI_ENABLE_HOST_NETWORK` is modified, AND
- `CSI_DISABLE_HOLDER_PODS` is `"true"`

**Step 1**
If `CSI_DISABLE_HOLDER_PODS` is unspecified or is `"false"`, follow the
[Disabling Holder Pods](#disabling-holder-pods) section first.

**Step 2**
Begin by setting the desired `CSI_ENABLE_HOST_NETWORK` value.

**Step 3**
At this stage, PVCs for running applications are still using the the old network. These PVCs must be
migrated to the new network. Follow the below process to do so.

For each node in the Kubernetes cluster:
1. `cordon` and `drain` the node
2. Wait for all pods to drain
3. `uncordon` and `undrain` the node
4. Wait for the node to be rehydrated and stable
5. Proceed to the next node

**Step 4**
The migration is now complete! Congratulations!
