{{/*
Define the clusterName as defaulting to the release namespace
*/}}
{{- define "clusterName" -}}
{{ .Values.clusterName | default .Release.Namespace }}
{{- end }}

{{/*
Return the target Kubernetes version.
*/}}
{{- define "capabilities.kubeVersion" -}}
{{- default .Capabilities.KubeVersion.Version .Values.kubeVersion -}}
{{- end }}

{{/*
Return the appropriate apiVersion for ingress.
*/}}
{{- define "capabilities.ingress.apiVersion" -}}
{{- if semverCompare "<1.19-0" (include "capabilities.kubeVersion" .) -}}
{{- print "networking.k8s.io/v1beta1" -}}
{{- else -}}
{{- print "networking.k8s.io/v1" -}}
{{- end }}
{{- end }}
