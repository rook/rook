---
title: OpenShift
---

# OpenShift

[OpenShift](https://www.openshift.com/) adds a number of security and other enhancements to Kubernetes. In particular, [security context constraints](https://blog.openshift.com/understanding-service-accounts-sccs/) allow the cluster admin to define exactly which permissions are allowed to pods running in the cluster. You will need to define those permissions that allow the Rook pods to run.

The settings for Rook in OpenShift are described below, and are also included in the [example yaml files](https://github.com/rook/rook/blob/master/deploy/examples):

* [`operator-openshift.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/operator-openshift.yaml): Creates the security context constraints and starts the operator deployment
* [`object-openshift.yaml`](https://github.com/rook/rook/blob/master/deploy/examples/object-openshift.yaml): Creates an object store with rgw listening on a valid port number for OpenShift

## TL;DR

To create an OpenShift cluster, the commands basically include:

```console
oc create -f crds.yaml -f common.yaml
oc create -f operator-openshift.yaml
oc create -f cluster.yaml
```

## Rook Privileges

To orchestrate the storage platform, Rook requires the following access in the cluster:

* Create `hostPath` volumes, for persistence by the Ceph mon and osd pods
* Run pods in `privileged` mode, for access to `/dev` and `hostPath` volumes
* Host networking for the Rook agent and clusters that require host networking
* Ceph OSDs require host PIDs for communication on the same node

## Security Context Constraints

Before starting the Rook operator or cluster, create the security context constraints needed by the Rook pods. The following yaml is found in `operator-openshift.yaml` under `/deploy/examples`.

!!! hint
    Older versions of OpenShift may require `apiVersion: v1`.

Important to note is that if you plan on running Rook in namespaces other than the default `rook-ceph`, the example scc will need to be modified to accommodate for your namespaces where the Rook pods are running.

To create the scc you will need a privileged account:

```console
oc login -u system:admin
```

We will create the security context constraints with the operator in the next section.

## Rook Settings

There are some Rook settings that also need to be adjusted to work in OpenShift.

### Operator Settings

There is an environment variable that needs to be set in the operator spec that will allow Rook to run in OpenShift clusters.

* `ROOK_HOSTPATH_REQUIRES_PRIVILEGED`: Must be set to `true`. Writing to the hostPath is required for the Ceph mon and osd pods. Given the restricted permissions in OpenShift with SELinux, the pod must be running privileged in order to write to the hostPath volume.

```yaml
- name: ROOK_HOSTPATH_REQUIRES_PRIVILEGED
  value: "true"
```

Now create the security context constraints and the operator:

```console
oc create -f operator-openshift.yaml
```

### Cluster Settings

The cluster settings in `cluster.yaml` are largely isolated from the differences in OpenShift. There is perhaps just one to take note of:

* `dataDirHostPath`: Ensure that it points to a valid, writable path on the host systems.

### Object Store Settings

In OpenShift, ports less than 1024 cannot be bound. In the [object store CRD](../Storage-Configuration/Object-Storage-RGW/object-storage.md), ensure the port is modified to meet this requirement.

```yaml
gateway:
  port: 8080
```

You can expose a different port such as `80` by creating a service.

A sample object store can be created with these settings:

```console
oc create -f object-openshift.yaml
```
