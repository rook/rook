/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package installer

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/tests/framework/utils"
)

const (
	// The version from which the upgrade test will start
	Version1_5 = "v1.5.8"
)

// CephManifestsV1_4 wraps rook yaml definitions
type CephManifestsV1_5 struct {
	imageTag string
}

func readManifestFromGithub(filename string) string {
	url := fmt.Sprintf("https://raw.githubusercontent.com/rook/rook/%s/cluster/examples/kubernetes/ceph/%s", Version1_5, filename)
	logger.Infof("Retrieving manifest: %s", url)
	// nolint:gosec G107 This is only test code and is expected to read from a url
	response, err := http.Get(url)
	if err != nil {
		panic(errors.Wrapf(err, "failed to read manifest from %s", url))
	}
	defer response.Body.Close()

	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		panic(errors.Wrapf(err, "failed to read content from %s", url))
	}
	return string(content)
}

func (m *CephManifestsV1_5) GetRookCRDs(v1ExtensionsSupported bool) string {
	if v1ExtensionsSupported {
		return readManifestFromGithub("crds.yaml")
	}
	return readManifestFromGithub("pre-k8s-1.16/crds.yaml")
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsV1_5) GetRookOperator(operatorNamespace string) string {
	var operatorManifest string
	openshiftEnv := ""

	if utils.IsPlatformOpenShift() {
		openshiftEnv = `
        - name: ROOK_HOSTPATH_REQUIRES_PRIVILEGED
          value: "true"
        - name: FLEXVOLUME_DIR_PATH
          value: "/etc/kubernetes/kubelet-plugins/volume/exec"`
		operatorManifest = getOpenshiftSCC(operatorNamespace)
	}

	operatorManifest = operatorManifest + `---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  - apps
  - extensions
  resources:
  - pods
  - configmaps
  - services
  - daemonsets
  - statefulsets
  - deployments
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - k8s.cni.cncf.io
  resources:
  - network-attachment-definitions
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rook-ceph-cluster-mgmt
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  - pods
  - pods/log
  - services
  - configmaps
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - apps
  - extensions
  resources:
  - deployments
  - daemonsets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rook-ceph-global
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  # Pod access is needed for fencing
  - pods
  # Node access is needed for determining nodes where mons should run
  - nodes
  - nodes/proxy
  - services
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
    # PVs and PVCs are managed by the Rook provisioner
  - persistentvolumes
  - persistentvolumeclaims
  - endpoints
  verbs:
  - get
  - list
  - watch
  - patch
  - create
  - update
  - delete
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
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
  - "*"
  verbs:
  - "*"
- apiGroups:
  - rook.io
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - policy
  - apps
  - extensions
  resources:
  # This is for the clusterdisruption controller
  - poddisruptionbudgets
  # This is for both clusterdisruption and nodedrain controllers
  - deployments
  - replicasets
  verbs:
  - "*"
- apiGroups:
  - healthchecking.openshift.io
  resources:
  - machinedisruptionbudgets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - machine.openshift.io
  resources:
  - machines
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - delete
- apiGroups:
  - storage.k8s.io
  resources:
  - csidrivers
  verbs:
  - create
  - delete
  - get
  - update
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-cluster
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  - nodes
  - nodes/proxy
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
  - list
  - get
  - watch
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
---
# Aspects of Rook Ceph Agent that require access to secrets
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rook-ceph-agent-mount
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
---
# Aspects of ceph-mgr that require access to the system namespace
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-system
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-object-bucket
  labels:
    operator: rook
    storage-backend: ceph
rules:
- apiGroups:
  - ""
  verbs:
  - "*"
  resources:
  - secrets
  - configmaps
- apiGroups:
    - storage.k8s.io
  resources:
    - storageclasses
  verbs:
    - get
    - list
    - watch
- apiGroups:
  - "objectbucket.io"
  verbs:
  - "*"
  resources:
  - "*"
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-object-bucket
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-object-bucket
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: ` + operatorNamespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
  labels:
    operator: rook
    storage-backend: ceph
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-global
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-global
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-rbd-plugin-sa
  namespace: ` + operatorNamespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-nodeplugin
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "update"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-nodeplugin
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-plugin-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: ClusterRole
  name: rbd-csi-nodeplugin
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-rbd-provisioner-sa
  namespace: ` + operatorNamespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-external-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents/status"]
    verbs: ["update"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments/status"]
    verbs: ["patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots/status"]
    verbs: ["update"]
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["create", "list", "watch", "delete", "get", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-provisioner-role
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: ClusterRole
  name: rbd-external-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: ` + operatorNamespace + `
  name: rbd-external-provisioner-cfg
rules:
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "delete", "update"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rbd-csi-provisioner-role-cfg
  namespace: ` + operatorNamespace + `
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: Role
  name: rbd-external-provisioner-cfg
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-cephfs-plugin-sa
  namespace: ` + operatorNamespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-nodeplugin
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "update"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]

---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-nodeplugin
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-plugin-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: ClusterRole
  name: cephfs-csi-nodeplugin
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-csi-cephfs-provisioner-sa
  namespace: ` + operatorNamespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-external-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments/status"]
    verbs: ["patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list", "watch", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents/status"]
    verbs: ["update"]
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["create", "list", "watch", "delete", "get", "update"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots/status"]
    verbs: ["update"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-provisioner-role
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: ClusterRole
  name: cephfs-external-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: ` + operatorNamespace + `
  name: cephfs-external-provisioner-cfg
rules:
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cephfs-csi-provisioner-role-cfg
  namespace: ` + operatorNamespace + `
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + operatorNamespace + `
roleRef:
  kind: Role
  name: cephfs-external-provisioner-cfg
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: 00-rook-privileged
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: 'runtime/default'
    seccomp.security.alpha.kubernetes.io/defaultProfileName:  'runtime/default'
spec:
  privileged: true
  allowedCapabilities:
    # required by CSI
    - SYS_ADMIN
  # fsGroup - the flexVolume agent has fsGroup capabilities and could potentially be any group
  fsGroup:
    rule: RunAsAny
  # runAsUser, supplementalGroups - Rook needs to run some pods as root
  # Ceph pods could be run as the Ceph user, but that user isn't always known ahead of time
  runAsUser:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  # seLinux - seLinux context is unknown ahead of time; set if this is well-known
  seLinux:
    rule: RunAsAny
  volumes:
    # recommended minimum set
    - configMap
    - downwardAPI
    - emptyDir
    - persistentVolumeClaim
    - secret
    - projected
    # required for Rook
    - hostPath
    - flexVolume
  # allowedHostPaths can be set to Rook's known host volume mount points when they are fully-known
  # allowedHostPaths:
  #   - /run/udev      # for OSD prep
  #   - /dev           # for OSD prep
  #   - /var/lib/rook  # or whatever the dataDirHostPath value is set to
  # Ceph requires host IPC for setting up encrypted devices
  hostIPC: true
  # Ceph OSDs need to share the same PID namespace
  hostPID: true
  # hostNetwork can be set to 'false' if host networking isn't used
  hostNetwork: true
  hostPorts:
    # Ceph messenger protocol v1
    - min: 6789
      max: 6790 # <- support old default port
    # Ceph messenger protocol v2
    - min: 3300
      max: 3300
    # Ceph RADOS ports for OSDs, MDSes
    - min: 6800
      max: 7300
    # # Ceph dashboard port HTTP (not recommended)
    # - min: 7000
    #   max: 7000
    # Ceph dashboard port HTTPS
    - min: 8443
      max: 8443
    # Ceph mgr Prometheus Metrics
    - min: 9283
      max: 9283
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: 'psp:rook'
rules:
  - apiGroups:
      - policy
    resources:
      - podsecuritypolicies
    resourceNames:
      - 00-rook-privileged
    verbs:
      - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-ceph-system-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-ceph-system
    namespace: ` + operatorNamespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-rbd-provisioner-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-provisioner-sa
    namespace: ` + operatorNamespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-rbd-plugin-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-rbd-plugin-sa
    namespace: ` + operatorNamespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-cephfs-provisioner-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-provisioner-sa
    namespace: ` + operatorNamespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: rook-csi-cephfs-plugin-sa-psp
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: 'psp:rook'
subjects:
  - kind: ServiceAccount
    name: rook-csi-cephfs-plugin-sa
    namespace: ` + operatorNamespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-admission-controller
  namespace: ` + operatorNamespace + `
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-admission-controller-role
rules:
  - apiGroups: ["ceph.rook.io"]
    resources: ["*"]
    verbs: ["get", "watch", "list"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-admission-controller-rolebinding
subjects:
  - kind: ServiceAccount
    name: rook-ceph-admission-controller
    apiGroup: ""
    namespace: ` + operatorNamespace + `
roleRef:
  kind: ClusterRole
  name: rook-ceph-admission-controller-role
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-ceph-operator
  namespace: ` + operatorNamespace + `
  labels:
    operator: rook
    storage-backend: ceph
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rook-ceph-operator
  template:
    metadata:
      labels:
        app: rook-ceph-operator
    spec:
      serviceAccountName: rook-ceph-system
      containers:
      - name: rook-ceph-operator
        image: rook/ceph:` + m.imageTag + `
        args: ["ceph", "operator"]
        env:
        - name: ROOK_LOG_LEVEL
          value: INFO
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: ROOK_ENABLE_FLEX_DRIVER
          value: "true"
        - name: ROOK_CURRENT_NAMESPACE_ONLY
          value: "false"` + openshiftEnv + `
        - name: ROOK_ENABLE_DISCOVERY_DAEMON
          value: "true"
        volumeMounts:
        - mountPath: /var/lib/rook
          name: rook-config
        - mountPath: /etc/ceph
          name: default-config-dir
      volumes:
      - name: rook-config
        emptyDir: {}
      - name: default-config-dir
        emptyDir: {}
---
kind: ConfigMap
apiVersion: v1
metadata:
  name: rook-ceph-operator-config
  namespace: ` + operatorNamespace + `
data:
  ROOK_CSI_ENABLE_CEPHFS: "true"
  ROOK_CSI_ENABLE_RBD: "true"
  ROOK_CSI_ENABLE_GRPC_METRICS: "true"
  ROOK_OBC_WATCH_OPERATOR_NAMESPACE: "true"
  CSI_LOG_LEVEL: "5"
`
	return operatorManifest
}

// GetClusterRoles returns rook-cluster manifest
func (m *CephManifestsV1_5) GetClusterRoles(namespace, operatorNamespace string) string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: [ "get", "list", "watch", "create", "update", "delete" ]
- apiGroups: ["ceph.rook.io"]
  resources: ["cephclusters", "cephclusters/finalizers"]
  verbs: [ "get", "list", "create", "update", "delete" ]
---
# Aspects of ceph-mgr that operate within the cluster's namespace
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
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
  - "*"
  verbs:
  - "*"
---
# Allow the ceph osd to access cluster-wide resources necessary for determining their topology location
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd-` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access cluster-wide resources necessary for the mgr modules
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-cluster-` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-cluster
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
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
# Allow the operator to create resources in this cluster's namespace
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + operatorNamespace + `
---
# Allow the osd pods in this namespace to work with configmaps
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-osd
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-osd
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access the cluster-specific resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-mgr
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
# Allow the ceph mgr to access the rook system resources necessary for the mgr modules
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-mgr-system ` + namespace + `
  namespace: ` + operatorNamespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-mgr-system
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-default-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: default
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-osd-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-osd
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-mgr-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-mgr
  namespace: ` + namespace + `
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rook-ceph-cmd-reporter-psp
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: psp:rook
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
`
}

// GetRookCluster returns rook-cluster manifest
func (m *CephManifestsV1_5) GetRookCluster(settings *clusterSettings) string {
	store := "# storeType not specified; Rook will use default store types"
	if settings.StoreType != "" {
		store = `storeType: "` + settings.StoreType + `"`
	}

	crushRoot := "# crushRoot not specified; Rook will use `default`"
	if settings.Mons == 1 {
		crushRoot = `crushRoot: "custom-root"`
	}

	if settings.UsePVCs {
		return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  # set the name to something different from the namespace
  name: ` + settings.ClusterName + `
  namespace: ` + settings.Namespace + `
spec:
  dataDirHostPath: ` + settings.DataDirHostPath + `
  mon:
    count: ` + strconv.Itoa(settings.Mons) + `
    allowMultiplePerNode: true
    volumeClaimTemplate:
      spec:
        storageClassName: ` + settings.StorageClassName + `
        resources:
          requests:
            storage: 5Gi
  cephVersion:
    image: ` + settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(settings.CephVersion.AllowUnsupported) + `
  skipUpgradeChecks: false
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  dashboard:
    enabled: true
  network:
    hostNetwork: false
  crashCollector:
    disable: false
  storage:
    config:
      ` + crushRoot + `
    storageClassDeviceSets:
    - name: set1
      count: 1
      portable: false
      tuneDeviceClass: true
      encrypted: false
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          storageClassName: ` + settings.StorageClassName + `
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
  disruptionManagement:
    managePodBudgets: false
    osdMaintenanceTimeout: 30
    pgHealthCheckTimeout: 0
    manageMachineDisruptionBudgets: false
    machineDisruptionBudgetNamespace: openshift-machine-api`
	}

	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.ClusterName + `
  namespace: ` + settings.Namespace + `
spec:
  cephVersion:
    image: ` + settings.CephVersion.Image + `
    allowUnsupported: ` + strconv.FormatBool(settings.CephVersion.AllowUnsupported) + `
  dataDirHostPath: ` + settings.DataDirHostPath + `
  network:
    hostNetwork: false
  mon:
    count: ` + strconv.Itoa(settings.Mons) + `
    allowMultiplePerNode: true
  dashboard:
    enabled: true
  skipUpgradeChecks: true
  metadataDevice:
  storage:
    useAllNodes: ` + strconv.FormatBool(!settings.skipOSDCreation) + `
    useAllDevices: ` + strconv.FormatBool(!settings.skipOSDCreation) + `
    deviceFilter:  ''
    config:
      ` + store + `
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
  mgr:
    modules:
    - name: pg_autoscaler
      enabled: true
    - name: rook
      enabled: true
  healthCheck:
    daemonHealth:
      mon:
        interval: 10s
        timeout: 15s
      osd:
        interval: 10s
      status:
        interval: 5s`
}

// GetRookToolBox returns rook-toolbox manifest
func (m *CephManifestsV1_5) GetRookToolBox(namespace string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: rook-ceph-tools
  namespace: ` + namespace + `
spec:
  dnsPolicy: ClusterFirstWithHostNet
  containers:
  - name: rook-ceph-tools
    image: rook/ceph:` + m.imageTag + `
    imagePullPolicy: IfNotPresent
    command: ["/tini"]
    args: ["-g", "--", "/usr/local/bin/toolbox.sh"]
    env:
      - name: ROOK_CEPH_USERNAME
        valueFrom:
          secretKeyRef:
            name: rook-ceph-mon
            key: ceph-username
      - name: ROOK_CEPH_SECRET
        valueFrom:
          secretKeyRef:
            name: rook-ceph-mon
            key: ceph-secret
    securityContext:
      privileged: true
    volumeMounts:
      - mountPath: /dev
        name: dev
      - mountPath: /sys/bus
        name: sysbus
      - mountPath: /lib/modules
        name: libmodules
      - name: mon-endpoint-volume
        mountPath: /etc/rook
  hostNetwork: false
  volumes:
    - name: dev
      hostPath:
        path: /dev
    - name: sysbus
      hostPath:
        path: /sys/bus
    - name: libmodules
      hostPath:
        path: /lib/modules
    - name: mon-endpoint-volume
      configMap:
        name: rook-ceph-mon-endpoints
        items:
        - key: data
          path: mon-endpoints`
}

func (m *CephManifestsV1_5) GetBlockSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshotClass
metadata:
  name: ` + snapshotClassName + `
driver: ` + operatorNamespace + `.rbd.csi.ceph.com
deletionPolicy: ` + reclaimPolicy + `
parameters:
  clusterID: ` + namespace + `
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: ` + namespace + `
`
}

func (m *CephManifestsV1_5) GetFileStorageSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string {
	// Create a CSI driver snapshotclass
	return `
apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshotClass
metadata:
  name: ` + snapshotClassName + `
driver: ` + operatorNamespace + `.cephfs.csi.ceph.com
deletionPolicy: ` + reclaimPolicy + `
parameters:
  clusterID: ` + namespace + `
  csi.storage.k8s.io/snapshotter-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/snapshotter-secret-namespace: ` + namespace + `
`
}

func (m *CephManifestsV1_5) GetPVCRestore(claimName, snapshotName, namespace, storageClassName, accessModes, size string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  namespace: ` + namespace + `
spec:
  storageClassName: ` + storageClassName + `
  dataSource:
    name: ` + snapshotName + `
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: ` + size
}

func (m *CephManifestsV1_5) GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, accessModes, size string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + cloneClaimName + `
  namespace: ` + namespace + `
spec:
  storageClassName: ` + storageClassName + `
  dataSource:
    name: ` + parentClaimName + `
    kind: PersistentVolumeClaim
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: ` + size
}

func (m *CephManifestsV1_5) GetSnapshot(snapshotName, claimName, snapshotClassName, namespace string) string {
	return `apiVersion: snapshot.storage.k8s.io/v1beta1
kind: VolumeSnapshot
metadata:
  name: ` + snapshotName + `
  namespace: ` + namespace + `
spec:
  volumeSnapshotClassName: ` + snapshotClassName + `
  source:
    persistentVolumeClaimName: ` + claimName
}

func (m *CephManifestsV1_5) GetPod(podName, claimName, namespace, mountPath string, readOnly bool) string {
	return `
apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podName + `
    image: busybox
    command: ["/bin/sleep", "infinity"]
    imagePullPolicy: IfNotPresent
    volumeMounts:
    - mountPath: ` + mountPath + `
      name: csivol
  volumes:
  - name: csivol
    persistentVolumeClaim:
       claimName: ` + claimName + `
       readOnly: ` + strconv.FormatBool(readOnly) + `
  restartPolicy: Never
`
}

func (m *CephManifestsV1_5) GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize + `
    targetSizeRatio: .5
    requireSafeReplicaSize: false
  compressionMode: aggressive
  mirroring:
    enabled: true
    mode: image
  quotas:
    maxBytes: 10737418240
    maxObjects: 1000000
  statusCheck:
    mirror:
      disabled: false
      interval: 10s`
}

func (m *CephManifestsV1_5) GetBlockStorageClassDef(csi bool, poolName, storageClassName, reclaimPolicy, namespace, operatorNamespace string) string {
	// Create a CSI driver storage class
	if csi {
		return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + operatorNamespace + `.rbd.csi.ceph.com
reclaimPolicy: ` + reclaimPolicy + `
parameters:
  pool: ` + poolName + `
  clusterID: ` + namespace + `
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
  imageFeatures: layering
  csi.storage.k8s.io/fstype: ext4
`
	}
	// Create a FLEX driver storage class
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ceph.rook.io/block
allowVolumeExpansion: true
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    blockPool: ` + poolName + `
    clusterNamespace: ` + namespace
}

func (m *CephManifestsV1_5) GetFileStorageClassDef(fsName, storageClassName, operatorNamespace, namespace string) string {
	// Create a CSI driver storage class
	csiCephFSNodeSecret := "rook-csi-cephfs-node"               //nolint:gosec // We safely suppress gosec in tests file
	csiCephFSProvisionerSecret := "rook-csi-cephfs-provisioner" //nolint:gosec // We safely suppress gosec in tests file
	return `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ` + storageClassName + `
provisioner: ` + operatorNamespace + `.cephfs.csi.ceph.com
parameters:
  clusterID: ` + namespace + `
  fsName: ` + fsName + `
  pool: ` + fsName + `-data0
  csi.storage.k8s.io/provisioner-secret-name: ` + csiCephFSProvisionerSecret + `
  csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
  csi.storage.k8s.io/node-stage-secret-name: ` + csiCephFSNodeSecret + `
  csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
`
}

func (m *CephManifestsV1_5) GetPVC(claimName, namespace, storageClassName, accessModes, size string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  namespace: ` + namespace + `
spec:
  storageClassName: ` + storageClassName + `
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: ` + size
}

// GetFilesystem returns the manifest to create a Rook filesystem resource with the given config.
func (m *CephManifestsV1_5) GetFilesystem(namespace, name string, activeCount int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
  dataPools:
  - replicated:
      size: 1
      requireSafeReplicaSize: false
    compressionMode: none
  metadataServer:
    activeCount: ` + strconv.Itoa(activeCount) + `
    activeStandby: true`
}

// GetFilesystem returns the manifest to create a Rook Ceph NFS resource with the given config.
func (m *CephManifestsV1_5) GetNFS(namespace, name, pool string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  rados:
    pool: ` + pool + `
    namespace: nfs-ns
  server:
    active: ` + strconv.Itoa(count)
}

func (m *CephManifestsV1_5) GetObjectStore(namespace, name string, replicaCount, port int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  metadataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
    compressionMode: passive
  dataPool:
    replicated:
      size: 1
      requireSafeReplicaSize: false
  gateway:
    type: s3
    sslCertificateRef:
    port: ` + strconv.Itoa(port) + `
    instances: ` + strconv.Itoa(replicaCount) + `
  healthCheck:
    bucket:
      disabled: false
      interval: 10s
`
}

func (m *CephManifestsV1_5) GetObjectStoreUser(namespace, name string, displayName string, store string) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  displayName: ` + displayName + `
  store: ` + store
}

//GetBucketStorageClass returns the manifest to create object bucket
func (m *CephManifestsV1_5) GetBucketStorageClass(storeNameSpace string, storeName string, storageClassName string, reclaimPolicy string, region string) string {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ` + storeNameSpace + `.ceph.rook.io/bucket
reclaimPolicy: ` + reclaimPolicy + `
parameters:
    objectStoreName: ` + storeName + `
    objectStoreNamespace: ` + storeNameSpace + `
    region: ` + region
}

//GetObc returns the manifest to create object bucket claim
func (m *CephManifestsV1_5) GetObc(claimName string, storageClassName string, objectBucketName string, maxObject string, varBucketName bool) string {
	bucketParameter := "generateBucketName"
	if varBucketName {
		bucketParameter = "bucketName"
	}
	return `apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ` + claimName + `
spec:
  ` + bucketParameter + `: ` + objectBucketName + `
  storageClassName: ` + storageClassName + `
  additionalConfig:
    maxObjects: "` + maxObject + `"`
}

func (m *CephManifestsV1_5) GetClient(namespace string, claimName string, caps map[string]string) string {
	clientCaps := []string{}
	for name, cap := range caps {
		str := name + ": " + cap
		clientCaps = append(clientCaps, str)
	}
	return `apiVersion: ceph.rook.io/v1
kind: CephClient
metadata:
  name: ` + claimName + `
  namespace: ` + namespace + `
spec:
  caps:
    ` + strings.Join(clientCaps, "\n    ")
}

func (m *CephManifestsV1_5) GetClusterExternalRoles(namespace, firstClusterNamespace string) string {
	return `apiVersion: v1
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cluster-mgmt
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-cluster-mgmt
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + firstClusterNamespace + `
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rook-ceph-cmd-reporter
subjects:
- kind: ServiceAccount
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: rook-ceph-cmd-reporter
  namespace: ` + namespace + `
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
  - delete`
}

func (m *CephManifestsV1_5) GetRookExternalCluster(settings *clusterExternalSettings) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ` + settings.Namespace + `
  namespace: ` + settings.Namespace + `
spec:
  external:
    enable: true
  dataDirHostPath: ` + settings.DataDirHostPath + ``
}

// GetRBDMirror returns the manifest to create a Rook Ceph RBD Mirror resource with the given config.
func (m *CephManifestsV1_5) GetRBDMirror(namespace, name string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  count: ` + strconv.Itoa(count)
}
