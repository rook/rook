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
package mgr

import (
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePassword(t *testing.T) {
	password := generatePassword(0)
	assert.Equal(t, "", password)

	password = generatePassword(1)
	assert.Equal(t, 1, len(password))
	logger.Infof("password: %s", password)

	password = generatePassword(10)
	assert.Equal(t, 10, len(password))
	logger.Infof("password: %s", password)
}

func TestGetOrGeneratePassword(t *testing.T) {
	c := &Cluster{context: &clusterd.Context{Clientset: test.New(3)}, Namespace: "myns"}
	_, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(dashboardPasswordName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// Generate a password
	password, err := c.getOrGenerateDashboardPassword()
	require.Nil(t, err)
	assert.Equal(t, passwordLength, len(password))

	secret, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(dashboardPasswordName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(secret.Data))
	passwordFromSecret, err := decodeSecret(secret)
	assert.Equal(t, password, passwordFromSecret)

	// We should retrieve the same password on the second call
	retrievedPassword, err := c.getOrGenerateDashboardPassword()
	assert.Nil(t, err)
	assert.Equal(t, password, retrievedPassword)
}

func TestStartSecureDashboard(t *testing.T) {
	enables := 0
	disables := 0
	moduleRetries := 0
	exitCodeResponse := 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("command: %s %v", command, args)
			exitCodeResponse = 0
			if args[1] == "module" {
				if args[2] == "enable" {
					enables++
				} else if args[2] == "disable" {
					disables++
				}
			}
			if args[0] == "dashboard" && args[1] == "create-self-signed-cert" {
				if moduleRetries < 2 {
					logger.Infof("simulating retry...")
					exitCodeResponse = invalidArgErrorCode
					moduleRetries++
					return "", fmt.Errorf("test failure")
				}
			}
			return "", nil
		},
	}
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Mimic,
	}
	c := &Cluster{clusterInfo: clusterInfo, context: &clusterd.Context{Clientset: test.New(3), Executor: executor}, Namespace: "myns",
		dashboard: cephv1.DashboardSpec{Enabled: true}, cephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:v13.2.2"}}
	c.exitCode = func(err error) (int, bool) {
		if exitCodeResponse != 0 {
			return exitCodeResponse, true
		}
		return exitCodeResponse, false
	}
	mgrConfig := &mgrConfig{
		DaemonID:      "a",
		ResourceName:  "mgr",
		DashboardPort: dashboardPortHTTP,
	}

	dashboardInitWaitTime = 0
	err := c.configureDashboard(mgrConfig)
	assert.Nil(t, err)
	// the dashboard is enabled, then disabled and enabled again to restart
	// it with the cert, and another restart when setting the dashboard port
	assert.Equal(t, 3, enables)
	assert.Equal(t, 2, disables)
	assert.Equal(t, 2, moduleRetries)

	svc, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)

	// disable the dashboard
	c.dashboard.Enabled = false
	err = c.configureDashboard(mgrConfig)
	assert.Nil(t, err)
	assert.Equal(t, 3, enables)
	assert.Equal(t, 3, disables)

	svc, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
	assert.Nil(t, svc)
}
