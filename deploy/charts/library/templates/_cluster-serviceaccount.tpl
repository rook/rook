{{- /*
  ServiceAccounts needed for running a Rook CephCluster
*/}}
{{- define "library.cluster.serviceaccounts" -}}
---
# Service account for Ceph OSDs
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for Ceph mgrs
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for the job that reports the Ceph version in an image
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for job that purges OSDs from a Rook-Ceph cluster
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-purge-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for RGW server
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-rgw
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for other components
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-default
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
---
# Service account for NVMe-oF gateway
kind: ServiceAccount
apiVersion: v1
metadata:
  name: rook-ceph-nvmeof
  namespace: {{ .Release.Namespace }} # namespace:cluster
  labels:
    {{- include "library.rook-ceph.labels" . | nindent 4 }}
{{- include "library.imagePullSecrets" . | nindent 0 }}
{{- end }}
