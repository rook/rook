---
title: Ceph cluster Pod Security Policies
weight: 1400
indent: true
---

# Using Rook-Ceph with Pod Security Policies (PSPs)

See the [Rook overall PSP document](./psp.md) before continuing on here with Ceph specifics.

##### PodSecurityPolicy

You need at least one `PodSecurityPolicy` that allows privileged `Pod` execution. Here is an example
that is reasonably pared down for Ceph, though more work to minimize permissions can be done:

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
    - 'configMap'
    - 'emptyDir'
    - 'projected'
    - 'secret'
    - 'downwardAPI'
    - 'hostPath'
    - 'flexVolume'
  hostPID: true
  # hostNetwork is required for using host networking
  hostNetwork: false
```

**Hint**: Allowing `hostNetwork` usage is required when using `hostNetwork: true` in the Cluster
Resource Definition! You are then also required to allow the usage of `hostPorts` in the
`PodSecurityPolicy`. The given port range is a minimal working recommendation for a Rook Ceph cluster:
 ```yaml
   hostPorts:
     # Ceph msgr2 port
     - min: 3300
       max: 3300
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
  name: psp:rook
rules:
- apiGroups:
  - policy
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
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: rook-ceph
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
# Allow the default serviceAccount to use the privileged PSP
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-default-psp
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
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
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: rook-ceph
---
# Allow the rook-ceph-mgr serviceAccount to use the privileged PSP
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-mgr-psp
  namespace: rook-ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: rook-ceph

```
