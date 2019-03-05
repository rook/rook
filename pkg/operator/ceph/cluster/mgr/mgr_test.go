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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartMGR(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

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
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		context,
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		false,
		cephv1.DashboardSpec{Enabled: true},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
		"/var/lib/rook/",
	)
	defer os.RemoveAll(c.dataDir)

	// start a basic service
	err := c.Start()
	assert.Nil(t, err)
	validateStart(t, c)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	c.dashboard.UrlPrefix = "/test"
	c.dashboard.Port = 12345
	err = c.Start()
	assert.Nil(t, err)
	validateStart(t, c)
	assert.ElementsMatch(t, []string{"rook-ceph-mgr-a"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// starting again with more replicas
	c.Replicas = 3
	c.dashboard.Enabled = false
	err = c.Start()
	assert.Nil(t, err)
	validateStart(t, c)
	assert.ElementsMatch(t, []string{"rook-ceph-mgr-a"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
}

func validateStart(t *testing.T, c *Cluster) {
	mgrNames := []string{"a", "b"}
	for i := 0; i < c.Replicas; i++ {
		if i == 2 {
			break
		}
		logger.Infof("Looking for cephmgr replica %d", i)
		daemonName := mgrNames[i]
		_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(fmt.Sprintf("rook-ceph-mgr-%s", daemonName), metav1.GetOptions{})
		assert.Nil(t, err)
	}

	_, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr", metav1.GetOptions{})
	assert.Nil(t, err)

	ds, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	if c.dashboard.Enabled {
		assert.Nil(t, err)
		if c.dashboard.Port == 0 {
			// port=0 -> default port
			assert.Equal(t, ds.Spec.Ports[0].Port, int32(dashboardPortHTTPS))
		} else {
			// non-zero ports are configured as-is
			assert.Equal(t, ds.Spec.Ports[0].Port, int32(c.dashboard.Port))
		}
	} else {
		assert.True(t, errors.IsNotFound(err))
	}
}
