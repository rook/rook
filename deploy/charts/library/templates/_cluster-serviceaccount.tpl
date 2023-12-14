{{/*
ServiceAccounts needed for running a Rook CephCluster
*/}}
{{- define "library.cluster.serviceaccounts" }}
# Service account for Ceph OSDs
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{ include "library.imagePullSecrets" . }}
---
# Service account for Ceph mgrs
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{ include "library.imagePullSecrets" . }}
---
# Service account for the job that reports the Ceph version in an image
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{ include "library.imagePullSecrets" . }}
---
# Service account for job that purges OSDs from a Rook-Ceph cluster
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-purge-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
{{ include "library.imagePullSecrets" . }}
---
# Service account for RGW server
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-rgw
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{ include "library.imagePullSecrets" . }}
---
# Service account for other components
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-default
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    operator: rook
    storage-backend: ceph
{{ include "library.imagePullSecrets" . }}
{{ end }}
