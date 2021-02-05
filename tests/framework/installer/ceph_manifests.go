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
	"strconv"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
)

const (
	// Version1_4 rook version 1.4
	Version1_4 = "v1.4.7"
)

type CephManifests interface {
	GetRookCRDs(v1ExtensionsSupported bool) string
	GetRookOperator(namespace string) string
	GetClusterRoles(namespace, operatorNamespace string) string
	GetClusterExternalRoles(namespace, operatorNamespace string) string
	GetRookCluster(settings *clusterSettings) string
	GetRookExternalCluster(settings *clusterExternalSettings) string
	GetRookToolBox(namespace string) string
	GetBlockPoolDef(poolName, namespace, replicaSize string) string
	GetBlockStorageClassDef(csi bool, poolName, storageClassName, reclaimPolicy, namespace, operatorNamespace string) string
	GetFileStorageClassDef(fsName, storageClassName, operatorNamespace, namespace string) string
	GetPVC(claimName, namespace, storageClassName, accessModes, size string) string
	GetBlockSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string
	GetFileStorageSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string
	GetPVCRestore(claimName, snapshotName, namespace, storageClassName, accessModes, size string) string
	GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, accessModes, size string) string
	GetSnapshot(snapshotName, claimName, snapshotClassName, namespace string) string
	GetPod(podName, claimName, namespace, mountPoint string, readOnly bool) string
	GetFilesystem(namespace, name string, activeCount int) string
	GetNFS(namespace, name, pool string, daemonCount int) string
	GetRBDMirror(namespace, name string, daemonCount int) string
	GetObjectStore(namespace, name string, replicaCount, port int) string
	GetObjectStoreUser(namespace, name, displayName, store string) string
	GetBucketStorageClass(namespace, storeName, storageClassName, reclaimPolicy, region string) string
	GetObc(obcName, storageClassName, bucketName string, maxObject string, createBucket bool) string
	GetClient(namespace, name string, caps map[string]string) string
}

type clusterSettings struct {
	ClusterName      string
	Namespace        string
	StoreType        string
	DataDirHostPath  string
	Mons             int
	RBDMirrorWorkers int
	UsePVCs          bool
	StorageClassName string
	skipOSDCreation  bool
	CephVersion      cephv1.CephVersionSpec
	useCrashPruner   bool
}

// clusterExternalSettings represents the settings of an external cluster
type clusterExternalSettings struct {
	Namespace       string
	DataDirHostPath string
}

// CephManifestsMaster wraps rook yaml definitions
type CephManifestsMaster struct {
	imageTag string
}

// NewCephManifests gets the manifest type depending on the Rook version desired
func NewCephManifests(version string) CephManifests {
	switch version {
	case VersionMaster:
		return &CephManifestsMaster{imageTag: VersionMaster}
	case Version1_4:
		return &CephManifestsV1_4{imageTag: Version1_4}
	}
	panic(fmt.Errorf("unrecognized ceph manifest version: %s", version))
}

