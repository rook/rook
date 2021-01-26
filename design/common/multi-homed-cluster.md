# Rook Operators Multi-homed Cluster Network Spec

## Overview

This project aims to create new API to enable multiple network interface for
Rook storage providers. Currently, Rook providers only choice is to use
`hostNetwork` or not. The new API will be used to define networks resource for
Rook clusters. Rook operators will be able to consume those definitions and
manage them. Therefore, it enables more fine-grained control over storage
providers network access.

## Current Implementation

To achieve non-flat networking model, Rook can choose to enable `hostNetwork`
and expose host network interfaces to Storage Provider pods.

Ceph Rook cluster network definition example:
```yaml
network:
  hostNetwork: true
```

The Ceph operator without specifying this configuration will always
default to pod networking.

## Proposed Design

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

In the future, works can be added to support other multi network CNI plugins
such as [knitter][knitter-cni] or [genie][genie-cni].

One thing to remember, leaving the network configuration empty will default to
kubernetes cluster default networking.

### Keys

The key for each interface key-value pairs are left to each storage providers to
decide. The network interface selectors with key values `public` and
`cluster` are specified for Ceph, while other storage providers can define their
own key names if desired.

### Network Interface Selector

Network interface selector is fed to network provider to connect pod to cluster
network. This selector may vary from one network provider to another. For
example, host network provider only needs to know the interface name.

On the other hand, multi-plugin CNI plugin needs to know the network attachment
definition's name and vice versa. Multi-plugin such as multus may seem to follow
_Network Attachment Selection Annotation_ documented at [Kubernetes Network
Custom Resource Definition De-facto Standard][network-crd]. However, their
implementation has extra features not covered by the standard such as
`@<interface-name>` notation or `interfaceRequest` property documented
[here][multus-annotation].

## Example Configurations

### Ceph Rook Cluster Network definition example

<!--TODO-->

[multus-cni]: https://github.com/intel/multus-cni
[knitter-cni]: https://github.com/ZTE/Knitter
[genie-cni]: https://github.com/cni-genie/CNI-Genie/
[network-crd]: https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit
[multus-annotation]: https://github.com/intel/multus-cni/blob/master/doc/how-to-use.md#run-pod-with-network-annotation
