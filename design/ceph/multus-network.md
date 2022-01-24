# Multus networking integration with Ceph (not finalized yet and subject to update)

We have already explored and explained the benefit of multi-homed networking, so this document will not rehearse that but simply focus on the implementation for the Ceph backend.
If you are interested in learning more about multi-homed networking you can read the [design documentation on that matter](../core/multi-homed-cluster.md).

To make the story short, [Multus](https://github.com/intel/multus-cni) should allow us to get the same performance benefit as `HostNetworking` as well as increasing the security.
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

Steps must be taken to fix a CSI-with-multus issue documented
[here](https://github.com/rook/rook/issues/8085). To summarize the issue:
when a CephFS/RBD volume is mounted in a pod using Ceph CSI and then the CSI CephFS/RBD plugin is
restarted or terminated (e.g. by restarting or deleting its DaemonSet), all operations on the volume
become blocked, even after restarting the CSI pods. The only workaround is to restart the node where
the Ceph CSI plugin pod was restarted.

When deploying a CephCluster resource configured to use multus networks, a multus-connected network
interface will be added to the host network namespace for all nodes that will run CSI plugin pods.
This will allow Ceph CSI pods to run using host networking and still access Ceph's public multus
network.

The design for mitigating the issue is comprised of two components: a "holder" DaemonSet and a
"mover" Daemonset.

#### Holder DaemonSet and Pods
The Rook-Ceph Operator's CSI controller creates a DaemonSet configured to use the
`network.selectors.public` network specified for the CephCluster. This DaemonSet runs on all the
nodes that will have CSI plugin pods. Its pods exist to "hold" a particular network interface that
the CSI pods can reliably connect to for communication with the Ceph cluster. The process running
will merely be an infinite sleep.

These Pods should only be stopped and restarted when a node is stopped so that volume operations do
not become blocked. The Rook-Ceph Operator's CSI controller should set the DaemonSet's update
strategy to `OnDelete` so that the pods do not get deleted if the DaemonSet is updated while also
ensuring that the pods will be updated on the next node reboot (or node drain).

#### Mover DaemonSet and Pods
The Rook-Ceph Operator's CSI controller also creates a second DaemonSet configured to use host
networking. This DaemonSet also runs on all nodes that will have CSI plugin pods (and holder pods).
Mover pods exist to "move" the multus network interface being held by the holder pod on the node
into the host's network namespace to provide user's volumes with uninterrupted access to the Ceph
cluster, even when the CSI driver is restarted (or updated).

The mover must:
- be a privileged container
- have `SYS_ADMIN` and `NET_ADMIN` capabilities
- be on the host network namespace
- have access to the `/var/run/netns` directory

In order to not leave moved interfaces dangling in the host's network namespace, mover pods must
move interfaces back to their original namespace when CSI is being terminated. The most
straightforward way to accomplish this is to move interfaces back when the mover is being
terminated. If an interface is moved without user applications also being removed, this will cause
I/O disruption. Therefore, the DaemonSet should also use the `OnDelete` update strategy so that the
pods can be updated on node reboots (or node drains).

In order to better handle unexpected corner cases that leave moved interfaces in the host network
namespace (e.g., a mover is killed abruptly rather than gracefully terminated), instead treat "move"
operations as a disable-and-copy operation. To do this, disable the interface in the holder pod's
network namespace, and create a copy of the interface in the host namespace with the same MAC
address and IP config. From the user standpoint, the interface is still "moved" because the original
is disabled, so we keep the "mover" terminology. This merely helps Rook ensure that it is not
accidentally losing the original information.

A previous iteration of this design specified the mover application as a sidecar to Ceph CSI plugin
pods; however, this design would mean that the mover would need to be deleted and re-created
whenever the CSI plugin is updated, possibly resulting in I/O hangs in user pods during the update.
Keeping the mover independent allows CSI plugin updates to happen freely without complex
interactions between it and the mover.

#### Interactions between components
If a copied interface is left in the host network namespace after the holder pod is removed, multus
may later give the address to a different application, and the CSI driver may try to connect to the
different application with Ceph requests. We should try to avoid leaving interfaces on the host as
much as possible. Killing the mover pod abruptly will leave copied interfaces, but there is no way
to prevent this from happening.

If a holder pod is deleted, the interface hold will be lost. The mover must remove the
interface from the host's network namespace because multus may reassign the address to a different
application. We can document for users that they should not delete holder pods, but we cannot
prevent users from manually stopping holder pods. However, to prevent the Kubernetes scheduler from
terminating holder pods, the pods should be given the highest possible priority so that they are not
un-scheduled except by users.

There is a possible race condition where a mover pod is killed and where a holder is deleted before
a new mover starts up. In this case, the copied interface for the holder pod will be left in the
host network namespace, but the mover will not get a notification that the holder was removed. In
order to clean up from this case, upon startup, the mover should delete all interface copies in the
host network namespace that do not have a holder pod associated with them.

If the mover container is stopped, it should delete all copied interfaces in the host's
network namespace under the assumption that the CSI plugin is being removed, possibly by a
CephCluster being deleted or the node going down for maintenance. It might be possible to optimize
for the case where only the mover pod is being restarted, but it is very difficult to detect the
case where the mover is merely being restarted versus when the holder is also being removed without
possible race conditions between which pod might be stopped first by the Kubernetes API server
during a drain event. Therefore, focus on the simplest working implementation (described above)
instead of risking leaving the interface copy in host net namespace which could cause issues.

If an error occurs in the mover during network migration, it will fail and re-try migration until
the operation succeeds. If necessary, the mover will try to remove a partially-copied interface.

Restarting a node will cause the multus interface in the host namespace to go away. On restart, the
holder pod will get a new interface, and the mover will copy it into the host networking namespace again.

When a new node is added, holder and mover pods are added to it by their DaemonSets, and the
move/copy process described above occurs on the node.

The holder and mover DaemonSets should be deleted when the CSI driver components are removed. Both
the termination of the holder pods as well as the mover pods triggers the mover to remove the multus
interfaces from the host network namespace of a given node.

The initial implementation of this design will be limited to supporting a single CephCluster with
Multus until we can be sure that the CSI plugin can support multiple migrated interfaces as well as
interfaces that are added and removed dynamically. This limitation will be enforced by allowing only
a single instance of the holder DaemonSet. A possible (future) partial implementation may be
possible by restarting the CSI plugin Pods when network interfaces are added or removed.

**Known issue:** the Docker container runtime does not use Linux's native `/var/run/netns`
directory. This mitigation is known to work on cri-o runtime but not Docker. Therefore, this feature
will be disabled by default and enabled optionally by the
`ROOK_CSI_MULTUS_USE_HOLDER_MOVER_PATTERN=true` variable in the Rook-Ceph operator's config.

A previous version of the CSI proposal had the holder Pods creating "setup" and "teardown"
Kubernetes Jobs for migrating/un-migrating the multus networks. This design was rejected since new
Jobs wouldn't be able to be created if the Kubernetes namespace were in "Terminating" state.

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