func (m *CephManifestsMaster) GetRookCRDs(v1ExtensionsSupported bool) string {
	if v1ExtensionsSupported {
		return `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephclusters.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephCluster
    plural: cephclusters
    singular: cephcluster
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                cephVersion:
                  type: object
                  properties:
                    allowUnsupported:
                      type: boolean
                    image:
                      type: string
                dashboard:
                  type: object
                  properties:
                    enabled:
                      type: boolean
                    urlPrefix:
                      type: string
                    port:
                      type: integer
                      minimum: 0
                      maximum: 65535
                    ssl:
                      type: boolean
                dataDirHostPath:
                  pattern: ^/(\S+)
                  type: string
                disruptionManagement:
                  type: object
                  properties:
                    machineDisruptionBudgetNamespace:
                      type: string
                    managePodBudgets:
                      type: boolean
                    osdMaintenanceTimeout:
                      type: integer
                    pgHealthCheckTimeout:
                      type: integer
                    manageMachineDisruptionBudgets:
                      type: boolean
                skipUpgradeChecks:
                  type: boolean
                continueUpgradeAfterChecksEvenIfNotHealthy:
                  type: boolean
                mon:
                  type: object
                  properties:
                    allowMultiplePerNode:
                      type: boolean
                    count:
                      maximum: 9
                      minimum: 0
                      type: integer
                    volumeClaimTemplate:
                      type: object
                      x-kubernetes-preserve-unknown-fields: true
                    stretchCluster:
                      type: object
                      nullable: true
                      properties:
                        failureDomainLabel:
                          type: string
                        subFailureDomain:
                          type: string
                        zones:
                          type: array
                          items:
                            type: object
                            properties:
                              name:
                                type: string
                              arbiter:
                                type: boolean
                              volumeClaimTemplate:
                                type: object
                                x-kubernetes-preserve-unknown-fields: true
                mgr:
                  type: object
                  properties:
                    modules:
                      type: array
                      items:
                        type: object
                        properties:
                          name:
                            type: string
                          enabled:
                            type: boolean
                crashCollector:
                  type: object
                  properties:
                    disable:
                      type: boolean
                    daysToRetain:
                      type: integer
                network:
                  type: object
                  nullable: true
                  properties:
                    hostNetwork:
                      type: boolean
                    provider:
                      type: string
                    selectors:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    ipFamily:
                      type: string
                storage:
                  type: object
                  properties:
                    disruptionManagement:
                      type: object
                      nullable: true
                      properties:
                        machineDisruptionBudgetNamespace:
                          type: string
                        managePodBudgets:
                          type: boolean
                        osdMaintenanceTimeout:
                          type: integer
                        pgHealthCheckTimeout:
                          type: integer
                        manageMachineDisruptionBudgets:
                          type: boolean
                    useAllNodes:
                      type: boolean
                    nodes:
                      type: array
                      nullable: true
                      items:
                        type: object
                        properties:
                          name:
                            type: string
                          config:
                            type: object
                            nullable: true
                            properties:
                              metadataDevice:
                                type: string
                              storeType:
                                type: string
                                pattern: ^(bluestore)$
                              databaseSizeMB:
                                type: string
                              walSizeMB:
                                type: string
                              journalSizeMB:
                                type: string
                              osdsPerDevice:
                                type: string
                              encryptedDevice:
                                type: string
                                pattern: ^(true|false)$
                          useAllDevices:
                            type: boolean
                          deviceFilter:
                            type: string
                            nullable: true
                          devicePathFilter:
                            type: string
                          devices:
                            type: array
                            items:
                              type: object
                              properties:
                                name:
                                  type: string
                                fullPath:
                                  type: string
                                config:
                                  type: object
                                  nullable: true
                                  x-kubernetes-preserve-unknown-fields: true
                          resources:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                    useAllDevices:
                      type: boolean
                    deviceFilter:
                      type: string
                      nullable: true
                    devicePathFilter:
                      type: string
                    config:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    devices:
                      type: array
                      items:
                        type: object
                        properties:
                          name:
                            type: string
                          fullPath:
                            type: string
                          config:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                    storageClassDeviceSets:
                      type: array
                      nullable: true
                      items:
                        type: object
                        properties:
                          name:
                            type: string
                          count:
                            type: integer
                            format: int32
                          portable:
                            type: boolean
                          tuneDeviceClass:
                            type: boolean
                          tuneFastDeviceClass:
                            type: boolean
                          encrypted:
                            type: boolean
                          schedulerName:
                            type: string
                          config:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                          placement:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                          preparePlacement:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                          resources:
                            type: object
                            nullable: true
                            x-kubernetes-preserve-unknown-fields: true
                          volumeClaimTemplates:
                            type: array
                            items:
                              type: object
                              x-kubernetes-preserve-unknown-fields: true
                driveGroups:
                  type: array
                  nullable: true
                  items:
                    type: object
                    properties:
                      name:
                        type: string
                      spec:
                        type: object
                        x-kubernetes-preserve-unknown-fields: true
                      placement:
                        type: object
                        x-kubernetes-preserve-unknown-fields: true
                    required:
                      - name
                      - spec
                monitoring:
                  type: object
                  properties:
                    enabled:
                      type: boolean
                    rulesNamespace:
                      type: string
                    externalMgrEndpoints:
                      type: array
                      items:
                        type: object
                        properties:
                          ip:
                            type: string
                    externalMgrPrometheusPort:
                      type: integer
                      minimum: 0
                      maximum: 65535
                removeOSDsIfOutAndSafeToRemove:
                  type: boolean
                external:
                  type: object
                  properties:
                    enable:
                      type: boolean
                cleanupPolicy:
                  type: object
                  properties:
                    allowUninstallWithVolumes:
                      type: boolean
                    confirmation:
                      type: string
                      pattern: ^$|^yes-really-destroy-data$
                    sanitizeDisks:
                      type: object
                      properties:
                        method:
                          type: string
                          pattern: ^(complete|quick)$
                        dataSource:
                          type: string
                          pattern: ^(zero|random)$
                        iteration:
                          type: integer
                          format: int32
                logCollector:
                  type: object
                  properties:
                    enabled:
                      type: boolean
                    periodicity:
                      type: string
                annotations:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                placement:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                labels:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                resources:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                healthCheck:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                priorityClassNames:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: DataDirHostPath
          type: string
          description: Directory used on the K8s nodes
          jsonPath: .spec.dataDirHostPath
        - name: MonCount
          type: string
          description: Number of MONs
          jsonPath: .spec.mon.count
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
        - name: Phase
          type: string
          description: Phase
          jsonPath: .status.phase
        - name: Message
          type: string
          description: Message
          jsonPath: .status.message
        - name: Health
          type: string
          description: Ceph Health
          jsonPath: .status.ceph.health
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephclients.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephClient
    listKind: CephClientList
    plural: cephclients
    singular: cephclient
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                caps:
                  type: object
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephrbdmirrors.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephRBDMirror
    listKind: CephRBDMirrorList
    plural: cephrbdmirrors
    singular: cephrbdmirror
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                count:
                  type: integer
                  minimum: 1
                  maximum: 100
                peers:
                  type: object
                  properties:
                    secretNames:
                      type: array
                      items:
                        type: string
                resources:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                priorityClassName:
                  type: string
                placement:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                annotations:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystems.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystem
    listKind: CephFilesystemList
    plural: cephfilesystems
    singular: cephfilesystem
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                metadataServer:
                  type: object
                  properties:
                    activeCount:
                      minimum: 1
                      maximum: 10
                      type: integer
                    activeStandby:
                      type: boolean
                    annotations:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    placement:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    resources:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    priorityClassName:
                      type: string
                    labels:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                metadataPool:
                  type: object
                  nullable: true
                  properties:
                    failureDomain:
                      type: string
                    deviceClass:
                      type: string
                    crushRoot:
                      type: string
                    replicated:
                      type: object
                      nullable: true
                      properties:
                        size:
                          minimum: 0
                          maximum: 10
                          type: integer
                        requireSafeReplicaSize:
                          type: boolean
                        replicasPerFailureDomain:
                          type: integer
                        subFailureDomain:
                          type: string
                    parameters:
                      type: object
                      x-kubernetes-preserve-unknown-fields: true
                    erasureCoded:
                      type: object
                      nullable: true
                      properties:
                        dataChunks:
                          minimum: 0
                          maximum: 10
                          type: integer
                        codingChunks:
                          minimum: 0
                          maximum: 10
                          type: integer
                    compressionMode:
                      type: string
                      enum:
                        - ""
                        - none
                        - passive
                        - aggressive
                        - force
                dataPools:
                  type: array
                  nullable: true
                  items:
                    type: object
                    properties:
                      failureDomain:
                        type: string
                      deviceClass:
                        type: string
                      crushRoot:
                        type: string
                      replicated:
                        type: object
                        nullable: true
                        properties:
                          size:
                            minimum: 0
                            maximum: 10
                            type: integer
                          requireSafeReplicaSize:
                            type: boolean
                          replicasPerFailureDomain:
                            type: integer
                          subFailureDomain:
                            type: string
                      erasureCoded:
                        type: object
                        nullable: true
                        properties:
                          dataChunks:
                            minimum: 0
                            maximum: 10
                            type: integer
                          codingChunks:
                            minimum: 0
                            maximum: 10
                            type: integer
                      compressionMode:
                        type: string
                        enum:
                          - ""
                          - none
                          - passive
                          - aggressive
                          - force
                      parameters:
                        type: object
                        nullable: true
                        x-kubernetes-preserve-unknown-fields: true
                preservePoolsOnDelete:
                  type: boolean
                preserveFilesystemOnDelete:
                  type: boolean
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: ActiveMDS
          type: string
          description: Number of desired active MDS daemons
          jsonPath: .spec.metadataServer.activeCount
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephnfses.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephNFS
    listKind: CephNFSList
    plural: cephnfses
    singular: cephnfs
    shortNames:
      - nfs
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                rados:
                  type: object
                  properties:
                    pool:
                      type: string
                    namespace:
                      type: string
                server:
                  type: object
                  properties:
                    active:
                      type: integer
                    annotations:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    placement:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    resources:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    priorityClassName:
                      type: string
                    logLevel:
                      type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstores.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStore
    listKind: CephObjectStoreList
    plural: cephobjectstores
    singular: cephobjectstore
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                gateway:
                  type: object
                  properties:
                    type:
                      type: string
                    sslCertificateRef:
                      type: string
                      nullable: true
                    port:
                      type: integer
                      minimum: 0
                      maximum: 65535
                    securePort:
                      type: integer
                      minimum: 0
                      maximum: 65535
                    instances:
                      type: integer
                    externalRgwEndpoints:
                      type: array
                      nullable: true
                      items:
                        type: object
                        properties:
                          ip:
                            type: string
                    annotations:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    placement:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    resources:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    labels:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                    priorityClassName:
                      type: string
                metadataPool:
                  type: object
                  nullable: true
                  properties:
                    failureDomain:
                      type: string
                    crushRoot:
                      type: string
                    replicated:
                      type: object
                      nullable: true
                      properties:
                        size:
                          type: integer
                        requireSafeReplicaSize:
                          type: boolean
                        replicasPerFailureDomain:
                          type: integer
                        subFailureDomain:
                          type: string
                    erasureCoded:
                      type: object
                      nullable: true
                      properties:
                        dataChunks:
                          type: integer
                        codingChunks:
                          type: integer
                    compressionMode:
                      type: string
                      enum:
                        - ""
                        - none
                        - passive
                        - aggressive
                        - force
                    parameters:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                zone:
                  type: object
                  properties:
                    name:
                      type: string
                dataPool:
                  type: object
                  nullable: true
                  properties:
                    failureDomain:
                      type: string
                    crushRoot:
                      type: string
                    replicated:
                      type: object
                      nullable: true
                      properties:
                        size:
                          type: integer
                        requireSafeReplicaSize:
                          type: boolean
                        replicasPerFailureDomain:
                          type: integer
                        subFailureDomain:
                          type: string
                    erasureCoded:
                      type: object
                      nullable: true
                      properties:
                        dataChunks:
                          type: integer
                        codingChunks:
                          type: integer
                    compressionMode:
                      type: string
                      enum:
                        - ""
                        - none
                        - passive
                        - aggressive
                        - force
                    parameters:
                      type: object
                      x-kubernetes-preserve-unknown-fields: true
                preservePoolsOnDelete:
                  type: boolean
                healthCheck:
                  type: object
                  nullable: true
                  properties:
                    bucket:
                      type: object
                      nullable: true
                      properties:
                        enabled:
                          type: boolean
                        interval:
                          type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstoreusers.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStoreUser
    listKind: CephObjectStoreUserList
    plural: cephobjectstoreusers
    singular: cephobjectstoreuser
    shortNames:
      - rcou
      - objectuser
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                store:
                  type: string
                displayName:
                  type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephobjectrealms.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectRealm
    listKind: CephObjectRealmList
    plural: cephobjectrealms
    singular: cephobjectrealm
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                pull:
                  type: object
                  properties:
                    endpoint:
                      type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephobjectzonegroups.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectZoneGroup
    listKind: CephObjectZoneGroupList
    plural: cephobjectzonegroups
    singular: cephobjectzonegroup
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                realm:
                  type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephobjectzones.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectZone
    listKind: CephObjectZoneList
    plural: cephobjectzones
    singular: cephobjectzone
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                zoneGroup:
                  type: string
                metadataPool:
                  type: object
                  nullable: true
                  properties:
                    failureDomain:
                      type: string
                    crushRoot:
                      type: string
                    replicated:
                      type: object
                      nullable: true
                      properties:
                        size:
                          type: integer
                        requireSafeReplicaSize:
                          type: boolean
                    erasureCoded:
                      type: object
                      nullable: true
                      properties:
                        dataChunks:
                          type: integer
                        codingChunks:
                          type: integer
                    compressionMode:
                      type: string
                      enum:
                        - ""
                        - none
                        - passive
                        - aggressive
                        - force
                    parameters:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                dataPool:
                  type: object
                  properties:
                    failureDomain:
                      type: string
                    crushRoot:
                      type: string
                    replicated:
                      type: object
                      nullable: true
                      properties:
                        size:
                          type: integer
                        requireSafeReplicaSize:
                          type: boolean
                    erasureCoded:
                      type: object
                      nullable: true
                      properties:
                        dataChunks:
                          type: integer
                        codingChunks:
                          type: integer
                    compressionMode:
                      type: string
                      enum:
                        - ""
                        - none
                        - passive
                        - aggressive
                        - force
                    parameters:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                preservePoolsOnDelete:
                  type: boolean
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephblockpools.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephBlockPool
    listKind: CephBlockPoolList
    plural: cephblockpools
    singular: cephblockpool
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                failureDomain:
                  type: string
                deviceClass:
                  type: string
                crushRoot:
                  type: string
                replicated:
                  type: object
                  nullable: true
                  properties:
                    size:
                      type: integer
                      minimum: 0
                      maximum: 9
                    targetSizeRatio:
                      type: number
                    requireSafeReplicaSize:
                      type: boolean
                    replicasPerFailureDomain:
                      type: integer
                    subFailureDomain:
                      type: string
                erasureCoded:
                  type: object
                  nullable: true
                  properties:
                    dataChunks:
                      type: integer
                      minimum: 0
                      maximum: 9
                    codingChunks:
                      type: integer
                      minimum: 0
                      maximum: 9
                compressionMode:
                  type: string
                  enum:
                    - ""
                    - none
                    - passive
                    - aggressive
                    - force
                enableRBDStats:
                  description:
                    EnableRBDStats is used to enable gathering of statistics
                    for all RBD images in the pool
                  type: boolean
                parameters:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                mirroring:
                  type: object
                  nullable: true
                  properties:
                    enabled:
                      type: boolean
                    mode:
                      type: string
                      enum:
                        - image
                        - pool
                    snapshotSchedules:
                      type: array
                      items:
                        type: object
                        nullable: true
                        properties:
                          interval:
                            type: string
                          startTime:
                            type: string
                statusCheck:
                  type: object
                  x-kubernetes-preserve-unknown-fields: true
                annotations:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: volumes.rook.io
spec:
  group: rook.io
  names:
    kind: Volume
    listKind: VolumeList
    plural: volumes
    singular: volume
    shortNames:
      - rv
  scope: Namespaced
  versions:
    - name: v1alpha2
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            attachments:
              type: array
              items:
                type: object
                properties:
                  node:
                    type: string
                  podNamespace:
                    type: string
                  podName:
                    type: string
                  clusterName:
                    type: string
                  mountDir:
                    type: string
                  readOnly:
                    type: boolean
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: objectbuckets.objectbucket.io
spec:
  group: objectbucket.io
  names:
    kind: ObjectBucket
    listKind: ObjectBucketList
    plural: objectbuckets
    singular: objectbucket
    shortNames:
      - ob
      - obs
  scope: Cluster
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                storageClassName:
                  type: string
                endpoint:
                  type: object
                  nullable: true
                  properties:
                    bucketHost:
                      type: string
                    bucketPort:
                      type: integer
                      format: int32
                    bucketName:
                      type: string
                    region:
                      type: string
                    subRegion:
                      type: string
                    additionalConfig:
                      type: object
                      nullable: true
                      x-kubernetes-preserve-unknown-fields: true
                authentication:
                  type: object
                  nullable: true
                  items:
                    type: object
                    x-kubernetes-preserve-unknown-fields: true
                additionalState:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                reclaimPolicy:
                  type: string
                claimRef:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: objectbucketclaims.objectbucket.io
spec:
  group: objectbucket.io
  names:
    kind: ObjectBucketClaim
    listKind: ObjectBucketClaimList
    plural: objectbucketclaims
    singular: objectbucketclaim
    shortNames:
      - obc
      - obcs
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                storageClassName:
                  type: string
                bucketName:
                  type: string
                generateBucketName:
                  type: string
                additionalConfig:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                objectBucketName:
                  type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystemmirrors.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystemMirror
    listKind: CephFilesystemMirrorList
    plural: cephfilesystemmirrors
    singular: cephfilesystemmirror
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                resources:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                priorityClassName:
                  type: string
                placement:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
                annotations:
                  type: object
                  nullable: true
                  x-kubernetes-preserve-unknown-fields: true
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}`
	}
	return `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephclusters.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephCluster
    listKind: CephClusterList
    plural: cephclusters
    singular: cephcluster
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            annotations: {}
            cephVersion:
              properties:
                allowUnsupported:
                  type: boolean
                image:
                  type: string
            dashboard:
              properties:
                enabled:
                  type: boolean
                urlPrefix:
                  type: string
                port:
                  type: integer
                  minimum: 0
                  maximum: 65535
                ssl:
                  type: boolean
            dataDirHostPath:
              pattern: ^/(\S+)
              type: string
            disruptionManagement:
              properties:
                machineDisruptionBudgetNamespace:
                  type: string
                managePodBudgets:
                  type: boolean
                osdMaintenanceTimeout:
                  type: integer
                pgHealthCheckTimeout:
                  type: integer
                manageMachineDisruptionBudgets:
                  type: boolean
            skipUpgradeChecks:
              type: boolean
            mon:
              properties:
                allowMultiplePerNode:
                  type: boolean
                count:
                  maximum: 9
                  minimum: 0
                  type: integer
                volumeClaimTemplate: {}
            mgr:
              properties:
                modules:
                  items:
                    properties:
                      name:
                        type: string
                      enabled:
                        type: boolean
            network:
              properties:
                hostNetwork:
                  type: boolean
                provider:
                  type: string
                selectors: {}
            storage:
              properties:
                disruptionManagement:
                  properties:
                    machineDisruptionBudgetNamespace:
                      type: string
                    managePodBudgets:
                      type: boolean
                    osdMaintenanceTimeout:
                      type: integer
                    pgHealthCheckTimeout:
                      type: integer
                    manageMachineDisruptionBudgets:
                      type: boolean
                useAllNodes:
                  type: boolean
                nodes:
                  items:
                    properties:
                      name:
                        type: string
                      config:
                        properties:
                          metadataDevice:
                            type: string
                          storeType:
                            type: string
                            pattern: ^(bluestore)$
                          databaseSizeMB:
                            type: string
                          walSizeMB:
                            type: string
                          journalSizeMB:
                            type: string
                          osdsPerDevice:
                            type: string
                          encryptedDevice:
                            type: string
                            pattern: ^(true|false)$
                      useAllDevices:
                        type: boolean
                      deviceFilter: {}
                      devices:
                        type: array
                        items:
                          properties:
                            name:
                              type: string
                            config: {}
                      resources: {}
                useAllDevices:
                  type: boolean
                deviceFilter: {}
                config: {}
                storageClassDeviceSets: {}
            driveGroups:
              type: array
              items:
                properties:
                  name:
                    type: string
                  spec: {}
                  placement: {}
                required:
                - name
                - spec
            monitoring:
              properties:
                enabled:
                  type: boolean
                rulesNamespace:
                  type: string
            rbdMirroring:
              properties:
                workers:
                  type: integer
            removeOSDsIfOutAndSafeToRemove:
              type: boolean
            external:
              properties:
                enable:
                  type: boolean
            cleanupPolicy:
              properties:
                confirmation:
                  type: string
                  pattern: ^$|^yes-really-destroy-data$
                sanitizeDisks:
                  properties:
                    method:
                      type: string
                      pattern: ^(complete|quick)$
                    dataSource:
                      type: string
                      pattern: ^(zero|random)$
                    iteration:
                      type: integer
                      format: int32
            security:
              properties:
                kms:
                  properties:
                    connectionDetails:
                      type: object
                    tokenSecretName:
                      type: string
            placement: {}
            resources: {}
  additionalPrinterColumns:
    - name: DataDirHostPath
      type: string
      description: Directory used on the K8s nodes
      JSONPath: .spec.dataDirHostPath
    - name: MonCount
      type: string
      description: Number of MONs
      JSONPath: .spec.mon.count
    - name: Age
      type: date
      JSONPath: .metadata.creationTimestamp
    - name: State
      type: string
      description: Current State
      JSONPath: .status.state
    - name: Health
      type: string
      description: Ceph Health
      JSONPath: .status.ceph.health
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystems.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystem
    listKind: CephFilesystemList
    plural: cephfilesystems
    singular: cephfilesystem
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            metadataServer:
              properties:
                activeCount:
                  minimum: 1
                  maximum: 10
                  type: integer
                activeStandby:
                  type: boolean
                annotations: {}
                placement: {}
                resources: {}
            metadataPool:
              properties:
                failureDomain:
                  type: string
                crushRoot:
                  type: string
                replicated:
                  properties:
                    size:
                      minimum: 0
                      maximum: 10
                      type: integer
                    requireSafeReplicaSize:
                      type: boolean
                    replicasPerFailureDomain:
                      type: integer
                    subFailureDomain:
                      type: string
                erasureCoded:
                  properties:
                    dataChunks:
                      minimum: 0
                      maximum: 10
                      type: integer
                    codingChunks:
                      minimum: 0
                      maximum: 10
                      type: integer
                compressionMode:
                  type: string
                  enum:
                  - ""
                  - none
                  - passive
                  - aggressive
                  - force
            dataPools:
              type: array
              items:
                properties:
                  failureDomain:
                    type: string
                  crushRoot:
                    type: string
                  replicated:
                    properties:
                      size:
                        minimum: 0
                        maximum: 10
                        type: integer
                      requireSafeReplicaSize:
                        type: boolean
                    replicasPerFailureDomain:
                      type: integer
                    subFailureDomain:
                      type: string
                  erasureCoded:
                    properties:
                      dataChunks:
                        minimum: 0
                        maximum: 10
                        type: integer
                      codingChunks:
                        minimum: 0
                        maximum: 10
                        type: integer
                  compressionMode:
                    type: string
                    enum:
                    - ""
                    - none
                    - passive
                    - aggressive
                    - force
                  parameters:
                    type: object
            preservePoolsOnDelete:
              type: boolean
            preserveFilesystemOnDelete:
              type: boolean
  additionalPrinterColumns:
    - name: ActiveMDS
      type: string
      description: Number of desired active MDS daemons
      JSONPath: .spec.metadataServer.activeCount
    - name: Age
      type: date
      JSONPath: .metadata.creationTimestamp
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephnfses.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephNFS
    listKind: CephNFSList
    plural: cephnfses
    singular: cephnfs
    shortNames:
    - nfs
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            rados:
              properties:
                pool:
                  type: string
                namespace:
                  type: string
            server:
              properties:
                active:
                  type: integer
                annotations: {}
                placement: {}
                resources: {}
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstores.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStore
    listKind: CephObjectStoreList
    plural: cephobjectstores
    singular: cephobjectstore
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            gateway:
              properties:
                type:
                  type: string
                sslCertificateRef: {}
                port:
                  type: integer
                  minimum: 0
                  maximum: 65535
                securePort:
                  type: integer
                  minimum: 0
                  maximum: 65535
                instances:
                  type: integer
                annotations: {}
                placement: {}
                resources: {}
            metadataPool:
              properties:
                failureDomain:
                  type: string
                crushRoot:
                  type: string
                replicated:
                  properties:
                    size:
                      type: integer
                    requireSafeReplicaSize:
                      type: boolean
                    replicasPerFailureDomain:
                      type: integer
                    subFailureDomain:
                      type: string
                erasureCoded:
                  properties:
                    dataChunks:
                      type: integer
                    codingChunks:
                      type: integer
                compressionMode:
                  type: string
                  enum:
                  - ""
                  - none
                  - passive
                  - aggressive
                  - force
                parameters:
                  type: object
            dataPool:
              properties:
                failureDomain:
                  type: string
                crushRoot:
                  type: string
                replicated:
                  properties:
                    size:
                      type: integer
                    requireSafeReplicaSize:
                      type: boolean
                    replicasPerFailureDomain:
                      type: integer
                    subFailureDomain:
                      type: string
                erasureCoded:
                  properties:
                    dataChunks:
                      type: integer
                    codingChunks:
                      type: integer
                compressionMode:
                  type: string
                  enum:
                  - ""
                  - none
                  - passive
                  - aggressive
                  - force
                parameters:
                  type: object
            preservePoolsOnDelete:
              type: boolean
            healthCheck:
              properties:
                bucket:
                  properties:
                    enabled:
                      type: boolean
                    interval:
                      type: string
  subresources:
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectstoreusers.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectStoreUser
    listKind: CephObjectStoreUserList
    plural: cephobjectstoreusers
    singular: cephobjectstoreuser
    shortNames:
    - rcou
    - objectuser
  scope: Namespaced
  version: v1
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectrealms.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectRealm
    listKind: CephObjectRealmList
    plural: cephobjectrealms
    singular: cephobjectrealm
    shortNames:
    - realm
    - realms
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectzonegroups.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectZoneGroup
    listKind: CephObjectZoneGroupList
    plural: cephobjectzonegroups
    singular: cephobjectzonegroup
    shortNames:
    - zonegroup
    - zonegroups
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephobjectzones.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephObjectZone
    listKind: CephObjectZoneList
    plural: cephobjectzones
    singular: cephobjectzone
    shortNames:
    - zone
    - zones
  scope: Namespaced
  version: v1
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephblockpools.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephBlockPool
    listKind: CephBlockPoolList
    plural: cephblockpools
    singular: cephblockpool
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            failureDomain:
                type: string
            crushRoot:
                type: string
            replicated:
              properties:
                size:
                  type: integer
                  minimum: 0
                  maximum: 9
                targetSizeRatio:
                  type: number
                requireSafeReplicaSize:
                  type: boolean
                replicasPerFailureDomain:
                  type: integer
                subFailureDomain:
                  type: string
            erasureCoded:
              properties:
                dataChunks:
                  type: integer
                  minimum: 0
                  maximum: 9
                codingChunks:
                  type: integer
                  minimum: 0
                  maximum: 9
            compressionMode:
                type: string
                enum:
                - ""
                - none
                - passive
                - aggressive
                - force
            enableRBDStats:
              description: EnableRBDStats is used to enable gathering of statistics
                for all RBD images in the pool
              type: boolean
            parameters:
              type: object
            mirrored:
              properties:
                enabled:
                  type: boolean
                mode:
                  type: string
                  enum:
                  - image
                  - pool
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: volumes.rook.io
spec:
  group: rook.io
  names:
    kind: Volume
    listKind: VolumeList
    plural: volumes
    singular: volume
    shortNames:
    - rv
  scope: Namespaced
  version: v1alpha2
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectbuckets.objectbucket.io
spec:
  group: objectbucket.io
  versions:
    - name: v1alpha1
      served: true
      storage: true
  names:
    kind: ObjectBucket
    listKind: ObjectBucketList
    plural: objectbuckets
    singular: objectbucket
    shortNames:
      - ob
      - obs
  scope: Cluster
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: objectbucketclaims.objectbucket.io
spec:
  versions:
    - name: v1alpha1
      served: true
      storage: true
  group: objectbucket.io
  names:
    kind: ObjectBucketClaim
    listKind: ObjectBucketClaimList
    plural: objectbucketclaims
    singular: objectbucketclaim
    shortNames:
      - obc
      - obcs
  scope: Namespaced
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephclients.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephClient
    listKind: CephClientList
    plural: cephclients
    singular: cephclient
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            caps:
              properties:
                mon:
                  type: string
                osd:
                  type: string
                mds:
                  type: string
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephrbdmirrors.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephRBDMirror
    listKind: CephRBDMirrorList
    plural: cephrbdmirrors
    singular: cephrbdmirror
  scope: Namespaced
  version: v1
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            count:
              type: integer
              minimum: 1
              maximum: 100
            peers:
              properties:
                secretNames:
                  type: array
  subresources:
    status: {}
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: cephfilesystemmirrors.ceph.rook.io
spec:
  group: ceph.rook.io
  names:
    kind: CephFilesystemMirror
    listKind: CephFilesystemMirrorList
    plural: cephfilesystemmirrors
    singular: cephfilesystemmirror
  scope: Namespaced
  version: v1
  subresources:
    status: {}`
}

