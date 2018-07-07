package installer

import (
	"github.com/rook/rook/tests/framework/utils"
)

func BlockResourceOperation(k8sh *utils.K8sHelper, yaml string, action string) (string, error) {
	return k8sh.ResourceOperation(action, yaml)
}

func GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: ceph.rook.io/v1beta1
kind: Pool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize
}

func GetBlockStorageClassDef(poolName string, storageClassName string, namespace string, varClusterName bool) string {
	namespaceParameter := "clusterNamespace"
	if varClusterName {
		namespaceParameter = "clusterName"
	}
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: ceph.rook.io/block
parameters:
    pool: ` + poolName + `
    ` + namespaceParameter + `: ` + namespace
}

func GetBlockPvcDef(claimName string, storageClassName string, accessModes string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  annotations:
    volume.beta.kubernetes.io/storage-class: ` + storageClassName + `
spec:
  accessModes:
    - ` + accessModes + `
  resources:
    requests:
      storage: 1M`
}

func concatYaml(first, second string) string {
	return first + `
---
` + second
}

func GetBlockPoolStorageClassAndPvcDef(namespace string, poolName string, storageClassName string, blockName string, accessMode string) string {
	return concatYaml(GetBlockPoolDef(poolName, namespace, "1"),
		concatYaml(GetBlockStorageClassDef(poolName, storageClassName, namespace, false), GetBlockPvcDef(blockName, storageClassName, accessMode)))
}

func GetBlockPoolStorageClass(namespace string, poolName string, storageClassName string) string {
	return concatYaml(GetBlockPoolDef(poolName, namespace, "1"), GetBlockStorageClassDef(poolName, storageClassName, namespace, false))
}
