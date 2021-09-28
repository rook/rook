{{/*
Define the clusterName as defaulting to the release namespace
*/}}
{{- define "clusterName" -}}
{{ .Values.clusterName | default .Release.Namespace }}
{{- end -}}