func getOpenshiftSCC(namespace string) string {
	return `---
kind: SecurityContextConstraints
# older versions of openshift have "apiVersion: v1"
apiVersion: security.openshift.io/v1
metadata:
  name: rook-ceph
allowPrivilegedContainer: true
allowHostNetwork: true
allowHostDirVolumePlugin: true
priority:
allowedCapabilities: []
allowHostPorts: true
allowHostPID: true
allowHostIPC: true
readOnlyRootFilesystem: false
requiredDropCapabilities: []
defaultAddCapabilities: []
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
fsGroup:
  type: MustRunAs
supplementalGroups:
  type: RunAsAny
allowedFlexVolumes:
  - driver: "ceph.rook.io/rook"
  - driver: "ceph.rook.io/rook-ceph"
volumes:
  - configMap
  - downwardAPI
  - emptyDir
  - flexVolume
  - hostPath
  - persistentVolumeClaim
  - projected
  - secret
users:
  # A user needs to be added for each rook service account.
  # This assumes running in the default sample "rook-ceph" namespace.
  # If other namespaces or service accounts are configured, they need to be updated here.
  - system:serviceaccount:` + namespace + `:rook-ceph-system
  - system:serviceaccount:` + namespace + `:default
  - system:serviceaccount:` + namespace + `:rook-ceph-mgr
  - system:serviceaccount:` + namespace + `:rook-ceph-osd
---
kind: SecurityContextConstraints
# older versions of openshift have "apiVersion: v1"
apiVersion: security.openshift.io/v1
metadata:
  name: rook-ceph-csi
allowPrivilegedContainer: true
allowHostNetwork: true
allowHostDirVolumePlugin: true
priority:
allowedCapabilities: ['*']
allowHostPorts: true
allowHostPID: true
allowHostIPC: true
readOnlyRootFilesystem: false
requiredDropCapabilities: []
defaultAddCapabilities: []
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: RunAsAny
fsGroup:
  type: RunAsAny
supplementalGroups:
  type: RunAsAny
allowedFlexVolumes:
  - driver: "ceph.rook.io/rook"
  - driver: "ceph.rook.io/rook-ceph"
volumes: ['*']
users:
  # A user needs to be added for each rook service account.
  # This assumes running in the default sample "rook-ceph" namespace.
  # If other namespaces or service accounts are configured, they need to be updated here.
  - system:serviceaccount:` + namespace + `:rook-csi-rbd-plugin-sa
  - system:serviceaccount:` + namespace + `:rook-csi-rbd-provisioner-sa
  - system:serviceaccount:` + namespace + `:rook-csi-cephfs-plugin-sa
  - system:serviceaccount:` + namespace + `:rook-csi-cephfs-provisioner-sa
`
}

