{{- /*
  Define the clusterName as defaulting to the release namespace
*/}}
{{- define "clusterName" -}}
{{ .Values.clusterName | default .Release.Namespace }}
{{- end }}

{{- /*
  Return the target Kubernetes version.
*/}}
{{- define "capabilities.kubeVersion" -}}
{{ .Values.kubeVersion | default .Capabilities.KubeVersion.Version -}}
{{- end }}
