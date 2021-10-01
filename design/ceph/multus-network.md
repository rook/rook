# Multus networking integration with Ceph (not finalized yet and subject to update)

We have already explored and explained the benefit of multi-homed networking, so this document will not rehearse that but simply focus on the implementation for the Ceph backend.
If you are interested in learning more about multi-homed networking you can read the [design documentation on that matter](../core/multi-homed-cluster.md).

To make the story short, [Multus](https://github.com/intel/multus-cni) should allow us to get the same performance benefit as `HostNetworking`, but increasing the security.
Using `HostNetworking` results in exposing **all** the network interfaces (the entire stack) of the host inside the container where Multus allows you to pick the one you want.
Also, this minimizes the need of privileged containers (required for `HostNetworking`).

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

The CSI pods will run in the node's host network namespace.

When deploying a CephCluster resource configured to use multus networks, a multus-connected network interface will be added to the host network namespace of all nodes that will run CSI plugin pods.

Before deploying the CSI pods, the rook operator will first create a daemonset whose pods will run on the same nodes as the CSI pods, and is configured to use the multus network that will serve as the public network in the Ceph cluster. The pods from this daemonset will be referred to as the holder pods. These pods sleep indefinitely and are present only to hold the IP address from the multus IPAM. If these pods ever went down, the IPAM would consider the IP address free and reuse them, which could lead to IP address collisions. To avoid this, the daemonset is configured to  be part of the system-node-critical priority class, and has an onDelete upgrade policy.

Once the pods from the holder daemonset are up and running, the Rook operator deploys jobs in parallel on each node to migrate the multus interface from the holder pod's network namespace into the host network namespace. These jobs run privileged pods, with SYS_ADMIN and NET_ADMIN capabilities, on the host network namespace, and with access to the /var/run/netns directory.

The job is provided with the holder pod's pod IP, the multus IP to migrate, and the multus link to migrate. The multus IP is first used to check if the migration has already occurred. The job will return if it finds that the multus IP is already in use by an interface in the host network namespace. Prior to interface migration, the pod determines an available name for the new interface in the host network namespace. The migrated interfaces take on the name of "mlink#" in the host network namespace, starting with mlink0. The job uses the pod IP to find the holder pod's network namespace. The job then goes into the holder network namespace to rename the network interface and move it to the host network namespace. Moving namespaces causes the interface to lose its network configuration. The job therefore reconfigures the moved interface with its original IP configuration, and sets the link up. Before exiting, the job checks to ensure that the new interface is present on the host network namespace.

If an error occurs in any of the migration jobs, the operator runs a teardown job on all of the nodes where migration was to have occurred. The teardown job runs on the host network namespace and is passed the multus IP of that node. It searches for an interface in the host namespace with the multus IP and removes it. If there is no such interface present, the job is considered complete.

Restarting the node will cause the multus interface in the host namespace to go away. The holder pod will once again have the interface, and will be remigrated once the job runs again on the node.

The deployment of the migration and teardown steps requires a service account with the proper permissions to deploy it. The rook-ceph-csi SecurityContextConstraints already has the needed permissions, so the rook-ceph-multus service account was added to list of accounts allowed to use these constraints.


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

