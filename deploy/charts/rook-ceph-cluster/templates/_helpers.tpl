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

{{- /*
  Generate StorageClass parameters with smart defaults for block storage
*/}}
{{- define "rook-ceph-cluster.blockStorageClassParameters" -}}
{{- $blockPool := .blockPool -}}
{{- $storageClass := .storageClass -}}
{{- $root := .root -}}
pool: {{ $blockPool.name | quote }}
clusterID: {{ $root.Release.Namespace | quote }}
{{- with $storageClass.parameters }}
{{- tpl (. | toYaml) $root | nindent 0 }}
{{- end }}
{{- end -}}
