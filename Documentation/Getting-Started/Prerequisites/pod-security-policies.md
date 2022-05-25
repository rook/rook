---
title: Pod Security Policies
---

Rook requires privileges to manage the storage in your cluster. If you have Pod Security Policies enabled
please review this document. By default, Kubernetes clusters do not have PSPs enabled so you may
be able to skip this document.

If you are configuring Ceph on OpenShift, the Ceph walkthrough will configure the PSPs as well
when you start the operator with [operator-openshift.yaml](https://github.com/rook/rook/blob/master/deploy/examples/operator-openshift.yaml).

Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate that is bound to ClusterRole `cluster-admin`.

## RBAC for PodSecurityPolicies

If you have activated the [PodSecurityPolicy Admission Controller](https://kubernetes.io/docs/admin/admission-controllers/#podsecuritypolicy) and thus are
using [PodSecurityPolicies](https://kubernetes.io/docs/concepts/policy/pod-security-policy/), you will require additional `(Cluster)RoleBindings`
for the different `ServiceAccounts` Rook uses to start the Rook Storage Pods.

Security policies will differ for different backends. See Ceph's Pod Security Policies set up in
[common.yaml](https://github.com/rook/rook/blob/master/deploy/examples/common.yaml)
for an example of how this is done in practice.

## PodSecurityPolicy

You need at least one `PodSecurityPolicy` that allows privileged `Pod` execution. Here is an example
which should be more permissive than is needed for any backend:

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: privileged
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
    - '*'
  allowedCapabilities:
    - '*'
  hostPID: true
  # hostNetwork is required for using host networking
  hostNetwork: false
```

!!! hint
    Allowing `hostNetwork` usage is required when using `hostNetwork: true` in a Cluster `CustomResourceDefinition`!
    You are then also required to allow the usage of `hostPorts` in the `PodSecurityPolicy`. The given
    port range will allow all ports:

```yaml
   hostPorts:
     # Ceph msgr2 port
     - min: 1
       max: 65535
```
