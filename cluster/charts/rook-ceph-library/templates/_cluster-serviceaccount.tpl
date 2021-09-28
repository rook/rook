{{/*
ServiceAccounts needed for running a Rook CephCluster
*/}}
{{- define "rook-ceph-library.cluster-serviceaccounts" }}
# Service account for Ceph OSDs
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
{{- include "rook-ceph-library.imagePullSecrets" . }}
---
# Service account for Ceph mgrs
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
{{- include "rook-ceph-library.imagePullSecrets" . }}
---
# Service account for the job that reports the Ceph version in an image
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
{{- include "rook-ceph-library.imagePullSecrets" . }}
---
# Service account for job that purges OSDs from a Rook-Ceph cluster
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-purge-osd
{{- include "rook-ceph-library.imagePullSecrets" . }}
{{ end }}
