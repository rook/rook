# Rook-Ceph debugging

## Table of Contents <!-- omit in toc -->

- [Rook-Ceph debugging](#rook-ceph-debugging)
  - [Methods](#methods)
    - [Set operator log level to debug](#set-operator-log-level-to-debug)
    - [Use the toolbox pod](#use-the-toolbox-pod)
    - [Set Ceph log levels](#set-ceph-log-levels)
    - [Get container logs](#get-container-logs)
    - [Describe Deployments, Pods, and other Kubernetes resources](#describe-deployments-pods-and-other-kubernetes-resources)
    - [Stop the operator](#stop-the-operator)
    - [Restart the operator](#restart-the-operator)
    - [Restart a daemon pod](#restart-a-daemon-pod)
    - [Determine the node a Pod runs on](#determine-the-node-a-pod-runs-on)
  - [Debugging](#debugging)
    - [General](#general)
    - [OSDs](#osds)
    - [Mons](#mons)
    - [Problems with user applications mounting Rook Storage Classes (CSI)](#problems-with-user-applications-mounting-rook-storage-classes-csi)

## Methods
There are a number of basic actions a user might need to take during debugging. These actions are
defined here for reference when they are mentioned in more documentation below.

> **NOTE:** Please note that this document is not devoted to an in-depth explanation of what
> Kubernetes is, what its features are, how it is used, how to navigate it, or how to debug
> applications that run on it. This document will use Kubernetes terms, and users are expected to be
> empowered to look up Kubernetes information they do not already have. This document will connect
> dots to explain how to use Kubernetes tools to get information needed in the Rook-Ceph context and
> (when relevant) will briefly explain how Rook uses Kubernetes features.

### Set operator log level to debug
In general, the first place to look when encountering a failure is to get logs for the rook-ceph-operator pod. To get the best logs possible, set the operator log level to "DEBUG". There are many ways to do this, including modifying Helm's values.yaml or modifying the operator.yaml manifest. Regardless of method chosen, the log level can always be set by editing the deployment directly with kubectl.

```sh
kubectl --namespace rook-ceph edit deployment rook-ceph-operator
# manually edit ROOK_LOG_LEVEL 'env' value "DEBUG" (more logs) or "INFO" (less logs) as desired
```

After editing the deployment, the operator pod will restart automatically and will start outputting
logs with the new log level.

If you are experiencing a particular failure, it may take some time for the Rook operator to reach
the failure location again to report debug logs.

### Use the toolbox pod
Use the Rook toolbox pod to interface directly with the Ceph cluster via CLI. Enter into the toolbox
pod as shown below.
```
kubectl --namespace rook-ceph exec -it deploy/rook-ceph-tools -- bash
```

If the rook-ceph-tools deployment does not exist, it should be created using the toolbox.yaml manifest.

### Set Ceph log levels
To set log levels for Ceph daemons, it is advised to use the Ceph CLI from the toolbox pod.

### Get container logs
Getting logs for a container in Kubernetes can be fairly easy, but to get logs with more clarity, it
is slightly more complicated.

#### Quick and dirty <!-- omit in toc -->
At its simplest, get all the logs for all containers in a Pod as shown below. This is easy, but
since Rook-Ceph pods almost always contain init containers in addition to running containers, the
output does not make it clear which logs belong to makes debugging more difficult by not identifying
which logs belong to which container in a Pod.
```sh
# Get logs for all containers in a Pod
kubectl --namespace rook-ceph logs <pod-name> --all-containers
```

#### Manual, slow and more precise <!-- omit in toc -->
To get logs for a single container, first determine which containers belong to a pod, then request
logs only for the container desired.
```sh
# Get logs for a single container in a pod
kubectl --namespace rook-ceph get pod <pod-name> -o jsonpath='{..initContainers[*].name} {..containers[*].name} {"\n"}'
# ^ lists init containers followed by running containers
kubectl --namespace rook-ceph logs <pod-name> --container <container-name>
# ^ gets logs for a single container
```

#### Misc <!-- omit in toc -->
Logs can be followed and watched in real-time which is useful debugging a problem you are trying to
reproduce as shown in the example below.
```
kubectl --namespace rook-ceph logs <pod-name> --follow
```

You can view logs for previously-running containers in a pod. This is useful when a container/Pod
has encountered a failure and restarted where the currently running container has yet to fail. The
failure from the previous container can still be viewed.
```
kubectl --namespace rook-ceph logs <pod-name> --previous
```

### Describe Deployments, Pods, and other Kubernetes resources
All resources including Pods in Kubernetes can be "described". This will give more detailed
information about a resource. This information is critical to understanding how the Rook Operator
has configured Ceph resources, particularly Deployments.

#### Basic describe <!-- omit in toc -->
Basically detailed information about resources is easy to obtain.
```sh
kubectl --namespace rook-ceph describe <resource-type> <resource-name>
# e.g., kubectl --namespace rook-ceph describe deployment rook-ceph-operator
# e.g., kubectl --namespace rook-ceph describe pod rook-ceph-mon-a-69cd744894-zc9jm
```

#### JSON description (more advanced info) <!-- omit in toc -->
If you wish to use get JSON output from a detailed description (to use jq on the results or for
other reasons), that is possible. Additionally there are some much more advanced and not often
useful fields which are left out of "describe' output that are shown in JSON output.
```sh
kubectl --namespace rook-ceph get --output=json <resource-type> <resource-name>
```

### Stop the operator

Stopping the operator is done usually when the Rook-Ceph cluster will be modified manually in some
way. Do this when you don't want the operator to attempt to repair the cluster, either because it
would revert manual steps or because the operator could get into an error state because of changes
to the cluster. Stopping the operator is done in practice by scaling down the number of deployment
replicas to zero, thus keeping the original deployment intact but stopping the pod from continuing
to execute.

```sh
kubectl --namespace rook-ceph scale deployment --replicas=0 rook-ceph-operator
```

This should be undone by scaling the deployment back to 1 (and only 1) replica. This will start the
operator pod and thereby start the Rook operator running again.
```sh
kubectl --namespace rook-ceph scale deployment --replicas=1 rook-ceph-operator
```

### Restart the operator
In order to do a simple restart of the Rook operator, it does not need scaled down and scaled back
up again. This is as simple as deleting the Operator pod. The operator deployment will restart the
deleted pod almost immediately, thus restarting the Operator.
```sh
kubectl --namespace rook-ceph delete pod --selector app=rook-ceph-operator
```

### Restart a daemon pod
All Pods created by Rook are controlled using a Kubernetes Pod controller of some kind. This is most
often a Deployment. Therefore, deleting a Rook-Ceph Pod will never delete the Pod permanently. It
will be restarted almost immediately by Kubernetes. If you need to restart a Ceph (or CSI) daemon,
it can be done the same way as restarting the Rook operator.
```sh
kubectl --namespace rook-ceph delete pod <pod-name>
```

Whether or not the Pod is safe to be restarted cannot be said here. It cannot be guaranteed that the
daemon will be healthy upon restarting, though this is generally not an issue. Be mindful of SLAs
and failure domain considerations when restarting daemons

### Determine the node a Pod runs on
```sh
# Display all pods with node names next to them
kubectl --namespace rook-ceph get pod -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName
```

```sh
# Get the node name for only a single pod
kubectl --namespace rook-ceph get pod -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName <pod-name>
```

## Debugging
### General
General debugging of Rook can be difficult due to the number of moving parts involved. In general,
it is helpful to look at the following things for debugging Rook's behavior. The following should be
considered minimum necessary information to report when reporting bugs related to Rook.

* Logs from the pod of interest, whether it is failing or not behaving as expected
* A `kubectl describe` Kubernetes Pod controller (usually a Deployment) managing the pod
  * The Rook operator controls these resources in order to control how Ceph is run, and this is the
    de-facto representation of how Rook configures Ceph daemons
* Logs from the Rook operator (ALWAYS)
  * This helps identify what the Rook operator is observing and doing with respect to Ceph resources
    and how Rook is interpreting user configurations
* A `kubectl describe` of the CephCluster  and (if applicable) a `kubectl describe` of a relevant
  Rook-Ceph Custom Resource
  * This represents how the user has configured Rook
    e.g., if an MDS daemon is acting up, the CephFilesystem associated with the MDS is also needed

There are additional special considerations for some types of issues as noted below.

### OSDs
OSDs are not merely run by Rook. Rook provisions OSDs before running them using prepare Jobs. These
jobs create new OSDs and/or detect existing OSDs.

Debugging issues related to OSDs should also include the below.

* logs from `rook-ceph-osd-prepare-*` Pods from nodes on which failing or misconfigured OSDs are found
* `kubectl describe` output from the above `rook-ceph-osd-prepare-*` Pods

### Mons
Rook schedules Ceph mons by first scheduling what it calls "canary" Deployments/Pods. Rook runs mon
canary Deployments in Kubernetes that do not run mon daemons in order to observe where Kubernetes
schedules the Pods. After Rook observes this, it runs mon daemons by creating Deployments locked
onto the node it first observed Kubernetes schedule the mon to. It then deletes the canary
Deployments.

While all known cases are fixed, Rook might sometimes reach an edge case and fail to remove a mon
canary Deployment. In the worst case, this might prevent Rook from updating mons. It is generally
safe to stop the Rook operator and then delete the canary Deployments manually before resuming the
operator to resolve cases like this that might appear.

### Problems with user applications mounting Rook Storage Classes (CSI)
Problems with user applications mounting Rook storage are often problems with the CSI driver or
configuration. User applications request storage from Kubernetes which then requests storage from
Rook via the Container Storage Interface (CSI). The CSI driver and other CSI daemons need to be
inspected during debugging these types of issues in addition to the Rook operator logs. The CSI
driver in Rook is a scale-out application, and it's sometimes necessary to inspect several pods to
find relevant logs.

There is one CSI driver, but it has two modes: one for RBD, and one for CephFS. Be sure to focus on
the right set of CSI driver daemons for the issue you are debugging.

Firstly, get information about the user application. If logs from the application can be gotten,
that could be useful. Most importantly, it is necessary to see the messages which Kubernetes reports
for the user application's Pod and Pod Controller in the `kubectl describe` output of the Pod and
the Pod controller (e.g., Deployment, DaemonSet, StatefulSet, Job, etc.)

On the Rook CSI side of things, you will likely need to inspect logs from all `csi-*-provisioner-*`
Pods in order to find the provisioner which handled the CSI request for your issue. You should also
inspect logs from the `csi-*plugin-*` Pod which runs on the same node as the application
experiencing the issue. Also get `kubectl describe` output from the provisioner Deployment and
plugin DaemonSet.
