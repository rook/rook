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

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	bucketlifecycle "github.com/rook/rook/tests/integration/object/bucket/lifecycle"
	bucketowner "github.com/rook/rook/tests/integration/object/bucket/owner"
	bucketpolicy "github.com/rook/rook/tests/integration/object/bucket/policy"
	bucketquota "github.com/rook/rook/tests/integration/object/bucket/quota"
	bucketrw "github.com/rook/rook/tests/integration/object/bucket/rw"
	"github.com/rook/rook/tests/integration/object/cosi"
	"github.com/rook/rook/tests/integration/object/dependents"
	"github.com/rook/rook/tests/integration/object/notification"
	topickafka "github.com/rook/rook/tests/integration/object/topic/kafka"
	usercaps "github.com/rook/rook/tests/integration/object/user/caps"
	userkeys "github.com/rook/rook/tests/integration/object/user/keys"
	useropmask "github.com/rook/rook/tests/integration/object/user/opmask"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
)

const (
	objectStoreServicePrefixUniq = "rook-ceph-rgw-"
	objectStoreTLSName           = "tls-test-store"
)

var objectStoreServicePrefix = "rook-ceph-rgw-"

func TestCephObjectSuite(t *testing.T) {
	s := new(ObjectSuite)
	defer func(s *ObjectSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type ObjectSuite struct {
	suite.Suite
	helper    *clients.TestClient
	settings  *installer.TestCephSettings
	installer *installer.CephInstaller
	k8sh      *utils.K8sHelper
}

func (s *ObjectSuite) SetupSuite() {
	namespace := "object-ns"
	s.settings = &installer.TestCephSettings{
		ClusterName:             "object-cluster",
		Namespace:               namespace,
		OperatorNamespace:       installer.SystemNamespace(namespace),
		StorageClassName:        installer.StorageClassName(),
		UseHelm:                 false,
		UsePVC:                  installer.UsePVC(),
		Mons:                    3,
		SkipOSDCreation:         false,
		UseCrashPruner:          true,
		EnableVolumeReplication: false,
		RookVersion:             installer.LocalBuildTag,
		CephVersion:             installer.ReturnCephVersion(),
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	s.helper = clients.CreateTestClient(s.k8sh, s.installer.Manifests)
}

func (s *ObjectSuite) AfterTest(suiteName, testName string) {
	s.installer.CollectOperatorLog(suiteName, testName)
}

func (s *ObjectSuite) TearDownSuite() {
	s.installer.UninstallRook()
}

func (s *ObjectSuite) TestWithTLS() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}

	tls := true
	swiftAndKeystone := false
	objectStoreServicePrefix = objectStoreServicePrefixUniq
	runObjectE2ETest(s.helper, s.k8sh, s.installer, &s.Suite, s.settings, tls, swiftAndKeystone)
	cleanUpTLS(s)
}

func cleanUpTLS(s *ObjectSuite) {
	err := s.k8sh.Clientset.CoreV1().Secrets(s.settings.Namespace).Delete(context.TODO(), objectTLSSecretName, metav1.DeleteOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			logger.Fatal("failed to deleted store TLS secret")
		}
	}
	logger.Info("successfully deleted store TLS secret")
}

