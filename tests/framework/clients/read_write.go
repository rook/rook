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

package clients

import (
	"fmt"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
)

// NfsClientOperation is a wrapper for k8s rook file operations
type NfsClientOperation struct {
	k8sh *utils.K8sHelper
}

// CreateNfsClientOperation Constructor to create Nfs Client Operations - client to perform rook file system operations on k8s
func CreateNfsClientOperation(k8sh *utils.K8sHelper) *NfsClientOperation {
	return &NfsClientOperation{k8sh: k8sh}
}

// CreateReadWriteClient Function to create a nfs client in rook
func (f *NfsClientOperation) CreateReadWriteClient(name string, volName string) ([]string, error) {
	logger.Infof("creating the filesystem via replication controller")
	writerSpec := getReadWriteReplicationController(name, volName)

	if err := f.k8sh.ResourceOperation("create", writerSpec); err != nil {
		return nil, err
	}

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState(name, "default", 2, "Running"),
		"Make sure there are two read-write-test pods present in Running state")

	podList, err := f.k8sh.GetPodNamesForApp(name, "default")
	if err != nil {
		return nil, err
	}

	return podList, nil
}

// CreateReadOnlyClient Function to create a container to just do read operations...
func (f *NfsClientOperation) CreateReadOnlyClient(name, volName string) ([]string, error) {
	logger.Infof("createh the filesystem via replication controller")
	readerSpec := getReadOnlyReplicationController(name, volName)

	if _, err := f.k8sh.ResourceOperation("create", readerSpec); err != nil {
		return nil, err
	}

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState(name, "default", 2, "Running"),
		"Make sure there are two read-only-test pods present in Running state")

	podList, err := f.k8sh.GetPodNamesForApp(name, "default")
	if err != nil {
		return nil, err
	}

	return podList, nil
}

// Delete Function to delete a nfs consuming pod in rook
func (f *NfsClientOperation) Delete(name string) (string, error) {
	return f.k8sh.DeleteResource("rc", name)
}

// Read Function to read from nfs mount point created by rook ,i.e. Read data from a pod that has an nfs export mounted
func (f *NfsClientOperation) Read(name string) (string, error) {
	rd := "/mnt/data"

	args := []string{"exec", name}

	args = append(args, "--", "cat", rd)

	result, err := f.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to read data from pod -- : %s", err)

	}
	return result, nil
}

func getReadWriteReplicationController(name string, volName string) string {
	return `apiVersion: v1
kind: ReplicationController
metadata:
  name: ` + name + `
spec:
  replicas: 2
  selector:
    app: ` + name + `
  template:
    metadata:
      labels:
        app: ` + name + `
    spec:
      containers:
      - image: alpine
        command:
          - sh
          - -c
          - 'while true; do hostname > /mnt/data; sleep 3; done'
        name: alpine
        volumeMounts:
          - name: test-vol
            mountPath: "/mnt"
      volumes:
      - name: test-vol
        persistentVolumeClaim:
          claimName: ` + volName + `
`
}

func getReadOnlyReplicationController(name, volName string) string {
	return `apiVersion: v1
kind: ReplicationController
metadata:
  name: ` + name + `
spec:
  replicas: 2
  selector:
    app: ` + name + `
  template:
    metadata:
      labels:
        app: ` + name + `
    spec:
      containers:
      - image: alpine
				command:
					- sh
					- -c
					'while sleep 3600; do :; done'
        name: alpine
        volumeMounts:
          - name: test-vol
            mountPath: "/mnt"
      volumes:
      - name: test-vol
        persistentVolumeClaim:
          claimName: ` + volName + `
`
}
