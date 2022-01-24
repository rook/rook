{{/*
Common labels
*/}}
{{- define "library.rook-ceph.labels" -}}
app.kubernetes.io/part-of: rook-ceph-operator
app.kubernetes.io/managed-by: helm
app.kubernetes.io/created-by: helm
helm.sh/chart: "{{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}"
{{- end }}
