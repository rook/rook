/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"strconv"
)

func GetPodWithVolume(podName, claimName, namespace, mountPath string, readOnly bool) string {
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

func GetPVC(claimName, namespace, storageClassName, accessModes, size string) string {
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

func GetPVCRestore(claimName, snapshotName, namespace, storageClassName, accessModes, size string) string {
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

func GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, accessModes, size string) string {
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

func GetSnapshot(snapshotName, claimName, snapshotClassName, namespace string) string {
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
