{{/*
Roles needed for running a Rook CephCluster
*/}}
{{- define "library.cluster.roles" }}
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  # this is needed for rook's "key-management" CLI to fetch the vault token from the secret when
  # validating the connection details and for key rotation operations.
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "delete"]
  - apiGroups: ["ceph.rook.io"]
    resources: ["cephclusters", "cephclusters/finalizers"]
    verbs: ["get", "list", "create", "update", "delete"]
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-rgw
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  # Placeholder role so the rgw service account will
  # be generated in the csv. Remove this role and role binding
  # when fixing https://github.com/rook/rook/issues/10141.
  - apiGroups:
      - ""
    resources:
      - configmaps
    verbs:
      - get
---
# Aspects of ceph-mgr that operate within the cluster's namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  - apiGroups:
      - ""
    resources:
      - pods
      - services
      - pods/log
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - delete
  - apiGroups:
      - batch
    resources:
      - jobs
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - delete
  - apiGroups:
      - ceph.rook.io
    resources:
      - cephclients
      - cephclusters
      - cephblockpools
      - cephfilesystems
      - cephnfses
      - cephobjectstores
      - cephobjectstoreusers
      - cephobjectrealms
      - cephobjectzonegroups
      - cephobjectzones
      - cephbuckettopics
      - cephbucketnotifications
      - cephrbdmirrors
      - cephfilesystemmirrors
      - cephfilesystemsubvolumegroups
      - cephblockpoolradosnamespaces
      - cephcosidrivers
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - delete
      - patch
  - apiGroups:
      - apps
    resources:
      - deployments/scale
      - deployments
    verbs:
      - patch
      - delete
  - apiGroups:
      - ''
    resources:
      - persistentvolumeclaims
    verbs:
      - delete
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  - apiGroups:
      - ""
    resources:
      - pods
      - configmaps
    verbs:
      - get
      - list
      - watch
      - create
      - update
      - delete
---
# Aspects of ceph osd purge job that require access to the cluster namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-purge-osd
  namespace: {{ .Release.Namespace }} # namespace:cluster
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "delete"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "update", "delete", "list"]
{{- end }}
