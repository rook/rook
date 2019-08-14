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

// ReadWriteOperation is a wrapper for k8s rook file operations
type ReadWriteOperation struct {
	k8sh *utils.K8sHelper
}

// CreateReadWriteOperation Constructor to create ReadWriteOperation - client to perform rook file system operations on k8s
func CreateReadWriteOperation(k8sh *utils.K8sHelper) *ReadWriteOperation {
	return &ReadWriteOperation{k8sh: k8sh}
}

// CreateWriteClient Function to create a nfs client in rook
func (f *ReadWriteOperation) CreateWriteClient(volName string) ([]string, error) {
	logger.Infof("creating the filesystem via replication controller")
	writerSpec := getDeployment(volName)

	if err := f.k8sh.ResourceOperation("apply", writerSpec); err != nil {
		return nil, err
	}

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("read-write-test", "default", 2, "Running"),
		"Make sure there are two read-write-test pods present in Running state")

	podList, err := f.k8sh.GetPodNamesForApp("read-write-test", "default")
	if err != nil {
		return nil, err
	}

	return podList, nil
}

// Delete Function to delete a nfs consuming pod in rook
func (f *ReadWriteOperation) Delete() error {
	return f.k8sh.DeleteResource("deployment", "read-write-test")
}

// Read Function to read from nfs mount point created by rook ,i.e. Read data from a pod that has an nfs export mounted
func (f *ReadWriteOperation) Read(name string) (string, error) {
	rd := "/mnt/data"

	args := []string{"exec", name}

	args = append(args, "--", "cat", rd)

	result, err := f.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("unable to write data to pod -- : %s", err)

	}
	return result, nil
}

func getDeployment(volName string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: read-write-test
spec:
  replicas: 2
  selector:
    matchLabels:
      app: read-write-test
  template:
    metadata:
      labels:
        app: read-write-test
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
