{{- /*
  RoleBindings needed to enable Pod Security Policies for a CephCluster.
*/}}
{{- define "library.cluster.psp.rolebindings" -}}
{{- if semverCompare "<1.25.0-0" .Capabilities.KubeVersion.GitVersion -}}
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-default-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: default
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: rook-ceph-osd
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-rgw-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: rook-ceph-rgw
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: rook-ceph-mgr
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: rook-ceph-cmd-reporter
    namespace: {{ .Release.Namespace }} # namespace:cluster
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-purge-osd-psp
  namespace: {{ .Release.Namespace }} # namespace:cluster
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
  - kind: ServiceAccount
    name: rook-ceph-purge-osd
    namespace: {{ .Release.Namespace }} # namespace:cluster
{{- end }}
{{- end }}
