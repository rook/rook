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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
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
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("command: %s %v", command, args)
			if args[1] == "module" {
				if args[2] == "enable" {
					enables++
				} else if args[2] == "disable" {
					disables++
				}
			}
			return "", nil
		},
	}
	c := &Cluster{context: &clusterd.Context{Clientset: test.New(3), Executor: executor}, Namespace: "myns",
		dashboard: cephv1beta1.DashboardSpec{Enabled: true}, cephVersion: cephv1beta1.CephVersionSpec{Name: cephv1beta1.Mimic, Image: "ceph/ceph:13.2.2"}}
	dashboardInitWaitTime = 0
	err := c.configureDashboard()
	assert.Nil(t, err)
	// the dashboard is enabled, then disabled and enabled again to restart it with the cert
	assert.Equal(t, 2, enables)
	assert.Equal(t, 1, disables)

	svc, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)

	// disable the dashboard
	c.dashboard.Enabled = false
	err = c.configureDashboard()
	assert.Nil(t, err)
	assert.Equal(t, 2, enables)
	assert.Equal(t, 2, disables)

	svc, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-ceph-mgr-dashboard", metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
	assert.Nil(t, svc)
}
