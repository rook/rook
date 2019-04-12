/*
Copyright 2019 The Rook Authors. All rights reserved.

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

const (
	rbdProvisionerTemplate = `
    kind: StatefulSet
    apiVersion: apps/v1
    metadata:
      name: csi-rbdplugin-provisioner
      namespace: {{ .Namespace }}
    spec:
      serviceName: "csi-rbdplugin-provisioner"
      replicas: 1
      selector:
        matchLabels:
         app: csi-rbdplugin-provisioner
      template:
        metadata:
          labels:
            app: csi-rbdplugin-provisioner
        spec:
          serviceAccount: rook-csi-rbd-provisioner-sa
          containers:
            - name: csi-provisioner
              image: {{ .ProvisionerImage }}
              args:
                - "--csi-address=$(ADDRESS)"
                - "--v=5"
              env:
                - name: ADDRESS
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
            - name: csi-rbdplugin-attacher
              image: {{ .AttacherImage }}
              args:
                - "--v=5"
                - "--csi-address=$(ADDRESS)"
              env:
                - name: ADDRESS
                  value: /csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi

            - name: csi-snapshotter
              image:  {{ .SnapshotterImage }}
              args:
                - "--csi-address=$(ADDRESS)"
                - "--connection-timeout=15s"
                - "--v=5"
              env:
                - name: ADDRESS
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: Always
              securityContext:
                privileged: true
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
            - name: csi-rbdplugin
              securityContext:
                privileged: true
                capabilities:
                  add: ["SYS_ADMIN"]
              image: {{ .RBDPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--drivername=rbd.csi.ceph.com"
                - "--containerized=true"
                - "--metadatastorage=k8s_configmap"
              env:
                - name: HOST_ROOTFS
                  value: "/rootfs"
                - name: NODE_ID
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.namespace
                - name: CSI_ENDPOINT
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
                - mountPath: /dev
                  name: host-dev
                - mountPath: /rootfs
                  name: host-rootfs
                - mountPath: /sys
                  name: host-sys
                - mountPath: /lib/modules
                  name: lib-modules
                  readOnly: true
          volumes:
            - name: host-dev
              hostPath:
                path: /dev
            - name: host-rootfs
              hostPath:
                path: /
            - name: host-sys
              hostPath:
                path: /sys
            - name: lib-modules
              hostPath:
                path: /lib/modules
            - name: socket-dir
              hostPath:
                path: /var/lib/kubelet/plugins/rbd.csi.ceph.com
                type: DirectoryOrCreate
`
	rbdPluginTemplate = `
    kind: DaemonSet
    apiVersion: apps/v1
    metadata:
      name: csi-rbdplugin
      namespace: {{ .Namespace }}
    spec:
      selector:
        matchLabels:
          app: csi-rbdplugin
      template:
        metadata:
          labels:
            app: csi-rbdplugin
        spec:
          serviceAccount: rook-csi-rbd-plugin-sa
          hostNetwork: true
          hostPID: true
          # to use e.g. Rook orchestrated cluster, and mons' FQDN is
          # resolved through k8s service, set dns policy to cluster first
          dnsPolicy: ClusterFirstWithHostNet
          containers:
            - name: driver-registrar
              image: {{ .RegistrarImage }}
              args:
                - "--v=5"
                - "--csi-address=/csi/csi.sock"
                - "--kubelet-registration-path=/var/lib/kubelet/plugins/rbd.csi.ceph.com/csi.sock"
              lifecycle:
                preStop:
                  exec:
                      command: ["/bin/sh", "-c", "rm -rf /registration/csi-rbdplugin /registration/csi-rbdplugin-reg.sock"]
              env:
                - name: KUBE_NODE_NAME
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
              volumeMounts:
                - name: plugin-dir
                  mountPath: /csi
                - name: registration-dir
                  mountPath: /registration
            - name: csi-rbdplugin
              securityContext:
                privileged: true
                capabilities:
                  add: ["SYS_ADMIN"]
                allowPrivilegeEscalation: true
              image: {{ .RBDPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--drivername=rbd.csi.ceph.com"
                - "--containerized=true"
                - "--metadatastorage=k8s_configmap"
              env:
                - name: HOST_ROOTFS
                  value: "/rootfs"
                - name: NODE_ID
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.namespace
                - name: CSI_ENDPOINT
                  value: unix:///csi/csi.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: plugin-dir
                  mountPath: /csi
                - name: pods-mount-dir
                  mountPath: /var/lib/kubelet/pods
                  mountPropagation: "Bidirectional"
                - name: plugin-mount-dir
                  mountPath: /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/
                  mountPropagation: "Bidirectional"
                - mountPath: /dev
                  name: host-dev
                - mountPath: /rootfs
                  name: host-rootfs
                - mountPath: /sys
                  name: host-sys
                - mountPath: /lib/modules
                  name: lib-modules
                  readOnly: true
          volumes:
            - name: plugin-dir
              hostPath:
                path: /var/lib/kubelet/plugins/rbd.csi.ceph.com
                type: DirectoryOrCreate
            - name: plugin-mount-dir
              hostPath:
                path: /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/
                type: DirectoryOrCreate
            - name: registration-dir
              hostPath:
                path: /var/lib/kubelet/plugins_registry/
                type: Directory
            - name: pods-mount-dir
              hostPath:
                path: /var/lib/kubelet/pods
                type: Directory
            - name: host-dev
              hostPath:
                path: /dev
            - name: host-rootfs
              hostPath:
                path: /
            - name: host-sys
              hostPath:
                path: /sys
            - name: lib-modules
              hostPath:
                path: /lib/modules
    `
	cephfsProvisionerTemplate = `
    kind: StatefulSet
    apiVersion: apps/v1
    metadata:
      name: csi-cephfsplugin-provisioner
      namespace: {{ .Namespace }}
    spec:
      serviceName: "csi-cephfsplugin-provisioner"
      replicas: 1
      selector:
        matchLabels:
         app: csi-cephfsplugin-provisioner
      template:
        metadata:
          labels:
            app: csi-cephfsplugin-provisioner
        spec:
          serviceAccount: rook-csi-cephfs-provisioner-sa
          containers:
            - name: csi-provisioner
              image: {{ .ProvisionerImage }}
              args:
                - "--csi-address=$(ADDRESS)"
                - "--v=5"
              env:
                - name: ADDRESS
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
            - name: csi-cephfsplugin
              securityContext:
                privileged: true
                capabilities:
                  add: ["SYS_ADMIN"]
              image: {{ .CephFSPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--drivername=cephfs.csi.ceph.com"
                - "--metadatastorage=k8s_configmap"
              env:
                - name: NODE_ID
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.namespace
                - name: CSI_ENDPOINT
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
                - name: host-sys
                  mountPath: /sys
                - name: lib-modules
                  mountPath: /lib/modules
                  readOnly: true
                - name: host-dev
                  mountPath: /dev
          volumes:
            - name: socket-dir
              hostPath:
                path: /var/lib/kubelet/plugins/cephfs.csi.ceph.com
                type: DirectoryOrCreate
            - name: host-sys
              hostPath:
                path: /sys
            - name: lib-modules
              hostPath:
                path: /lib/modules
            - name: host-dev
              hostPath:
                path: /dev
`
	cephfsPluginTemplate = `
    kind: DaemonSet
    apiVersion: apps/v1
    metadata:
      name: csi-cephfsplugin
      namespace: {{ .Namespace }}
    spec:
      selector:
        matchLabels:
          app: csi-cephfsplugin
      template:
        metadata:
          labels:
            app: csi-cephfsplugin
        spec:
          serviceAccount: rook-csi-cephfs-plugin-sa
          hostNetwork: true
          # to use e.g. Rook orchestrated cluster, and mons' FQDN is
          # resolved through k8s service, set dns policy to cluster first
          dnsPolicy: ClusterFirstWithHostNet
          containers:
            - name: driver-registrar
              image: {{ .RegistrarImage }}
              args:
                - "--v=5"
                - "--csi-address=/csi/csi.sock"
                - "--kubelet-registration-path=/var/lib/kubelet/plugins/cephfs.csi.ceph.com/csi.sock"
              lifecycle:
                preStop:
                  exec:
                      command: ["/bin/sh", "-c", "rm -rf /registration/csi-cephfsplugin /registration/csi-cephfsplugin-reg.sock"]
              env:
                - name: KUBE_NODE_NAME
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
              volumeMounts:
                - name: plugin-dir
                  mountPath: /csi
                - name: registration-dir
                  mountPath: /registration
            - name: csi-cephfsplugin
              securityContext:
                privileged: true
                capabilities:
                  add: ["SYS_ADMIN"]
                allowPrivilegeEscalation: true
              image: {{ .CephFSPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--drivername=cephfs.csi.ceph.com"
                - "--metadatastorage=k8s_configmap"
              env:
                - name: NODE_ID
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      fieldPath: metadata.namespace
                - name: CSI_ENDPOINT
                  value: unix:///csi/csi.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: plugin-dir
                  mountPath: /csi
                - name: csi-plugins-dir
                  mountPath: /var/lib/kubelet/plugins/kubernetes.io/csi
                  mountPropagation: "Bidirectional"
                - name: pods-mount-dir
                  mountPath: /var/lib/kubelet/pods
                  mountPropagation: "Bidirectional"
                - name: host-sys
                  mountPath: /sys
                - name: lib-modules
                  mountPath: /lib/modules
                  readOnly: true
                - name: host-dev
                  mountPath: /dev
          volumes:
            - name: plugin-dir
              hostPath:
                path: /var/lib/kubelet/plugins/cephfs.csi.ceph.com/
                type: DirectoryOrCreate
            - name: csi-plugins-dir
              hostPath:
                path: /var/lib/kubelet/plugins/kubernetes.io/csi
                type: DirectoryOrCreate
            - name: registration-dir
              hostPath:
                path: /var/lib/kubelet/plugins_registry/
                type: Directory
            - name: pods-mount-dir
              hostPath:
                path: /var/lib/kubelet/pods
                type: Directory
            - name: host-sys
              hostPath:
                path: /sys
            - name: lib-modules
              hostPath:
                path: /lib/modules
            - name: host-dev
              hostPath:
                path: /dev
`
)
