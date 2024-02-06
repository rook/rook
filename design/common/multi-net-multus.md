# Multi-homed Network (Multus)

## Overview

This project aims to create an API to enable multiple network interface for
Rook storage providers. Currently, Rook providers only choice is to use
`hostNetwork` or not. The API will be used to define networks resource for
Rook clusters. It enables more fine-grained control over network access.

## Legacy Implementation

To achieve non-flat networking model, Rook can choose to enable `hostNetwork`
and expose host network interfaces to Storage Provider pods.

Ceph Rook cluster network definition example:
```yaml
network:
  hostNetwork: true
```

The Ceph operator without specifying this configuration will always
default to pod networking.

## Multi-homed Network Design

Rook operators can define storage cluster's network using network provider.
Network provider example includes host, and multus. To configure the cluster
network, cluster CRD needs to tell the network provider the appropriate
`NetworkInterfaceSelector`. `NetworkInterfaceSelector` will be provided as list
of `interfaces` key-value.

```yaml
network:
  provider: <network-provider>
  interfaces:
    <key>: <network-interface-selector>
    <key>: <network-interface-selector>
```

### Network Provider

Network provider determines multi-homing method and the network interface
selector. Using host network provider, pod can use host's network namespace
and its interfaces to use as cluster network. Using popular multi-plugin CNI
plugin such as [multus][multus-cni], Rook operators can attach multiple network
interfaces to pods.

One thing to remember, leaving the network configuration empty will default to
kubernetes cluster default networking.

### Keys

The network interface selectors with key values `public` and
`cluster` are specified for Ceph.

### Network Interface Selector

Network interface selector is fed to network provider to connect pod to cluster
network. This selector may vary from one network provider to another. For
example, host network provider only needs to know the interface name.

On the other hand, multi-plugin CNI plugin needs to know the network attachment
definition's name and vice versa. Multi-plugin such as multus may seem to follow
_Network Attachment Selection Annotation_ documented by Kubernetes Network Plumbing Working Group's
[Multi-Net Spec].

Any future multi-homed network provider that implements the Network Plumbing WG's Multi-net spec may
use the `multus` network. Previously-identified providers [CNI-Genie] and [Knitter] have since gone
dormant (upd. 2023), and Multus continues to be the most prominent known provider.

## Multi-Net validation tester

Rook will implement a user-runnable routine that validates a multi-net configuration based on the
[Multi-Net Spec]. Users can input one or both of the Network Attachment Definitions (NADs) for
Ceph's `multus` network, and the routine will perform cursory validation of the ability of the
specified networks to support Ceph.

The routine will start up **a single** web server with the specified networks attached (via NADs).
The routine will also start up a number of clients on each node that will test the network(s)'s
connections by HTTP(S) requests to the web server.

A client represents a simplified view of a Ceph daemon from the networking perspective. The number
of clients is intended to represent the number of Ceph daemons that could run on a node in the worst
possible failure case where all daemons are rescheduled to a single node. This helps verify that the
network provider is able to successfully assign IP addresses to Ceph daemon pods in the worst case.
Some network interface software/hardware may limit the number of addresses that can be assigned to
an interface on a given node, and this helps verify that issue is not present.

It is important that the web server be only **a single** instance. If clients collocated on the same
node with the web server can make successful HTTP(S) requests but clients on other nodes are not,
that is an important indicator of inter-node traffic being blocked.

The validation test routine will run in the Rook operator container. Rook's kubectl [kubectl plugin]
may facilitate running the test, but it is useful to have the option of executing a long-running
validation routine running in the Kubernetes cluster instead of on an administrator's log-in
session. Additionally, the results of the validation tester may become important for users wanting
to get help with Multus configuration, and having a tool that is present in the Rook container image
will allow users to run the routine for bug reports more readily than if they needed to install the
kubectl plugin.


<!--
LINKS
-->
[multus-cni]: https://github.com/intel/multus-cni
[multi-net spec]: https://github.com/k8snetworkplumbingwg/multi-net-spec
[cni-genie]: https://github.com/cni-genie/CNI-Genie/
[knitter]: https://github.com/ZTE/Knitter
[kubectl plugin]: https://github.com/rook/kubectl-rook-ceph
