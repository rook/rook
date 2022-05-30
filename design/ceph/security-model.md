# Security Model

The Rook operator currently uses a highly privileged service account with permissions to create namespaces, roles, role bindings, etc. Our approach would not pass a security audit and this design explores an improvement to this. Furthermore given our use of multiple service accounts and namespace, setting policies and quotas is harder than it needs to be.

## Goals
 * Reduce the number of service accounts and privileges used by Rook
 * Reduce the number of namespaces that are used by Rook
 * Only use services accounts and namespaces used by the cluster admin -- this enables them to set security policies and quotas that rook adheres to
 * Continue to support a least privileged model

## What we do today

Today the cluster admin creates the rook system namespace, rook-operator service account and RBAC rules as follows:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-operator
  namespace: rook-system
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-operator
rules:
- apiGroups: [""]
  resources: ["namespaces", "serviceaccounts", "secrets", "pods", "services", "nodes", "nodes/proxy", "configmaps", "events", "persistenvolumes", "persistentvolumeclaims"]
  verbs: [ "get", "list", "watch", "patch", "create", "update", "delete" ]
- apiGroups: ["extensions"]
  resources: ["thirdpartyresources", "deployments", "daemonsets", "replicasets"]
  verbs: [ "get", "list", "watch", "create", "delete" ]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: [ "get", "list", "watch", "create", "delete" ]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings", "roles", "rolebindings"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses"]
  verbs: [ "get", "list", "watch", "delete" ]
- apiGroups: ["rook.io"]
  resources: ["*"]
  verbs: [ "*" ]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-operator
subjects:
- kind: ServiceAccount
  name: rook-operator
  namespace: rook-system
```

`rook-operator` is a highly privileged service account with cluster wide scope. It likely has more privileges than is currently needed, for example, the operator does not create namespaces today. Note the name `rook-system` and `rook-operator` are not important and can be set to anything.

Once the rook operator is up and running it will automatically create the service account for the rook agent and the following RBAC rules:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-agent
  namespace: rook-system
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-agent
rules:
- apiGroups: [""]
  resources: ["pods", "secrets", "configmaps", "persistenvolumes", "nodes", "nodes/proxy"]
  verbs: [ "get", "list" ]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses"]
  verbs: [ "get" ]
- apiGroups: ["rook.io"]
  resources: ["volumeattachment"]
  verbs: [ "get", "list", "watch", "create", "update" ]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-agent
subjects:
- kind: ServiceAccount
  name: rook-ceph-agent
  namespace: rook-system
```

When the cluster admin create a new Rook cluster they do so by adding a namespace and the rook cluster spec:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: mycluster
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: myrookcluster
  namespace: mycluster
	...
```

At this point the rook operator will notice that a new rook cluster CRD showed up and proceeds to create a service account for the `rook-api` and `rook-ceph-osd`. It will also use the `default` service account in the `mycluster` namespace for some pods.

The `rook-api` service account and RBAC rules are as follows:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-api
  namespace: mycluster
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-api
  namespace: mycluster
rules:
- apiGroups: [""]
  resources: ["namespaces", "secrets", "pods", "services", "nodes", "configmaps", "events"]
  verbs: [ "get", "list", "watch", "create", "update" ]
- apiGroups: ["extensions"]
  resources: ["thirdpartyresources", "deployments", "daemonsets", "replicasets"]
  verbs: [ "get", "list", "create", "update" ]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses"]
  verbs: [ "get", "list" ]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: [ "get", "list", "create" ]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-api
  namespace: mycluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-api
subjects:
- kind: ServiceAccount
  name: rook-api
  namespace: mycluster
```

The `rook-ceph-osd` service account and RBAC rules are as follows:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: mycluster
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: mycluster
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: mycluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: mycluster
```

## Proposed Changes

Just as we do today the cluster admin is responsible for creating the `rook-system` namespace. I propose we have a single service account in this namespace and call it `rook-system` by default. The names used are inconsequential and can be set to something different by the cluster admin.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: rook-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-system
  namespace: rook-system
```

The `rook-system` service account is responsible for launching all pods, services, daemonsets, etc. for Rook and should have enough privilege to do and nothing more. I've not audited all the RBAC rules but a good tool to do is [here](https://github.com/liggitt/audit2rbac). For example:

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-system
rules:
- apiGroups: [""]
  resources: ["pods", "services", "configmaps"]
  verbs: [ "get", "list", "watch", "patch", "create", "update, "delete" ]
- apiGroups: ["extensions"]
  resources: ["deployments", "daemonsets", "replicasets"]
  verbs: [ "get", "list", "watch", "patch", "create", "update, "delete" ]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: [ "get", "list", "watch", "patch", "create", "update, "delete" ]
- apiGroups: ["rook.io"]
  resources: ["*"]
  verbs: [ "*" ]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-system
  namespace: rook-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-system
subjects:
- kind: ServiceAccount
  name: rook-system
  namespace: rook-system
```

Notably absent here are privileges to set other RBAC rules and create read cluster-wide secrets and other resources. Because the admin created the `rook-system` namespace and service account they are free to set policies on them using PSP or namespace quotas.

Also note that while we use a `ClusterRole` for rook-system we only use a `RoleBinding` to grant it access to the `rook-system` namespace. It does not have cluster-wide privileges.

When creating a Rook cluster the cluster admin will continue to define the namespace and cluster CRD as follows:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: mycluster
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: myrookcluster
  namespace: mycluster
	...
```

In addition we will require that the cluster-admin define a service account and role binding as follows:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-cluster
  namespace: mycluster
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-cluster
  namespace: mycluster
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-cluster
  namespace: mycluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-system
subjects:
- kind: ServiceAccount
  name: rook-system
  namespace: rook-system
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-system
  namespace: mycluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-cluster
  namespace: rook-cluster
subjects:
- kind: ServiceAccount
  name: rook-cluster
  namespace: mycluster
```

This will grant the `rook-system` service account access to the new namespace and also setup a least privileged service account `rook-cluster` to be used for pods in this namespace that need K8S api access.

With this approach `rook-system` will only have access to namespaces nominated by the cluster admin. Also we will no longer create any service accounts or namespaces enabling admins to set stable policies and quotas.

Also all rook pods except the rook operator pod should run using `rook-cluster` service account in the namespace they're in.

### Supporting common namespaces

Finally, we should support running multiple rook clusters in the same namespaces. While namespaces are a great organizational unit for pods etc. they are also a unit of policy and quotas. While we can force the cluster admin to go to an approach where they need to manage multiple namespaces, we would be better off if we give the option to cluster admin decide how they use namespace.

For example, it should be possible to run rook-operator, rook-agent, and multiple independent rook clusters in a single namespace. This is going to require setting a prefix for pod names and other resources that could collide.

The following should be possible:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: myrook
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: red
  namespace: mycluster
	...
---
apiVersion: rook.io/v1alpha1
kind: Cluster
metadata:
  name: blue
  namespace: mycluster
	...
```
