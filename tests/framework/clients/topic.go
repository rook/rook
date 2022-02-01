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

package clients

import (
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

// TopicOperation is a wrapper for rook topic operations
type TopicOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateTopicOperation creates a new topic client
func CreateTopicOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *TopicOperation {
	return &TopicOperation{k8sh, manifests}
}

func (t *TopicOperation) CreateTopic(topicName string, storeName string, httpEndpointService string) error {
	return t.k8sh.ResourceOperation("create", t.manifests.GetBucketTopic(topicName, storeName, httpEndpointService))
}

func (t *TopicOperation) DeleteTopic(topicName string, storeName string, httpEndpointService string) error {
	return t.k8sh.ResourceOperation("delete", t.manifests.GetBucketTopic(topicName, storeName, httpEndpointService))
}

func (t *TopicOperation) UpdateTopic(topicName string, storeName string, httpEndpointService string) error {
	return t.k8sh.ResourceOperation("apply", t.manifests.GetBucketTopic(topicName, storeName, httpEndpointService))
}

// CheckTopic if topic has an ARN set in its status
func (t *TopicOperation) CheckTopic(topicName string) bool {
	const resourceName = "cephbuckettopic"
	_, err := t.k8sh.GetResource(resourceName, topicName)
	if err != nil {
		logger.Infof("%q %q does not exist", resourceName, topicName)
		return false
	}

	topicARN, _ := t.k8sh.GetResource(resourceName, topicName, "--output", "jsonpath={.status.ARN}")
	if topicARN == "" {
		logger.Infof("%q %q exist, but ARN was not set", resourceName, topicName)
		return false
	}

	logger.Infof("topic ARN is %q", topicARN)
	return true
}

func (t *TopicOperation) CreateHTTPServer(serverName, namespace, port string) error {
	// TODO: Fix https://github.com/rook/rook/issues/9741, do not use third party image
	deployment := `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ` + serverName + `
  namespace: ` + namespace + `
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ` + serverName + `
  template:
    metadata:
      labels:
        app: ` + serverName + `
    spec:
      containers:
      - name: ` + serverName + `
        image: quay.io/jthottan/pythonwebserver:latest
---
apiVersion: v1
kind: Service
metadata:
  name: ` + serverName + `
  namespace: ` + namespace + `
spec:
  type: NodePort
  selector:
    app: ` + serverName + `
  ports:
  - port: ` + port + `
    targetPort: ` + port + `
`
	_, err := t.k8sh.KubectlWithStdin(deployment, []string{"apply", "-f", "-"}...)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return err
	}
	appLabel := "app=" + serverName
	err = t.k8sh.WaitForLabeledPodsToRun(appLabel, namespace)
	if err != nil {
		return err
	}
	return nil
}
