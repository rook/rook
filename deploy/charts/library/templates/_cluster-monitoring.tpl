{{/*
RBAC needed for enabling monitoring for a Rook CephCluster.
These should be scoped to the namespace where the CephCluster is located.
*/}}

{{- define "library.cluster.monitoring.roles" -}}
# ---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-monitoring
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  - apiGroups:
      - "monitoring.coreos.com"
    resources:
      - servicemonitors
      - prometheusrules
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - delete
---
# Allow management of monitoring resources in the mgr
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-monitoring-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
- apiGroups:
  - monitoring.coreos.com
  resources:
  - servicemonitors
  verbs:
  - get
  - list
  - create
  - update
{{- end }}

{{- define "library.cluster.monitoring.rolebindings" }}
# Allow the operator to get ServiceMonitors in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-monitoring
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-monitoring
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
# Allow creation of monitoring resources in the mgr
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-monitoring-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-monitoring-mgr
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
{{- end }}
