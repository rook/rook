---
title: RBAC Security
weight: 14
indent: true
---

# RBAC Security

## Cluster Role
Creating the Rook operator requires privileges for setting up RBAC. To launch the operator you need to have created your user certificate that is bound to ClusterRole `cluster-admin`.

One simple way to achieve it is to assign your certificate with the `system:masters` group:
```
-subj "/CN=admin/O=system:masters"
```

`system:masters` is a special group that is bound to `cluster-admin` ClusterRole, but it can't be easily revoked so be careful with taking that route in a production setting.
Binding individual certificate to ClusterRole `cluster-admin` is revocable by deleting the ClusterRoleBinding.

## RBAC for PodSecurityPolicies

If you have activated the [PodSecurityPolicy Admission Controller](https://kubernetes.io/docs/admin/admission-controllers/#podsecuritypolicy) and thus are
using [PodSecurityPolicies](https://kubernetes.io/docs/concepts/policy/pod-security-policy/), you will require additional `(Cluster)RoleBindings`
for the different `ServiceAccounts` Rook uses to start the Rook Storage Pods.

**Note**: You do not have to perform these steps if you do not have the `PodSecurityPolicy` Admission Controller activated!  

##### PodSecurityPolicy

You need one `PodSecurityPolicy` that allows privileged `Pod` execution. Here is an example:

```yaml
apiVersion: extensions/v1beta1
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
  hostIPC: true
  hostNetwork: false
```

**Hint**: Allowing `hostNetwork` usage is required when using `hostNetwork: true` in the Cluster `CustomResourceDefinition`!
You are then also required to allow the usage of `hostPorts` in the `PodSecurityPolicy`. The given port range is a minimal
working recommendation for a Rook Ceph cluster:
 ```yaml
   hostPorts:
     # Ceph ports
     - min: 6789
       max: 7300
     # Ceph MGR Prometheus Metrics
     - min: 9283
       max: 9283
```

##### ClusterRole and ClusterRoleBinding

Next up you require a `ClusterRole` and a corresponding `ClusterRoleBinding`, which enables the Rook Agent `ServiceAccount` to run the rook-ceph-agent `Pods` on all nodes
with privileged rights. Here are the definitions:

```yaml
# privilegedPSP grants access to use the privileged PSP.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: privileged-psp-user
rules:
- apiGroups:
  - extensions
  resources:
  - podsecuritypolicies
  resourceNames:
  - privileged
  verbs:
  - use

```
and
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph-system
---
# Allow the rook-ceph-system serviceAccount to use the privileged PSP
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-ceph-system-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph-system
```

Save these definitions to one or multiple yaml files and create them by executing `kubectl apply -f <nameOfYourFile>.yaml`

You will also require two more `RoleBindings` for each Rook Cluster you deploy:
Create these two `RoleBindings` in the Namespace you plan to deploy your Rook Cluster into (default is "rook" namespace):

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
# Allow the default serviceAccount to use the priviliged PSP
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-default-psp
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: default
  namespace: rook-ceph
---
# Allow the rook-ceph-osd serviceAccount to use the privileged PSP
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-osd-psp
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: privileged-psp-user
subjects:
- kind: ServiceAccount
  name: rook-ceph-cluster
  namespace: rook-ceph
```
