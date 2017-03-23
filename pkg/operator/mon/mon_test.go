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
package mon

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"

	testclient "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
)

func TestStartMonPods(t *testing.T) {
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	c := New("ns", factory, "", "myversion")
	c.retryDelay = 0

	clientset := test.New(3)

	// start a basic cluster
	// an error is expected since mocking always creates pods that are not running
	info, err := c.Start(clientset)
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c, clientset)

	// starting again should be a no-op, but still results in an error
	info, err = c.Start(clientset)
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c, clientset)
}

func validateStart(t *testing.T, c *Cluster, clientset *fake.Clientset) {
	s, err := clientset.CoreV1().Secrets(c.Namespace).Get("rook-admin")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(s.StringData))

	s, err = clientset.CoreV1().Secrets(c.Namespace).Get("mon")
	assert.Nil(t, err)
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	p, err := clientset.CoreV1().Pods(c.Namespace).Get("mon0")
	assert.Nil(t, err)
	assert.Equal(t, "mon0", p.Name)

	pods, err := clientset.CoreV1().Pods(c.Namespace).List(v1.ListOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pods.Items))

	// no pods are running or pending
	running, pending, err := c.pollPods(clientset, c.ClusterName)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(running))
	assert.Equal(t, 0, len(pending))
}
