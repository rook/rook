{{- /*
  Define imagePullSecrets option to pass to all service accounts
*/}}
{{- define "library.imagePullSecrets" -}}
{{- if .Values.imagePullSecrets -}}
imagePullSecrets:
  {{- .Values.imagePullSecrets | toYaml | nindent 2 }}
{{- else -}}
{{- /* if the secrets are not included, include a comment for generating common.yaml */ -}}
# imagePullSecrets:
#   - name: my-registry-secret
{{- end }}
{{- end }}
