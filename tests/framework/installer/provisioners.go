/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/tests/framework/utils"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	hostPathStorageClassName = "hostpath"
)

// ************************************************************************************************
// HostPath provisioner functions
// ************************************************************************************************
func CreateHostPathPVs(k8shelper *utils.K8sHelper, count int, readOnly bool, pvcSize string) error {
	logger.Info("creating test PVs")

	pv := `
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: test-hostpath-%d
  labels:
    hostpath: test
spec:
  storageClassName: %s
  capacity:
    storage: %s
  accessModes:
    - %s
  persistentVolumeReclaimPolicy: Delete
  volumeMode: Filesystem
  local:
    path: "%s"
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
          - key: kubernetes.io/hostname
            operator: Exists
`
	storageClass := `
---
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: %s
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
`
	accessMode := "ReadWriteMany"
	if readOnly {
		accessMode = "ReadWriteOnce"
	}
	yamlToCreate := fmt.Sprintf(storageClass, hostPathStorageClassName)
	for i := 0; i < count; i++ {
		tempDir, err := ioutil.TempDir("", "example")
		if err != nil {
			return fmt.Errorf("failed to create temp dir. %v", err)
		}
		logger.Infof("created temp dir: %s", tempDir)
		yamlToCreate += fmt.Sprintf(pv, i, hostPathStorageClassName, pvcSize, accessMode, tempDir)
	}
	out, err := k8shelper.KubectlWithStdin(yamlToCreate, createFromStdinArgs...)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create hostpath provisioner StorageClass: %+v. %s", err, out)
	}

	present, err := k8shelper.IsStorageClassPresent(hostPathStorageClassName)
	if !present {
		logger.Errorf("storageClass %s not found: %+v", hostPathStorageClassName, err)
		k8shelper.PrintStorageClasses(true /*detailed*/)
		return err
	}

	return nil
}

func DeleteHostPathPVs(k8shelper *utils.K8sHelper) error {
	logger.Info("deleting hostpath PVs")

	args := []string{"delete", "pv", "-l", "hostpath=test"}
	_, err := k8shelper.Kubectl(args...)
	if err != nil {
		return fmt.Errorf("failed to delete test PVs. %v", err)
	}

	args = []string{"delete", "sc", hostPathStorageClassName}
	out, err := k8shelper.Kubectl(args...)
	if err != nil && !utils.IsKubectlErrorNotFound(out, err) {
		return fmt.Errorf("failed to delete hostpath StorageClass: %v. %s", err, out)
	}

	return nil
}
