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
package mgr

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartMGR(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}
	c := New(context, "ns", "myversion", rookalpha.Placement{}, false, cephv1beta1.DashboardSpec{Enabled: true}, v1.ResourceRequirements{}, metav1.OwnerReference{})
	defer os.RemoveAll(c.dataDir)

	// start a basic service
	err := c.Start()
	assert.Nil(t, err)
	validateStart(t, c)

	// starting again with more replicas
	c.Replicas = 3
	c.dashboard.Enabled = false
	err = c.Start()
	assert.Nil(t, err)
	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {

	for i := 0; i < c.Replicas; i++ {
		if i == len(mgrNames) {
			break
		}
		logger.Infof("Looking for cephmgr replica %d", i)
		daemonName := mgrNames[i]
		_, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get(fmt.Sprintf("rook-ceph-mgr-%s", daemonName), metav1.GetOptions{})
		assert.Nil(t, err)
	}

	_, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr", metav1.GetOptions{})
	assert.Nil(t, err)

	_, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	if c.dashboard.Enabled {
		assert.Nil(t, err)
	} else {
		assert.True(t, errors.IsNotFound(err))
	}
}
