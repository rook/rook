{{/*
Some ClusterRoles or Roles in the operator namespace need to be bound to service accounts in the
CephCluster namespace.
If the cluster namespace is the same as the operator namespace, we want a binding with a basic name.
  This is the case for the rook-ceph (Rook-Ceph Operator) chart.
If the cluster namespace is different from the operator namespace, we want to name the binding
  (in the operator namespace) with a suffixed that has the cluster namespace. This is the case for
  some instances of the rook-ceph-cluster (CephCluster) chart
*/}}

{{- define "library.suffix-cluster-namespace" -}}
{{/* the operator chart won't set .Values.operatorNamespace, so default to .Release.Namespace */}}
{{- $operatorNamespace := .Values.operatorNamespace | default .Release.Namespace -}}
{{- $clusterNamespace := .Release.Namespace -}}
{{- if ne $clusterNamespace $operatorNamespace -}}
{{ printf "-%s" $clusterNamespace }}
{{- end }}
{{- end }}
