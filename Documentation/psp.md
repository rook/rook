---
title: Pod Security Policies
weight: 1400
indent: true
---
{% assign url = page.url | split: '/' %}
{% assign currentVersion = url[3] %}
{% if currentVersion != 'master' %}
{% assign branchName = currentVersion | replace: 'v', '' | prepend: 'release-' %}
{% else %}
{% assign branchName = currentVersion %}
{% endif %}

# Using Rook with Pod Security Policies

## Cluster Role

> **NOTE**: Cluster role configuration is only needed when you are not already `cluster-admin` in your Kubernetes cluster!

Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate that is bound to ClusterRole `cluster-admin`.

One simple way to achieve it is to assign your certificate with the `system:masters` group:

```console
-subj "/CN=admin/O=system:masters"
```

`system:masters` is a special group that is bound to `cluster-admin` ClusterRole, but it can't be easily revoked so be careful with taking that route in a production setting.
Binding individual certificate to ClusterRole `cluster-admin` is revocable by deleting the ClusterRoleBinding.

## RBAC for PodSecurityPolicies

If you have activated the [PodSecurityPolicy Admission Controller](https://kubernetes.io/docs/admin/admission-controllers/#podsecuritypolicy) and thus are
using [PodSecurityPolicies](https://kubernetes.io/docs/concepts/policy/pod-security-policy/), you will require additional `(Cluster)RoleBindings`
for the different `ServiceAccounts` Rook uses to start the Rook Storage Pods.

Security policies will differ for different backends. See Ceph's Pod Security Policies set up in its
[common.yaml](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph/common.yaml)
for an example of how this is done in practice.

> **NOTE**: You do not have to perform these steps if you do not have the `PodSecurityPolicy` Admission Controller activated!

### PodSecurityPolicy

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

**Hint**: Allowing `hostNetwork` usage is required when using `hostNetwork: true` in a Cluster `CustomResourceDefinition`!
You are then also required to allow the usage of `hostPorts` in the `PodSecurityPolicy`. The given
port range will allow all ports:

```yaml
   hostPorts:
     # Ceph msgr2 port
     - min: 1
       max: 65535
```
