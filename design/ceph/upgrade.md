# **Rook Cluster Upgrades**

## **Overview**
Over time, new versions with improvements to the Rook software will be released and Rook clusters that have already been deployed should be upgraded to the newly released version.
Being able to keep the deployed software current is an important part of managing the deployment and ensuring its health.
In the theme of Rook's orchestration and management capabilities making the life of storage admins easier, this upgrade process should be both automatic and reliable.
This document will describe a proposed design for the upgrading of Rook software as well as pose questions to the community for feedback so that we can deliver on the goal of automatic and reliable upgrades.

## **Goal**
In order for software upgrade support in Rook to be considered successful, the goals listed below should be met.
Note that these goals are for a long term vision and are not all necessarily deliverable within the v0.6 release time frame.
* **Automatic:** When a new version of Rook is released and the admin has chosen to start the upgrade, a live cluster should be able to update all its components to the new version without further user intervention.
* **No downtime:** During an upgrade window, there should be **zero** downtime of cluster functionality.
  * The upgrade process should be carried out in a rolling fashion so that not all components are being updated simultaneously.
The cluster should be maintained in a healthy state the entire time.
* **Migrations:** Breaking changes, as well as schema and data format changes should be handled through an automated migration processes.
* **Rollback:** In the event that the upgrade is not successful, the Rook software should be rolled back to the previous version and cluster health should be restored.

## **User Guide**
Until automated upgrade support is available in Rook, we have authored a user guide that walks you through the steps to upgrade the software in a Rook cluster.
Consideration is also provided in the guide for how to verify the cluster remains healthy during and after the upgrade process.
Please refer to the [Rook Upgrade User Guide](/Documentation/Upgrade/rook-upgrade.md) to learn more about the current Rook upgrade process.

## **Detailed Design**
The responsibility for performing and orchestrating an upgrade will be handled by an upgrade controller that runs as part of the Rook operator, in the same pod and process (similar to how the Rook volume provisioner is run).
This controller will be responsible for carrying out the sequence of steps for updating each individual Rook component.
Additionally, the controller will monitor cluster and component health during the upgrade process, taking corrective steps to restore health, up to and including a full rollback to the old version.

