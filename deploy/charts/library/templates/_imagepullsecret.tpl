{{/*
Define imagePullSecrets option to pass to all service accounts
*/}}
{{- define "library.imagePullSecrets" }}
{{- if .Values.imagePullSecrets }}
imagePullSecrets:
{{ toYaml .Values.imagePullSecrets }}
{{- else }}
  {{/* if the secrets are not included, include a comment for generating common.yaml */}}
# imagePullSecrets:
#   - name: my-registry-secret
{{- end }}
{{- end }}
