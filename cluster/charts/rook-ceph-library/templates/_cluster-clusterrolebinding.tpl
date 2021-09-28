{{/*
ClusterRoleBindings needed for running a Rook CephCluster
*/}}
{{- define "rook-ceph-library.cluster-clusterrolebindings" }}
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {{ printf "rook-ceph-mgr-cluster--%s" .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
  - kind: ServiceAccount
    name: rook-ceph-mgr
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
# Allow the ceph osd to access cluster-wide resources necessary for determining their topology location
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: {{ printf "rook-ceph-osd--%s" .Release.Namespace }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-osd
subjects:
  - kind: ServiceAccount
    name: rook-ceph-osd
    namespace: {{ .Release.Namespace }} # namespace:cluster
{{- end }}
