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
                - "--timeout=60s"
                - "--retry-interval-start=500ms"
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
                - "--v=5"
                - "--timeout=60s"
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
              image: {{ .CSIPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--type=rbd"
                - "--drivername={{ .DriverNamePrefix }}rbd.csi.ceph.com"
                - "--containerized=true"
                - "--metadatastorage=k8s_configmap"
                - "--pidlimit=-1"
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
                - name: ceph-csi-config
                  mountPath: /etc/ceph-csi-config/
                - name: keys-tmp-dir
                  mountPath: /tmp/csi/keys
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
                path: /var/lib/kubelet/plugins/{{ .DriverNamePrefix }}rbd.csi.ceph.com
                type: DirectoryOrCreate
            - name: ceph-csi-config
              configMap:
                name: rook-ceph-csi-config
                items:
                  - key: csi-cluster-config-json
                    path: config.json
            - name: keys-tmp-dir
              emptyDir: {
                medium: "Memory"
              }
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
                - "--kubelet-registration-path=/var/lib/kubelet/plugins/{{ .DriverNamePrefix }}rbd.csi.ceph.com/csi.sock"
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
              image: {{ .CSIPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--type=rbd"
                - "--drivername={{ .DriverNamePrefix }}rbd.csi.ceph.com"
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
                  mountPath: /var/lib/kubelet/plugins
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
                - name: ceph-csi-config
                  mountPath: /etc/ceph-csi-config/
                - name: keys-tmp-dir
                  mountPath: /tmp/csi/keys
          volumes:
            - name: plugin-dir
              hostPath:
                path: /var/lib/kubelet/plugins/{{ .DriverNamePrefix }}rbd.csi.ceph.com
                type: DirectoryOrCreate
            - name: plugin-mount-dir
              hostPath:
                path: /var/lib/kubelet/plugins
                type: Directory
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
            - name: ceph-csi-config
              configMap:
                name: rook-ceph-csi-config
                items:
                  - key: csi-cluster-config-json
                    path: config.json
            - name: keys-tmp-dir
              emptyDir: {
                medium: "Memory"
              }
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
                - "--timeout=60s"
                - "--retry-interval-start=500ms"
              env:
                - name: ADDRESS
                  value: unix:///csi/csi-provisioner.sock
              imagePullPolicy: "IfNotPresent"
              volumeMounts:
                - name: socket-dir
                  mountPath: /csi
            - name: csi-attacher
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
            - name: csi-cephfsplugin
              securityContext:
                privileged: true
                capabilities:
                  add: ["SYS_ADMIN"]
              image: {{ .CSIPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--type=cephfs"
                - "--drivername={{ .DriverNamePrefix }}cephfs.csi.ceph.com"
                - "--metadatastorage=k8s_configmap"
                - "--pidlimit=-1"
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
                - name: ceph-csi-config
                  mountPath: /etc/ceph-csi-config/
                - name: keys-tmp-dir
                  mountPath: /tmp/csi/keys
          volumes:
            - name: socket-dir
              hostPath:
                path: /var/lib/kubelet/plugins/{{ .DriverNamePrefix }}cephfs.csi.ceph.com
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
            - name: ceph-csi-config
              configMap:
                name: rook-ceph-csi-config
                items:
                  - key: csi-cluster-config-json
                    path: config.json
            - name: keys-tmp-dir
              emptyDir: {
                medium: "Memory"
              }
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
                - "--kubelet-registration-path=/var/lib/kubelet/plugins/{{ .DriverNamePrefix }}cephfs.csi.ceph.com/csi.sock"
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
              image: {{ .CSIPluginImage }}
              args :
                - "--nodeid=$(NODE_ID)"
                - "--endpoint=$(CSI_ENDPOINT)"
                - "--v=5"
                - "--type=cephfs"
                - "--drivername={{ .DriverNamePrefix }}cephfs.csi.ceph.com"
                - "--metadatastorage=k8s_configmap"
                - "--mountcachedir=/mount-cache-dir"
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
                  mountPath: /var/lib/kubelet/plugins
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
                - name: mount-cache-dir
                  mountPath: /mount-cache-dir
                - name: ceph-csi-config
                  mountPath: /etc/ceph-csi-config/
                - name: keys-tmp-dir
                  mountPath: /tmp/csi/keys
          volumes:
            - name: plugin-dir
              hostPath:
                path: /var/lib/kubelet/plugins/{{ .DriverNamePrefix }}cephfs.csi.ceph.com/
                type: DirectoryOrCreate
            - name: csi-plugins-dir
              hostPath:
                path: /var/lib/kubelet/plugins
                type: Directory
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
            - name: mount-cache-dir
              emptyDir: {}
            - name: ceph-csi-config
              configMap:
                name: rook-ceph-csi-config
                items:
                  - key: csi-cluster-config-json
                    path: config.json
            - name: keys-tmp-dir
              emptyDir: {
                medium: "Memory"
              }

`
)
