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
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePassword(t *testing.T) {
	password, err := GeneratePassword(0)
	require.Nil(t, err)
	assert.Equal(t, "", password)

	password, err = GeneratePassword(1)
	require.Nil(t, err)
	assert.Equal(t, 1, len(password))
	logger.Infof("password: %s", password)

	password, err = GeneratePassword(10)
	require.Nil(t, err)
	assert.Equal(t, 10, len(password))
	logger.Infof("password: %s", password)
}

func TestGetOrGeneratePassword(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 3)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns", OwnerInfo: ownerInfo}
	c := &Cluster{context: &clusterd.Context{Clientset: clientset}, clusterInfo: clusterInfo}
	_, err := c.context.Clientset.CoreV1().Secrets(clusterInfo.Namespace).Get(ctx, dashboardPasswordName, metav1.GetOptions{})
	assert.True(t, kerrors.IsNotFound(err))

	// Generate a password
	password, err := c.getOrGenerateDashboardPassword()
	require.Nil(t, err)
	assert.Equal(t, passwordLength, len(password))

	secret, err := c.context.Clientset.CoreV1().Secrets(clusterInfo.Namespace).Get(ctx, dashboardPasswordName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(secret.Data))
	passwordFromSecret, err := decodeSecret(secret)
	assert.NoError(t, err)
	assert.Equal(t, password, passwordFromSecret)

	// We should retrieve the same password on the second call
	retrievedPassword, err := c.getOrGenerateDashboardPassword()
	assert.Nil(t, err)
	assert.Equal(t, password, retrievedPassword)
}

func TestStartSecureDashboard(t *testing.T) {
	ctx := context.TODO()
	enables := 0
	disables := 0
	moduleRetries := 0
	exitCodeResponse := 0
	clientset := test.New(t, 3)
	mockFN := func(command string, args ...string) (string, error) {
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
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: mockFN,
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
			return mockFN(command, arg...)
		},
	}

	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "myns",
		CephVersion: cephver.Nautilus,
		OwnerInfo:   ownerInfo,
		Context:     ctx,
	}
	c := &Cluster{clusterInfo: clusterInfo, context: &clusterd.Context{Clientset: clientset, Executor: executor},
		spec: cephv1.ClusterSpec{
			Dashboard:   cephv1.DashboardSpec{Port: dashboardPortHTTP, Enabled: true, SSL: true},
			CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
		},
	}
	c.exitCode = func(err error) (int, bool) {
		if exitCodeResponse != 0 {
			return exitCodeResponse, true
		}
		return exitCodeResponse, false
	}

	dashboardInitWaitTime = 0
	err := c.configureDashboardService("a")
	assert.NoError(t, err)
	err = c.configureDashboardModules()
	assert.NoError(t, err)
	// the dashboard is enabled once with the new dashboard and modules
	assert.Equal(t, 2, enables)
	assert.Equal(t, 1, disables)
	assert.Equal(t, 2, moduleRetries)

	svc, err := c.context.Clientset.CoreV1().Services(clusterInfo.Namespace).Get(ctx, "rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)

	// disable the dashboard
	c.spec.Dashboard.Enabled = false
	err = c.configureDashboardService("a")
	assert.Nil(t, err)
	err = c.configureDashboardModules()
	assert.NoError(t, err)
	assert.Equal(t, 2, enables)
	assert.Equal(t, 2, disables)

	svc, err = c.context.Clientset.CoreV1().Services(clusterInfo.Namespace).Get(ctx, "rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, kerrors.IsNotFound(err))
	assert.Nil(t, svc)
}

func TestFileBasedPasswordSupported(t *testing.T) {
	// for Ceph version Nautilus 14.2.17
	clusterInfo := &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 17}}
	value := FileBasedPasswordSupported(clusterInfo)
	assert.True(t, value)

	// for Ceph version Octopus 15.2.10
	clusterInfo = &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 10}}
	value = FileBasedPasswordSupported(clusterInfo)
	assert.True(t, value)

	// for Ceph version Pacific
	clusterInfo = &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 16, Minor: 0, Extra: 0}}
	value = FileBasedPasswordSupported(clusterInfo)
	assert.True(t, value)

	// for Ceph version Quincy
	clusterInfo = &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 17, Minor: 0, Extra: 0}}
	value = FileBasedPasswordSupported(clusterInfo)
	assert.True(t, value)

	// for other Ceph Versions
	clusterInfo = &cephclient.ClusterInfo{CephVersion: cephver.CephVersion{Major: 14, Minor: 2, Extra: 15}}
	value = FileBasedPasswordSupported(clusterInfo)
	assert.False(t, value)
}
