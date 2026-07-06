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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
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

	// now test operation of the first object store
	testObjectStoreOperations(s, helper, k8sh, settings, storeName, swiftAndKeystone)

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
	notification.TestBucketNotification(s.T(), k8sh, sharedObjectStore)
}

func testObjectStoreOperations(s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, settings *installer.TestCephSettings, storeName string, swiftAndKeystone bool) {
	ctx := context.TODO()
	namespace := settings.Namespace
	clusterInfo := client.AdminTestClusterInfo(namespace)
	t := s.T()

	logger.Infof("Testing Object Operations on %s", storeName)
	t.Run("create CephObjectStoreUser", func(t *testing.T) {
		createCephObjectUser(s, helper, k8sh, namespace, storeName, userid, true)
		i := 0
		for i = 0; i < 4; i++ {
			if helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) {
				break
			}
			logger.Info("waiting 5 more seconds for user secret to exist")
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
	})

	context := k8sh.MakeContext()
	objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
	assert.Nil(t, err)
	rgwcontext, err := rgw.NewMultisiteContext(context, clusterInfo, objectStore)
	assert.Nil(t, err)
	t.Run("create ObjectBucketClaim", func(t *testing.T) {
		logger.Infof("create OBC %q with storageclass %q - using reclaim policy 'delete' so buckets don't block deletion", obcName, bucketStorageClassName)
		cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete")
		assert.Nil(t, cobErr)
		cobcErr := helper.BucketClient.CreateObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
		assert.Nil(t, cobcErr)
		created := utils.Retry(20, 2*time.Second, "OBC is created", func() bool {
			return helper.BucketClient.CheckOBC(obcName, "bound")
		})
		assert.True(t, created)
		logger.Info("OBC created successfully")

		var bkt rgw.ObjectBucket
		i := 0
		for i = 0; i < 4; i++ {
			b, code, err := rgw.GetBucket(rgwcontext, bucketname)
			if b != nil && err == nil {
				bkt = *b
				break
			}
			logger.Warningf("cannot get bucket %q, retrying... bucket: %v. code: %d, err: %v", bucketname, b, code, err)
			logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, bucketname, bkt.Name)
		logger.Info("OBC, Secret and ConfigMap created")
	})

	t.Run("delete CephObjectStore should be blocked by OBC bucket and CephObjectStoreUser", func(t *testing.T) {
		deleteObjectStore(t, k8sh, namespace, storeName)

		store := &cephv1.CephObjectStore{}
		i := 0
		for i = 0; i < 4; i++ {
			storeStr, err := k8sh.GetResource("-n", namespace, "CephObjectStore", storeName, "-o", "json")
			assert.NoError(t, err)

			err = json.Unmarshal([]byte(storeStr), &store)
			assert.NoError(t, err)

			cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
			if cond != nil {
				break
			}
			logger.Info("waiting 2 more seconds for CephObjectStore to reach Deleting state")
			time.Sleep(2 * time.Second)
		}
		assert.NotEqual(t, 4, i)

		assert.Equal(t, cephv1.ConditionDeleting, store.Status.Phase) // phase == "Deleting"
		// verify deletion is blocked b/c object has dependents
		cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
		logger.Infof("condition: %+v", cond)
		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)
		// the CephObjectStoreUser and the bucket should both block deletion
		assert.Contains(t, cond.Message, "CephObjectStoreUsers")
		assert.Contains(t, cond.Message, userid)
		assert.Contains(t, cond.Message, "buckets")
		assert.Contains(t, cond.Message, bucketname)

		// The event is created by the same method that adds that condition, so we can be pretty
		// sure it exists here. No need to do extra work to validate the event.
	})

	t.Run("delete OBC", func(t *testing.T) {
		i := 0
		dobcErr := helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
		assert.Nil(t, dobcErr)
		logger.Info("Checking to see if the obc, secret, and cm have all been deleted")
		for i = 0; i < 4 && !helper.BucketClient.CheckOBC(obcName, "deleted"); i++ {
			logger.Infof("(%d) obc deleted check, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)

		logger.Info("ensure OBC bucket was deleted")
		var rgwErr int
		for i = 0; i < 4; i++ {
			_, rgwErr, _ = rgw.GetBucket(rgwcontext, bucketname)
			if rgwErr == rgw.RGWErrorNotFound {
				break
			}
			logger.Infof("(%d) check bucket deleted, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, rgwErr, rgw.RGWErrorNotFound)

		dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete")
		assert.Nil(t, dobErr)
	})

	t.Run("delete CephObjectStoreUser", func(t *testing.T) {
		dosuErr := helper.ObjectUserClient.Delete(namespace, userid)
		assert.Nil(t, dosuErr)
		logger.Info("Object store user deleted successfully")
		logger.Info("Checking to see if the user secret has been deleted")
		i := 0
		for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == true; i++ {
			logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.False(t, helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))
	})

	t.Run("Regression check: mgrs are not in a crashloop", func(t *testing.T) {
		assert.True(t, k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
	})

	// tests are complete, now delete the objectstore
	s.T().Run("CephObjectStore should delete now that dependents are gone", func(t *testing.T) {
		// wait initially since it will almost never detect on the first try without this.
		time.Sleep(3 * time.Second)

		assertObjectStoreDeletion(t, k8sh, namespace, storeName)
	})

	// TODO : Add case for brownfield/cleanup s3 client}
}