// GetRookOperator returns rook Operator manifest
func (m *CephManifestsMaster) GetRookOperator(operatorNamespace string) string {
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
- apiGroups:
  - batch
  resources:
  - cronjobs
  verbs:
  - delete
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
  - cronjobs
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
func (m *CephManifestsMaster) GetClusterRoles(namespace, operatorNamespace string) string {
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
func (m *CephManifestsMaster) GetRookCluster(settings *clusterSettings) string {
	store := "# storeType not specified; Rook will use default store types"
	if settings.StoreType != "" {
		store = `storeType: "` + settings.StoreType + `"`
	}

	crushRoot := "# crushRoot not specified; Rook will use `default`"
	if settings.Mons == 1 {
		crushRoot = `crushRoot: "custom-root"`
	}

	pruner := "# daysToRetain not specified;"
	if settings.useCrashPruner {
		pruner = "daysToRetain: 5"
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
    ` + pruner + `
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
    managePodBudgets: true
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
  crashCollector:
    disable: false
    ` + pruner + `
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
    deviceFilter:  ` + getDeviceFilter() + `
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
func (m *CephManifestsMaster) GetRookToolBox(namespace string) string {
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

func (m *CephManifestsMaster) GetBlockSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string {
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

func (m *CephManifestsMaster) GetFileStorageSnapshotClass(snapshotClassName, namespace, operatorNamespace, reclaimPolicy string) string {
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

func (m *CephManifestsMaster) GetPVCRestore(claimName, snapshotName, namespace, storageClassName, accessModes, size string) string {
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

func (m *CephManifestsMaster) GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, accessModes, size string) string {
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

func (m *CephManifestsMaster) GetSnapshot(snapshotName, claimName, snapshotClassName, namespace string) string {
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

func (m *CephManifestsMaster) GetPod(podName, claimName, namespace, mountPath string, readOnly bool) string {
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

func (m *CephManifestsMaster) GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
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
  statusCheck:
    mirror:
      disabled: false
      interval: 10s`
}

func (m *CephManifestsMaster) GetBlockStorageClassDef(csi bool, poolName, storageClassName, reclaimPolicy, namespace, operatorNamespace string) string {
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

func (m *CephManifestsMaster) GetFileStorageClassDef(fsName, storageClassName, operatorNamespace, namespace string) string {
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

func (m *CephManifestsMaster) GetPVC(claimName, namespace, storageClassName, accessModes, size string) string {
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
func (m *CephManifestsMaster) GetFilesystem(namespace, name string, activeCount int) string {
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
func (m *CephManifestsMaster) GetNFS(namespace, name, pool string, count int) string {
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

func (m *CephManifestsMaster) GetObjectStore(namespace, name string, replicaCount, port int) string {
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

func (m *CephManifestsMaster) GetObjectStoreUser(namespace, name string, displayName string, store string) string {
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
func (m *CephManifestsMaster) GetBucketStorageClass(storeNameSpace string, storeName string, storageClassName string, reclaimPolicy string, region string) string {
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
func (m *CephManifestsMaster) GetObc(claimName string, storageClassName string, objectBucketName string, maxObject string, varBucketName bool) string {
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

func (m *CephManifestsMaster) GetClient(namespace string, claimName string, caps map[string]string) string {
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

func (m *CephManifestsMaster) GetClusterExternalRoles(namespace, firstClusterNamespace string) string {
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

func (m *CephManifestsMaster) GetRookExternalCluster(settings *clusterExternalSettings) string {
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
func (m *CephManifestsMaster) GetRBDMirror(namespace, name string, count int) string {
	return `apiVersion: ceph.rook.io/v1
kind: CephRBDMirror
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  count: ` + strconv.Itoa(count)
}
