# **Rook Local Node Agent**

## **Overview**
In a distributed storage system, there are operations that must be performed on a specific node in the cluster where storage is to be consumed from.
For example, a database pod that requires persistent storage needs to attach and mount a volume backed by the storage cluster on the same node that the pod is scheduled to run on.
In this document, we propose a design for a Rook "agent" that will be deployed in the cluster to run on nodes that have a need to perform operations to consume storage from the cluster.

## **Background: Flexvolume Issues**
In Kubernetes 1.8, the Kubernetes Storage SIG recommends storage providers to implement out-of-tree plugins to provide persistent storage.
With the full implementation of Container Storage Interface (CSI) several months away from completion, the recommended approach in the meantime is to implement a Flexvolume.
Deployment of a Flexvolume will be improved in 1.8 to make it less of a manual process (e.g., dynamic discovery), as described in the following pull request: https://github.com/kubernetes/community/pull/833

However, there are still some limitations to the capabilities of a Flexvolume implementation.
For example, a Flexvolume plugin does not execute in a context that has cluster credentials, so it cannot communicate with the Kubernetes API to perform such operations as creating Custom Resource Definitions (CRDs).
This document will describe how a Rook local node agent would work with the Flexvolume architecture to provide storage in a Kubernetes cluster, in addition to other responsibilities that must be performed on specific nodes in the cluster.

## **Detailed Design**

### **Responsibilities**
The Rook agent will have multiple responsibilities beyond just performing storage operations for the Rook Flexvolume driver.
One can think of the agent as a mini operator that functions at the node level.
The initial proposed responsibilities of the agent are:

1. Deploy the Rook Flexvolume driver to the `volume-plugin-dir` directory on every node
1. Perform storage operations on behalf of the Flexvolume driver, such as attaching, detaching, mounting and unmounting Rook cluster storage
1. Cluster clean up operations, such as forcefully unmapping RBD devices when the Rook cluster is being deleted while there are still pods consuming those volumes
1. Proxy I/O traffic from kernel modules to user space (e.g. Network Block Device (NBD) kernel module to librbd in userspace)

### **Deployment**
The Rook operator will deploy the Rook agent to all nodes in the cluster via a Daemonset in the same namespace in which the operator is running.
It is a permanent (long running) daemon that has a lifecycle tied to the node that it is scheduled on.
The agent deployment will happen when the operator is first created, in the same flow where the operator is declaring CRDs for clusters, pools, etc.
This means that the Rook agents are not associated with a specific Rook cluster and that they will be able to handle operations for any Rook cluster instance.
The Rook operator CRD will be updated to allow selectors to control the set of nodes that the agent is scheduled on, but in most cases it is desirable for it to be running on all nodes.

