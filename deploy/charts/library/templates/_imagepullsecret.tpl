{{/*
Define imagePullSecrets option to pass to all service accounts
*/}}
{{- define "library.imagePullSecrets" }}
{{- if .Values.imagePullSecrets }}
imagePullSecrets:
{{ toYaml .Values.imagePullSecrets }}
{{- end }}
{{- end }}
