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
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePassword(t *testing.T) {
	password, err := generatePassword(0)
	require.Nil(t, err)
	assert.Equal(t, "", password)

	password, err = generatePassword(1)
	require.Nil(t, err)
	assert.Equal(t, 1, len(password))
	logger.Infof("password: %s", password)

	password, err = generatePassword(10)
	require.Nil(t, err)
	assert.Equal(t, 10, len(password))
	logger.Infof("password: %s", password)
}

func TestGetOrGeneratePassword(t *testing.T) {
	c := &Cluster{context: &clusterd.Context{Clientset: test.New(3)}, Namespace: "myns"}
	_, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(dashboardPasswordName, metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err))

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
					return "", errors.New("test failure")
				}
			}
			return "", nil
		},
	}
	executor.MockExecuteCommandWithOutputFileTimeout = func(debug bool, timeout time.Duration, actionName string, command, outfileArg string, arg ...string) (string, error) {
		return executor.MockExecuteCommandWithOutputFile(debug, actionName, command, outfileArg, arg...)
	}

	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Mimic,
	}
	c := &Cluster{clusterInfo: clusterInfo, context: &clusterd.Context{Clientset: test.New(3), Executor: executor}, Namespace: "myns",
		dashboard: cephv1.DashboardSpec{Port: dashboardPortHTTP, Enabled: true, SSL: true}, cephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:v13.2.2"}}
	c.exitCode = func(err error) (int, bool) {
		if exitCodeResponse != 0 {
			return exitCodeResponse, true
		}
		return exitCodeResponse, false
	}

	dashboardInitWaitTime = 0
	err := c.configureDashboardService()
	assert.NoError(t, err)
	err = c.configureDashboardModules()
	assert.NoError(t, err)
	// the dashboard is enabled, then disabled and enabled again to restart
	// it with the cert, and another restart when setting the dashboard port
	assert.Equal(t, 2, enables)
	assert.Equal(t, 1, disables)
	assert.Equal(t, 2, moduleRetries)

	svc, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)

	// disable the dashboard
	c.dashboard.Enabled = false
	err = c.configureDashboardService()
	assert.Nil(t, err)
	err = c.configureDashboardModules()
	assert.NoError(t, err)
	assert.Equal(t, 2, enables)
	assert.Equal(t, 2, disables)

	svc, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, kerrors.IsNotFound(err))
	assert.Nil(t, svc)
}