Each agent pod will be running the same `rook/rook` container image in use today, but with a new `agent` command (similar to the existing `mon` or `osd` commands).
This image will have all the tools necessary to provision and manage storage (e.g., `rbd`, iSCSI tools and the Flexvolume driver).
When the agent starts up, it will immediately copy the Rook Flexvolume driver from its image to the `volume-plugin-dir` on its host node.
In Kubernetes 1.8+, the [dynamic Flexvolume plugin discovery](https://github.com/kubernetes/community/pull/833) will find and initialize our driver, but in older versions of Kubernetes a manual restart of the Kubelet will be required.

After the driver has been deployed, the Rook agent will create a Unix domain socket that will serve as the communications channel between the agent and the Flexvolume driver.
The driver will initiate a handshake to the agent over the socket during its [`Init` function](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#init) when it is executed after being discovered by the Controller manager.
The agent will wait for the driver to complete the handshake before moving on and being ready to accept Flexvolume storage requests.

Note that the Rook operator will continue to run its existing [dynamic volume provisioner](https://github.com/rook/rook/tree/master/pkg/operator/provisioner) to provision and delete persistent volumes as needed.

### **Flexvolume**
The Rook Flexvolume driver will be very lightweight, simply implementing `Mount()` and `Unmount()` from the [required interface](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#driver-invocation-model)
and then offloading the storage provider work to the Rook agent over the Unix domain socket.
Note this means that the attach/detach controller and its centralized attaching/detaching will not be used.
Instead, attaching will be performed as part of `Mount()` and detaching as part of `Unmount()`, as described in the next section.
This is important because it means that the Rook Flexvolume driver and Agent do **not** need to be installed on the Kubernetes master node (control plane), which is not possible in some environments such as Google Container Engine (GKE).
This makes this proposed design more portable.

#### Control Flow Overview
Below is a simplified description of the control flow for providing block storage ready to be consumed by a pod:

1. Rook operator is running and a Rook agent pod is running on every node in the cluster.  The Rook Flexvolume driver has been deployed to the `volume-plugin-dir` on each node.
1. A user creates a Persistent Volume Claim (PVC) specifying a storageclass that uses the `rook.io/block` [provisioner](https://github.com/kubernetes-incubator/external-storage/blob/e6e64ad1a431fea37f723882f36251f8d2fe4247/lib/controller/volume.go#L29)
1. `Provision()` is called on the operator's provisioner to create a block image in the cluster.  At this point, the Provision phase is complete and the PVC/PV are considered `Bound` together.
1. Once a pod has been created that consumes the PVC, `Mount()` is called by the Kubelet on the Rook Flexvolume on the node that will be consuming the storage, which calls into its local Rook agent via the Unix domain socket.
    * `Mount()` is a blocking call by the Kubelet and it will wait while the entire mapping/mounting is performed by the driver and agent synchronously.
1. The agent then creates a volume attach CRD that represents the attachment of the cluster volume to the node.
1. Next, the agent performs the mapping of the volume to a local device on the node and updates the status of the CRD and its device path field (e.g., `/dev/rbd0`).
1. Control is returned to the driver, and if the mapping was successful then the driver will proceed with mounting the new local device to the requested path on the host.
If necessary, the driver will also format the volume according to the filesystem type expressed on the storageclass for the volume.
1. The driver then returns from `Mount()` with a successful result to the Kubelet.

##### Unmount
During an unmount operation, the [`Unmount()`](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md#unmount) call is only given the mount dir to unmount from.
With this limited information, it is difficult to ascertain more information about the specific volume attachment that is intended.
Currently, the mount dir has some of this information encoded in the full path.
Take for example the following mount dir:
```
/var/lib/kubelet/pods/4b859788-9290-11e7-a54f-001c42fe7d2c/volumes/kubernetes.io~rbd/pvc-4b7eab9a-9290-11e7-a54f-001c42fe7d2c
```

Given the mount dir above, one can infer that it is for pod `4b859788-9290-11e7-a54f-001c42fe7d2c` and PV `pvc-4b7eab9a-9290-11e7-a54f-001c42fe7d2c`.
The agent will use this information to look up the correct CRD instance, granting itself full context for how to perform the detach and unmount.

Parsing this particular mount dir format doesn't have any guarantee of stability in future Kubernetes releases, so we cannot rely on this long term.
The ideal solution would be for the Kubelet to pass along full context information to the `Unmount()` call.
This improvement is being tracked in https://github.com/kubernetes/kubernetes/issues/52590.

#### Usage of CRDs
The control flow described above explained how the agent uses the Kubernetes API to create a volume attach CRD that represents the attachment of a cluster volume to a specific cluster node.
This usage of the Kubernetes API is a reason why the Flexvolume driver, which does not run in a context that has any cluster credentials, is insufficient for this design, and thus why a Rook agent pod is desirable.
Let's enumerate some of the benefits of using a volume attach CRD:

1. The admin gets a native `kubectl` experience for viewing and getting information on attached volumes in their cluster.
1. The CRD helps provide fencing for the volume in a generalized way, it is not specific to the underlying storage Rook is creating and managing.
The existence of the CRD provides a means of bookkeeping to signal that the volume is locked and in use.
1. In the event that a node dies that had a volume attached, a CRD allows centralized detachment of the volume by the Rook operator.  This will be explained in more detail in the [fencing section](#fencing).

#### Improvements Over Rook's Current Support for Persistent Volumes
Rook currently has a hybrid approach of providing persistent storage in a Kubernetes cluster.  While the Rook operator implements a dynamic volume provisioner, the attach and mounting is offloaded to the existing [RBD plugin](https://github.com/kubernetes/kubernetes/tree/master/pkg/volume/rbd).
This requires that the Ceph tools are installed alongside the Kubelet on every node in the cluster, which causes friction in the experience for users of Rook.
Furthermore, if users want to consume a Rook volume outside of the default namespace, then they must manually copy secrets to their namespace of choice.

Both of these issues are handled by this proposed design for the Rook agent, greatly streamlining the Rook storage experience in Kubernetes and removing the common causes for errors and failure.  This is a big win for our users.

This proposed design also allows Rook to normalize on a single path for providing persistent volumes across all Kubernetes versions that it supports.
Since this design is fully out-of-tree for Kubernetes, it is not tied to Kubernetes release timelines.
Updates, fixes and new features can be released through the normal Rook release process and timelines.

The experience is also normalized across other distributed storage platforms that Rook will support in the future, such as GlusterFS.
Instead of the user having to create a storageclass specific for the underlying distributed storage (e.g., RBD if Rook is using Ceph), this will abstract that decision away, allowing the user to simply request storage.
This was already true of the hybrid approach in Rook v0.5, but it's worth noting here still.

#### Volume Attach CRD Details
This section will discuss the volume attach CRD in more detail.
The name (primary identity) of each CRD instance will be the Persistent Volume (PV) name that it is for.
This will enable very efficient look ups of volume attach instances for a particular PV, thereby making fencing checks very efficient.

Each CRD instance will track all consumers of the volume.
In the case of `ReadWriteOnce` there will only be a single consumer, but for `ReadWriteMany` there can be multiple simultaneous consumers.
There will be an `Attachments` list that captures each instance of pod, node, and mount dir, which can all be used for the fencing checks described in the next section.

The full schema of the volume attachment CRD is shown below:
```go
type Volumeattachment struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Attachments       []Attachment `json:"attachments"`
}

type Attachment struct {
    Node         string `json:"node"`
    PodNamespace string `json:"podNamespace"`
    Pod          string `json:"pod"`
    MountDir     string `json:"mountDir,omitempty"`
}
```

#### Fencing
Ensuring either exclusive single client access (`ReadWriteOnce`) or shared multi-pod access (`ReadWriteMany`) to the persistent volume can be achieved by the Rook agent,
independently from the underlying storage protocol that is being used (RBD, NBD, iSCSI, etc.).
Remember that during the attach operation, the Rook agent will create a volume attach CRD.
When the agent performs the attach on its local node, it can create the CRD with information stating that the volume is now locked and who it is locked by.
Thus, the CRD itself is providing the accounting for the volume's lock in a generalized way, independent of the of the underlying storage protocol the agent chose to use for the volume.
Of course, this lock can also be augmented at the lower layer of the specific storage protocol that's being used.

##### **Race conditions**
While using the volume attach CRD for fencing, it is important to avoid race conditions.
For example, if two pods are attempting to use the same volume at the same time, we must still ensure that only one of them is granted access while the other ones fails.
This can be accomplished due to consistency guarantees that Kubernetes grants for all its objects, including CRDs, because they are stored in etcd.
For CRD create operations, an error will be returned if a CRD with that name already exists, and a duplicate CRD will not be created.
Furthermore, for update operations, an error will be returned if an attempt to update an existing CRD occurs that specifies an out of date version of the object.

##### **ReadWriteOnce**
For `ReadWriteOnce`, the agent needs to ensure that only one client is accessing the volume at any time.
During the `Mount()` operation, the agent will look for an existing CRD by its primary key, the PV ID.
If no CRD currently exists, then the agent will create one, signifying that it has won exclusive access on the volume, then proceed with the attach and mount.
The CRD that the agent creates will contain the pod, node and mount dir of the current attachment.

If a CRD does already exist, the agent will check its existing attachments list.
If the list specifies that the volume is attached to a different pod than the one we are currently mounting for, then another consumer already has exclusive access of the volume and the agent must honor `ReadWriteOnce` and fail the operation.
However, if the previous attachment is for the **same** pod and namespace that we are currently mounting for, this means that the volume is being failed over to a new node and was not properly cleaned up on its previous node.
Therefore, the agent will "break" the old lock by removing the old attachment entry from the list and adding itself, then continuing with attaching and mounting as usual.

##### **ReadWriteMany**
For `ReadWriteMany`, the agent will allow multiple entries in the attachments list of the CRD.  When `Mount()` is called, the agent will either create a new CRD instance if it does not already exist, or simply add a new attachment entry for itself to the existing CRD.

##### **Detach**
During the detach operation, the CRD will be deleted to signify that the volume has been unlocked (or updated to remove the entry from the attachment list of the CRD for the case of `ReadWriteMany`).

However, if the node where the volume was attached dies, the agent on that node may not get a chance to perform the detach and update the CRD instance.
In this case, we will need to clean up the stale record and potentially perform the detach later on if the node returns.
This will be performed in two separate ways:
1. As previously mentioned, the agent will check for existing attachments when it is requested to `Attach()`.  If it finds that the attachment belongs to a node that no longer exists, it will "break" the lock as described above.
1. Periodically (once per day), the operator will scan all volume attachment CRD instances, looking for any that belong to a node that no longer exists.

In both cases, when a "stale" attachment record is found, its details will be added to a volume attachment garbage collection list, indexed by node name.
Upon start up of a Rook agent on a node, as well as periodically, the agent can look at this GC list to see if any are for the node its running on.
If so, the agent will attempt to detach (if the device still exists) and then remove the entry from the GC list.
Additionally, if there are "stale" records that are no longer applicable for a given node (e.g., a node went down but then came back up), the agent should clean up those invalid records as well.

#### Security
The only interface for communicating with and invoking operations on the Rook agent is the Unix domain socket.
This socket will have read/write only accessible by `root` and it is only accessible on the local node (not remotely accessible).

### **Cluster Cleanup**
Rook makes storage as a service a deeply integrated part of the Kubernetes cluster as opposed to an external entity.
This integration makes more attention to lifecycle management of the storage components necessary.

#### Hung RBD Kernel Module
If the monitor pods of a Rook cluster are no longer accessible while block storage is mapped to a node, the kernel RBD module will be hung and require a [power cycle of the machine](https://github.com/rook/rook/issues/376#issuecomment-318803799).

The Rook agent can help mitigate this scenario, by watching for the cluster CRD delete event.
When a Rook cluster is being deleted, there may still be consumers of the storage in the cluster in the form of pods with PVCs.
When a Rook agent receives a cluster CRD delete event, they will respond by checking for any Rook storage on the local node they are running on and then forcefully remove them.

To forcefully remove the storage from local pods, the agent will perform the following sequence of steps for each Rook PVC:
```bash
$ kubectl delete pvc <pvc name>
$ sudo rbd unmap -o force /dev/rbdX
# wait for it to time out or send SIGINT
$ kubectl delete pv <pv name>
$ sudo umount <mount point>
```

As mentioned above, each agent will be watching for events on cluster CRDs.
Even in a large scale cluster, this is appropriate to have many watchers on the cluster CRDs for a couple reasons:
1. Cluster CRD events are relatively rare since they are tied to cluster lifecycles
1. Each agent truly does need to be informed about cluster CRD events since they all may need to act on them

## **Open Questions**
1. Reliability: Does the centralized `Detach()` fallback by the operator work reliably when the node consuming the volume has died?  We will vet this further while we are testing the implementation under real world scenarios.
1. Security: Are there further security considerations that need to be made?
1. Portability: Since this design does not need to run anything on the master node (control plane), it is fairly portable across Kubernetes deployments and environments.  However, some environments, such as Google Container Engine, do not have suitable kernel drivers for I/O traffic.  Those environments need to be updated with a lowest common denominator kernel driver, such as `NBD`.
