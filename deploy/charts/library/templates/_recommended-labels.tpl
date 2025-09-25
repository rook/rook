{{- /*
  Modern recommended labels following Kubernetes best practices
  Based on: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
*/}}
{{- define "library.rook-ceph.labels" -}}
operator: rook
storage-backend: ceph
app.kubernetes.io/name: rook-ceph
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | default .Chart.Version }}
app.kubernetes.io/part-of: rook-ceph-operator
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/created-by: helm
helm.sh/chart: "{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}"
{{- end }}