### **Prerequisites**
In order for the upgrade controller to begin an upgrade process, the following conditions must be met:
* The cluster should be in a healthy state in accordance with our defined [health verification checks](#upgrade-health-verification).
The upgrade controller should not begin an upgrade if the cluster is currently unhealthy.
* Metadata for pods must be persistent.  If config files and other metadata only resides on an ephemeral empty dir for the pods (i.e., `dataDirHostPath` is not set), then the upgrade controller will not perform an upgrade.

### **General Sequence**
This section describes in a broad sense the general sequence of steps for upgrading a Rook cluster after a new Rook software version is released, e.g. `v0.6.1`.
Note that this sequence is modeled after the [Rook Upgrade User Guide](/Documentation/Upgrade/rook-upgrade.md), including the cluster health checks described in the [health verification section](/Documentation/Upgrade/rook-upgrade.md#health-verification).

#### **Rook System Namespace**
The Rook system namespace contains the single control plane for all Rook clusters in the environment.
This system namespace should be upgraded first before any individual clusters are upgraded.

**Operator:** The operator pod itself is upgraded first since it is the host of the upgrade controller.
If there is any new upgrade logic or any migration needed, the new version of the upgrade controller would know how to perform it, so it needs to be updated first.
This will be a manual operation by the admin, ensuring that they are ready for their cluster to begin the upgrade process:
```bash
kubectl set image deployment/rook-operator rook-operator=rook/rook:v0.6.1
```
This command will update the image field of the operator's pod template, which will then begin the process of the deployment that manages the operator pod to terminate the pod and start a new one running the new version in its place.

**Agents:** The Rook agents will also be running in the Rook system namespace since they perform operations for all Rook clusters in the environment.
When the operator pod comes up on a newer version than the agents, it will use the Kubernetes API to update the image field of the agent's pod template.
After this update, it will then terminate each agent pod in a rolling fashion so that their managing daemon set will replace them with a new pod on the new version.

Once the operator and all agent pods are running and healthy on the new version, the administrator is free to begin the upgrade process for each of their Rook clusters.

#### **Rook Cluster(s)**
1. The Rook operator, at startup after being upgraded, iterates over each Cluster CRD instance and proceeds to verify desired state.
    1. If the Rook system namespace upgrade described above has not yet occurred, then the operator will delay upgrading a cluster until the system upgrade is completed.  The operator should never allow a cluster's version to be newer than its own version.
1. The upgrade controller begins a reconciliation to bring the cluster's actual version value in agreement with the desired version, which is the container version of the operator pod.
As each step in this sequence begins/ends, the status field of the cluster CRD will be updated to indicate the progress (current step) of the upgrade process.
This will help the upgrade controller resume the upgrade if it were to be interrupted.
Also, each step should be idempotent so that if the step has already been carried out, there will be no unintended side effects if the step is resumed or run again.
1. **Mons:** The monitor pods will be upgraded in a rolling fashion.  **For each** monitor, the following actions will be performed by the upgrade controller:
    1. The `image` field of the pod template spec will be updated to the new version number.
    Then the pod will be terminated, allowing the replica set that is managing it to bring up a new pod on the new version to replace it.
    1.  The controller will verify that the new pod is on the new version, in the `Running` state, and that the monitor returns to `in quorum` and has a Ceph status of `OK`.
    The cluster health will be verified as a whole before moving to the next monitor.
1. **Ceph Managers:** The Ceph manager pod will be upgraded next by updating the `image` field on the pod template spec.
The deployment that is managing the pod will then terminate it and start a new pod running the new version.
    1. The upgrade controller will verify that the new pod is on the new version, in the `Running` state and that the manager instance shows as `Active` in the Ceph status output.
1. **OSDs:** The OSD pods will be upgraded in a rolling fashion after the monitors.  **For each** OSD, the following actions will take place:
    1. The `image` field of the pod template spec will be updated to the new version number.
    1. The lifecycle management of OSDs can be done either as a whole by a single daemon set or individually by a replica set per OSD.
    In either case, each individual OSD pod will be terminated so that its managing controller will respawn a new pod on the new version in its place.
    1. The controller will verify that each OSD is running the new version and that they return to the `UP` and `IN` statuses.
    Placement group health will also be verified to ensure all PGs return to the `active+clean` status before moving on.
1. If the user has installed optional components, such as object storage (**RGW**) or shared file system (**MDS**), they will also be upgraded to the new version.
They are both managed by deployments, so the upgrade controller will update the `image` field in their pod template specs which then causes the deployment to terminate old pods and start up new pods on the new versions to replace them.
    1.  Cluster health and object/file functionality will be verified before the upgrade controller moves on to the next instances.

### **Upgrade Health Verification**
As mentioned previously, the manual health verification steps found in the [upgrade user guide](/Documentation/Upgrade/rook-upgrade.md#health-verification) will be used by the upgrade controller, in an automated fashion, to ensure the cluster is healthy before proceeding with the upgrade process.
This approach of upgrading one component, verifying health and stability, then upgrading the next component can be viewed as a form of [canary deployment](https://kubernetes.io/docs/concepts/cluster-administration/manage-deployment/#canary-deployments).

Here is a quick summary of the standard health checks the upgrade controller should perform:
* All pods are in the `Running` state and have few, if any, restarts
  * No pods enter a crash loop backoff
* Overall status: The overall cluster status is `OK` and there are no warning or error status messages displayed.
* Monitors:  All of the monitors are `in quorum` and have individual status of `OK`.
* OSDs: All OSDs are `UP` and `IN`.
* MGRs: All Ceph managers are in the `Active` state.
* Placement groups: All PGs are in the `active+clean` state.

#### Pod Readiness/Liveness Probes
To further supplement the upgrade controller's ability to determine health, as well as facilitate the built-in Kubernetes upgrade capabilities, the Rook pods should implement [liveness and readiness probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/) when possible.
For pods that implement these probes, the upgrade controller can check them as another data point in determining if things are healthy before proceeding with the upgrade.

### **Rollback**
If the upgrade controller observes the cluster to be in an unhealthy state (that does not recover) during the upgrade process, it will need to roll back components in the cluster to the previous stable version.
This is possible due to the rolling/canary approach of the upgrade controller.
To roll a component back to the previous version, the controller will simply set the `image` field of the pod template spec to the previous version then terminate each pod to allow their managing controller to start a new pod on the *old* version to replace it.

The hope is that cluster health and stability will be restored once it has been rolled back to the previous version, but it is possible that simply rolling back the version may not solve all cases of cluster instability that begin during an upgrade process.
We will need more hands on experience with cluster upgrades in order to improve both upgrade reliability and rollback effectiveness.

### **Upgrade Tools**
We should consider implementing status commands that will help the user monitor and verify the upgrade progress and status.
Some examples for potential new commands would be:
* `rook versions`: This command would return the version of all Rook components in the cluster, so they can see at a glance which components have finished upgrading.
This is similar to the [`ceph versions` command](http://ceph.com/community/new-luminous-upgrade-complete/).
* `rook status --upgrade`:  This command would return a summary, retrieved from the upgrade controller, of the most recent completed steps and status of the upgrade that it is currently working on.

### **Migrations and Breaking Changes**
When a breaking change or a data format change occurs, the upgrade controller will have the ability to automatically perform the necessary migration steps during the upgrade process.
While migrations are possible, they are certainly not desirable since they require extra upgrade logic to be written and tested, as well as providing new potential paths for failure.
Going forward, it will be important for the Rook project to increase our discipline regarding the introduction of breaking changes.
We should be **very** careful about adding any new code that requires a migration during the update process.

### **Kubernetes Built-in Support**
Kubernetes has some [built-in support for rolling updates](https://kubernetes.io/docs/tasks/run-application/rolling-update-replication-controller/) with the `kubectl rolling-update` command.
Rook can potentially take advantage of this support for our replication controllers that have multiple stateless pods deployed, such as RGW.
This support is likely not a good fit for some of the more critical and sensitive components in the cluster, such as monitors, that require careful observation to ensure health is maintained and quorum is reestablished.

If the upgrade controller uses the built-in rolling update support for certain stateless components, it should still verify all cluster health checks before proceeding with the next set of components.

### **Synchronization**
The upgrade process should be carefully orchestrated in a controlled manner to ensure reliability and success.
Therefore, there should be some locking or synchronization that can ensure that while an upgrade is in progress, other changes to the cluster cannot be made.
For example, if the upgrade controller is currently rolling out a new version, it should not be possible to modify the cluster CRD with other changes, such as removing a node from the cluster.
This could be done by the operator stopping its watches on all CRDs or it could choose to simply return immediately from CRD events while the upgrade is in progress.

There are also some mechanisms within Ceph that can help the upgrade proceed in a controlled manner.
For example, the `noout` flag can be set in the Ceph cluster, indicating that while OSDs will be taken down to upgrade them, they should not be marked out of the cluster, which would trigger unnecessary recovery operations.
The [Ceph Luminous upgrade guide](http://ceph.com/releases/v12-1-4-luminous-rc-released/#upgrading) recommends setting the `noout` flag for the duration of the upgrade.
Details of the `noout` flag can be found in the [Ceph documentation](http://docs.ceph.com/docs/giant/rados/troubleshooting/troubleshooting-osd/#stopping-w-out-rebalancing).

### **Scalability**
For small clusters, the process of upgrading one pod at a time should be sufficient.
However, for much later clusters (100+ nodes), this would result in an unacceptably long upgrade window duration.
The upgrade controller should be able to batch some of its efforts to upgrade multiple pods at once in order to finish an upgrade in a more timely manner.

This batching should not be done across component types (e.g. upgrading mons and OSDs at the same time), those boundaries where the health of the entire cluster is verified should still exist.
This batching should also not be done for monitors as there are typically only a handful of monitors servicing the entire cluster and it is not recommended to have multiple monitors down at the same time.

But, the upgrade controller should be able to slowly increase its component update batch size as it proceeds through some other component types, such as OSDs, MDS and RGW.
For example, in true canary deployment fashion, a single OSD could be upgraded to the new version and OSD/cluster health will be verified.
Then two OSDs could be updated at once and verification occurs again, followed by four OSDs, etc. up to a reasonable upper bound.
We do not want too many pods going down at one time, which could potentially impact cluster health and functionality, so a sane upper bound will be important.

### **Troubleshooting**
If an upgrade does not succeed, especially if the rollback effort also fails, we want to have some artifacts that are accessible by the storage administrator to troubleshoot the issue or to reach out to the Rook community for help.
Because the upgrade process involves terminating pods and starting new ones, we need some strategies for investigating what happened to pods that may no longer be alive.
Listed below are a few techniques for accessing debugging artifacts from pods that are no longer running:
* `kubectl logs --previous ${POD_NAME} ${CONTAINER_NAME}` allows you to retrieve logs from a previous instance of a pod (e.g. a pod that crashed but is not yet terminated)
* `kubectl get pods --show-all=true` will list all pods, including older versioned pods that were terminated in order to replace them with pods running the newer version.
* The Rook operator logs (which host the upgrade controller output) should be thorough and verbose about the following:
  * The sequence of actions it took during the upgrade
  * The replication controllers (e.g. daemon set, replica set, deployment) that it modified and the pod names that it terminated
  * All health check status and output it encountered

## **Next Steps**
We have demonstrated that Rook is upgradable with the manual process outlined in the [Rook Upgrade User Guide](/Documentation/Upgrade/rook-upgrade.md).
Fully automated upgrade support has been described within this design proposal, but will likely need to be implemented in an iterative process, with lessons learned along the way from pre-production field experience.

The next step will be to implement the happy path where the upgrade controller automatically updates all Rook components in the [described sequence](#general-sequence) and stops immediately if any health checks fail and the cluster does not return to a healthy functional state.

Handling failure cases with rollback as well as handling migrations and breaking changes will likely be implemented in future milestones, along with reliability and stability improvements from field and testing experience.

## **Open Questions**
1. What other steps can be taken to restore cluster health before resorting to rollback?
1. What do we do if rollback doesn't succeed?
1. What meaningful liveness/readiness probes can our pods implement?