func (s *ObjectSuite) TestWithoutTLS() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}

	tls := false
	swiftAndKeystone := false
	objectStoreServicePrefix = objectStoreServicePrefixUniq
	runObjectE2ETest(s.helper, s.k8sh, s.installer, &s.Suite, s.settings, tls, swiftAndKeystone)
}

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
// Test for ObjectStore with and without TLS enabled
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, s *suite.Suite, settings *installer.TestCephSettings, tlsEnable bool, swiftAndKeystone bool) {
	namespace := settings.Namespace
	storeName := "test-store"
	if tlsEnable {
		storeName = objectStoreTLSName
	}

	logger.Infof("Running on Rook Cluster %s", namespace)
	// a single gateway makes quota enforcement deterministic: rgw quota checks
	// run against per-instance cached stats, and with multiple gateways an
	// instance can be blind to writes served by its peers for up to
	// rgw_user_quota_bucket_sync_interval; multi-instance deployment is still
	// covered by the keystone suite
	createCephObjectStore(s.T(), helper, k8sh, installer, namespace, storeName, 1, tlsEnable, swiftAndKeystone)

	// Canary test: verify all *_pool fields in zone.json are covered by Rook's zonePoolNSSuffix map.
	// This catches new RGW pool fields added in newer Ceph versions that Rook doesn't yet handle,
	// which would cause ghost default pools when shared pools are configured.
	// Run right after store creation when the zone is fresh and definitely accessible.
	s.T().Run("all zone.json pool fields are covered by Rook shared pool mapping", func(t *testing.T) {
		output, err := installer.Execute("radosgw-admin",
			[]string{"zone", "get", fmt.Sprintf("--rgw-zone=%s", storeName), fmt.Sprintf("--rgw-realm=%s", storeName)}, namespace)
		require.NoError(t, err, "failed to get zone config; output: %s", output)
		require.NotEmpty(t, output, "zone config is empty")

		var zoneConfig map[string]interface{}
		err = json.Unmarshal([]byte(output), &zoneConfig)
		require.NoError(t, err, "failed to parse zone config JSON; output: %s", output)

		knownPools := rgw.ZoneJsonPoolKeys()
		for field, val := range zoneConfig {
			if _, ok := val.(string); !ok {
				continue
			}
			if !strings.HasSuffix(field, "_pool") {
				continue
			}
			assert.Contains(t, knownPools, field,
				"RGW zone.json contains unknown pool field %q — add it to zonePoolNSSuffix in pkg/operator/ceph/object/objectstore.go", field)
		}
	})

	// test that a second object store can be created (and deleted) while the first exists
	s.T().Run("run a second object store", func(t *testing.T) {
		otherStoreName := "other-" + storeName
		// The lite e2e test is perfect, as it only creates a cluster, checks that it is healthy,
		// and then deletes it.
		deleteStore := true
		runObjectE2ETestLite(t, helper, k8sh, installer, namespace, otherStoreName, 1, deleteStore, tlsEnable, swiftAndKeystone)
	})

	// the deletion checks that consumed this store live in the dependents
	// package now; delete it so it cannot block cluster teardown
	s.T().Run("delete the suite object store", func(t *testing.T) {
		deleteObjectStore(t, k8sh, namespace, storeName)
		assertObjectStoreDeletion(t, k8sh, namespace, storeName)
	})

	sharedObjectStore := sharedstore.Create(s.T(), k8sh, installer, tlsEnable,
		bucketlifecycle.Namespace,
		bucketowner.Namespace,
		bucketpolicy.Namespace,
		bucketquota.Namespace,
		bucketrw.Namespace,
		userkeys.Namespace,
		topickafka.Namespace,
		useropmask.Namespace,
		usercaps.Namespace,
		cosi.Namespace,
		dependents.Namespace,
		notification.Namespace,
	)
	defer sharedObjectStore.Destroy()

	bucketlifecycle.TestObjectBucketClaimLifecycle(s.T(), k8sh, sharedObjectStore)
	bucketowner.TestObjectBucketClaimBucketOwner(s.T(), k8sh, sharedObjectStore)
	bucketpolicy.TestObjectBucketClaimPolicy(s.T(), k8sh, sharedObjectStore)
	bucketquota.TestObjectBucketClaimQuota(s.T(), k8sh, sharedObjectStore)
	bucketrw.TestObjectBucketClaimReadWrite(s.T(), k8sh, sharedObjectStore)
	userkeys.TestObjectStoreUserKeys(s.T(), k8sh, sharedObjectStore)
	topickafka.TestBucketTopicKafka(s.T(), k8sh, sharedObjectStore)
	useropmask.TestObjectStoreUserOpMask(s.T(), k8sh, sharedObjectStore)
	usercaps.TestObjectStoreUserCaps(s.T(), k8sh, sharedObjectStore)
	// the ceph-cosi driver cannot reach a TLS object store endpoint, so this
	// suite skips itself in the TLS pass
	cosi.TestCephCOSIDriver(s.T(), k8sh, sharedObjectStore)
	dependents.TestCephObjectStoreDependents(s.T(), k8sh, sharedObjectStore)
	notification.TestBucketNotification(s.T(), k8sh, sharedObjectStore)
}
