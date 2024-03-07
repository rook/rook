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

## Multus
`network.provider: multus`

Rook supports using Multus NetworkAttachmentDefinitions for Ceph public and cluster networks.

This allows Rook to attach any CNI to Ceph as a public and/or cluster network. This provides strong
isolation between Kubernetes applications and Ceph cluster daemons.

While any CNI may be used, the intent is to allow use of CNIs which allow Ceph to be connected to
specific host interfaces. This improves latency and bandwidth while preserving host-level network
isolation.

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
  namespace: rook-ceph
spec:
  config: '{
      "cniVersion": "0.3.0",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "whereabouts",
        "range": "192.168.200.0/24"
      }
    }'
```

* Ensure that `master` matches the network interface on hosts that you want to use.
  It must be the same across all hosts.
* CNI type `macvlan` is highly recommended.
  It has less CPU and memory overhead compared to traditional Linux `bridge` configurations.
* IPAM type `whereabouts` is recommended because it ensures each pod gets an IP address unique
  within the Kubernetes cluster. No DHCP server is required. If a DHCP server is present on the
  network, ensure the IP range does not overlap with the DHCP server's range.

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
