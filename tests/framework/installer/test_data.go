package installer

import (
	"github.com/rook/rook/tests/framework/utils"
)

func BlockResourceOperation(k8sh *utils.K8sHelper, yaml string, action string) (string, error) {
	return k8sh.ResourceOperation(action, yaml)
}

func GetBlockPoolDef(poolName string, namespace string, replicaSize string) string {
	return `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: ` + poolName + `
  namespace: ` + namespace + `
spec:
  replicated:
    size: ` + replicaSize
}

func GetBlockStorageClassDef(poolName string, storageClassName string, namespace string) string {
	return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + storageClassName + `
provisioner: rook.io/block
parameters:
    pool: ` + poolName + `
    clusterName: ` + namespace
}

func GetBlockPvcDef(claimName string, storageClassName string) string {
	return `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ` + claimName + `
  annotations:
    volume.beta.kubernetes.io/storage-class: ` + storageClassName + `
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1M`
}

func concatYaml(first, second string) string {
	return first + `
---
` + second
}

func GetBlockPoolStorageClassAndPvcDef(namespace string, poolName string, storageClassName string, blockName string) string {
	return concatYaml(GetBlockPoolDef(poolName, namespace, "1"),
		concatYaml(GetBlockStorageClassDef(poolName, storageClassName, namespace), GetBlockPvcDef(blockName, storageClassName)))

}
