/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package integration

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ***************************************************
// *** Major scenarios tested by the TestKeystoneAuthSuite ***
// Setup
// - A CephObject store will be created in a test cluster
// Access
// - Create a container
// - Put a file in a container
// - Get a file from the container
// - Try to get the file without valid credentials
// ***************************************************

func TestCephKeystoneAuthSuite(t *testing.T) {
	s := new(KeystoneAuthSuite)
	defer func(s *KeystoneAuthSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type KeystoneAuthSuite struct {
	suite.Suite
	helper    *clients.TestClient
	installer *installer.CephInstaller
	settings  *installer.TestCephSettings
	k8shelper *utils.K8sHelper
}

func (h *KeystoneAuthSuite) SetupSuite() {
	namespace := "keystoneauth-ns"
	h.settings = &installer.TestCephSettings{
		Namespace:         namespace,
		OperatorNamespace: namespace,
		StorageClassName:  "",
		UseHelm:           true,
		UsePVC:            false,
		Mons:              1,
		SkipOSDCreation:   false,
		// EnableAdmissionController: true,
		EnableDiscovery:      true,
		ChangeHostName:       true,
		ConnectionsEncrypted: true,
		RookVersion:          installer.LocalBuildTag,
		CephVersion:          installer.QuincyVersion,
		SkipClusterCleanup:   false,
		SkipCleanupPolicy:    false,
	}
	h.settings.ApplyEnvVars()
	h.installer, h.k8shelper = StartTestCluster(h.T, h.settings)

	// install yaook-keystone here
	if err := InstallKeystoneInTestCluster(h.k8shelper, namespace); err != nil {
		return
	}

	// create usersecret for object store to use
	testCtx := context.TODO()

	secrets := map[string][]byte{
		"OS_AUTH_TYPE":            []byte("password"),
		"OS_IDENTITY_API_VERSION": []byte("3"),
		"OS_PROJECT_DOMAIN_NAME":  []byte("Default"),
		"OS_USER_DOMAIN_NAME":     []byte("Default"),
		"OS_PROJECT_NAME":         []byte(testuserdata["rook-user"]["project"]),
		"OS_USERNAME":             []byte(testuserdata["rook-user"]["username"]),
		"OS_PASSWORD":             []byte(testuserdata["rook-user"]["password"]),
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "usersecret",
			Namespace: namespace,
		},
		Data: secrets,
	}

	_, err := h.k8shelper.Clientset.CoreV1().Secrets(namespace).Create(testCtx, secret, metav1.CreateOptions{})

	if err != nil {
		return
	}

	h.helper = clients.CreateTestClient(h.k8shelper, h.installer.Manifests)
}

func (h *KeystoneAuthSuite) TearDownSuite() {
	CleanUpKeystoneInTestCluster(h.k8shelper, h.settings.Namespace)
	h.installer.UninstallRook()
}

func (h *KeystoneAuthSuite) AfterTest(suiteName, testName string) {
	h.installer.CollectOperatorLog(suiteName, testName)
}

// Test Object StoreCreation on Rook that was installed via helm
func (h *KeystoneAuthSuite) TestObjectStoreOnRookInstalledViaHelmUsingKeystone() {
	deleteStore := true
	tls := false
	swiftAndKeystone := true

	runObjectE2ETestLite(h.T(), h.helper, h.k8shelper, h.installer, h.settings.Namespace, "default", 3, deleteStore, tls, swiftAndKeystone)
}

func (h *KeystoneAuthSuite) TestWithSwiftAndKeystone() {
	deleteStore := true
	tls := false
	swiftAndKeystone := true

	objectStoreServicePrefix = objectStoreServicePrefixUniq
	runSwiftE2ETest(h.T(), h.helper, h.k8shelper, h.installer, h.settings.Namespace, "default", 3, deleteStore, tls, swiftAndKeystone)
	cleanUpTLSks(h)

}

func cleanUpTLSks(h *KeystoneAuthSuite) {
	err := h.k8shelper.Clientset.CoreV1().Secrets(h.settings.Namespace).Delete(context.TODO(), objectTLSSecretName, metav1.DeleteOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Fatal("failed to deleted store TLS secret")
		}
	}
	logger.Info("successfully deleted store TLS secret")
}