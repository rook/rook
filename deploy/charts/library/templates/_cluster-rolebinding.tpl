{{- /*
  RoleBindings needed for running a Rook CephCluster
*/}}
{{- define "library.cluster.rolebindings" -}}
---
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: {{ .Values.operatorNamespace | default .Release.Namespace }} # namespace:operator
---
# Allow the osd pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
  - kind: ServiceAccount
    name: rook-ceph-osd
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
# Allow the ceph mgr to access resources scoped to the CephCluster namespace necessary for mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr
subjects:
  - kind: ServiceAccount
    name: rook-ceph-mgr
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
# Allow the ceph mgr to access resources in the Rook operator namespace necessary for mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-system{{ include "library.suffix-cluster-namespace" . }}
  namespace: {{ .Values.operatorNamespace | default .Release.Namespace }} # namespace:operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-system
subjects:
  - kind: ServiceAccount
    name: rook-ceph-mgr
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
  - kind: ServiceAccount
    name: rook-ceph-cmd-reporter
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
# Allow the osd purge job to run in this namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-purge-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-purge-osd
subjects:
  - kind: ServiceAccount
    name: rook-ceph-purge-osd
    namespace: {{ .Release.Namespace }} # namespace:cluster
{{- end }}
