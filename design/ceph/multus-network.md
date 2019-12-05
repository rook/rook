# Multus networking integration with Ceph (not finalized yet and subject to update)

We have already explored and explained the benefit of multi-homed networking, so this document will not rehearse that but simply focus on the implementation for the Ceph backend.
If you are interested in learning more about multi-homed networking you can read the [design documentation on that matter](../core/multi-homed-cluster.md).

To make the story short, [Multus](https://github.com/intel/multus-cni) should allow us to get the same performance benefit as `HostNetworking` by increasing the security.
Using `HostNetworking` results in exposing **all** the network interfaces (the entire stack) of the host inside the container where Multus allows you to pick the one you want.
Also, this removes the need of privileged containers (required for `HostNetworking`).

## Proposed CRD changed

We already have a `network` CRD property, which looks like:

```yaml
network:
  provider:
  selectors:
```

We will expand the `selectors` with the following two hardcoded keys:

```yaml
selectors:
  public:
  cluster:
```

Each selector represents a [`NetworkAttachmentDefinition`](https://github.com/intel/multus-cni/blob/master/doc/quickstart.md#storing-a-configuration-as-a-custom-resource) object in Multus.
At least, one must be provided and by default they will represent:

- `public`: data daemon public network (binds to `public_network` Ceph option).
- `cluster`: data daemon cluster network (binds to `cluster_network` Ceph option).

If only `public` is set then `cluster` will take the value of `public`.

## Multus supported configuration

### Interface type

As part of the CNI spec, Multus supports several [interface types](https://github.com/containernetworking/plugins#main-interface-creating).
Rook will naturally support any of them as they don't fundamentally change the working behavior.

### IPAM type

This is where things get more complex.
Currently there are [three different IPAM](https://github.com/containernetworking/plugins#ipam-ip-address-allocation) solutions available.

As part of our research we have found that the following IPAM types are not good candidates:

- host-local: Maintains a local database of allocated IPs, this only works on a per host basis, so not suitable for a distributed environment since we will end up with IP collision
To fix this, the [whereabouts project](https://github.com/dougbtv/whereabouts) looks promising but is not officially supported.
- static: Allocate a static IPv4/IPv6 addresses to container and it's useful in debugging purpose.
This cannot at scale because this means we will have to allocate IPs for **all** the daemon so it's not scalable.

You can find a more detailed analysis at the end of the document in the [rejected proposal section](#rejected-proposals).

## Ceph daemons implementation challenges

### Monitors and OSDs

The Ceph monitors only need access to the public network.
The OSDs needs access to both public and cluster networks.

Monitors requirements so far are the following:

- Predictable IP addresses
- Keep the same IP address for the life time of the deployment (IP should survive a restart)

### RGW implementation

Only need access to the public network.
They use service IP with a load-balancer so we need to be careful.

### MDS/RBD-MIRROR/NFS implementation

Only need access to the public network.
Nothing to do in particular since they don't use any service IPs.

### CSI pods

We can add annotations to these pods and they can reach out the Ceph public network, then the driver will expose the block or the filesystem normally.

## Accepted proposal

So far, the team has decided to go with the [whereabouts](https://github.com/dougbtv/whereabouts) IPAM.
It's an IP Address Management (IPAM) CNI plugin that assigns IP addresses cluster-wide.
If you need a way to assign IP addresses dynamically across your cluster -- Whereabouts is the tool for you. If you've found that you like how the host-local CNI plugin works, but, you need something that works across all the nodes in your cluster (host-local only knows how to assign IPs to pods on the same node) -- Whereabouts is just what you're looking for.
Whereabouts can be used for both IPv4 & IPv6 addressing.

It is under active development and is not ready but ultimately will allow to:

- Have static IP addresses distributed across the cluster
- These IPs will survive any deployment restart
- The allocation will be done by whereabouts and not by Rook

/!\

The only thing that is not solved yet is how can we predict the IPs for the upcoming monitors?
We might need to rework the way we bootstrap the monitors a little bit to not require to know the IP in advance.

## Rejected proposals

The following proposal were rejected but we keep them here for traceability and knowledge.

### IPAM type 'DHCP'

In this scenario, a DHCP server will distribute an IP address to a pod using a given range.

Pros:

- Pods will get a dedicated IP on a physical network interface
- No changes required in Ceph, Rook will detect the CIDR via the `NetworkAttachmentDefinition` then populate Ceph's flag `public_network` and `cluster_network`

Cons:

- IP allocation not predictable, we don't know it until the pod is up and running.
So the detection must append inside the monitor container is running, similarly to what the OSD code does today.
- Requires drastic changes in the monitor bootstrap code
- This adds a DHCP daemon on every part of the cluster and this has proven to be troublesome

Assuming we go with this solution, we might need to change the way the monitors are bootstrapped:

1. let the monitor discovered its own IP based on an interface
2. once the first mon is bootstrapped, we register its IP in a ConfigMap as well as populating clusterInfo
3. boot the second mon, look up in clusterInfo for the initial member (if the op dies in the process, we always `CreateOrLoadClusterInfo()` as each startup based on the cm so no worries)
4. go on and on with the rest of the monitors

TBT: if the pod restarts, it keeps the same IP.

### IPAM type 'DHCP' with service IP

We could and this is pure theory at this point use a IPAM with DHCP along with service IP.
This would require interacting with Kubeproxy and there is no such feature yet.
Even if it was there, we decided not to go with DHCP so this is not relevant.
