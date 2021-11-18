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
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	objectStoreServicePrefixUniq = "rook-ceph-rgw-"
	objectStoreTLSName           = "tls-test-store"
)

var (
	objectStoreServicePrefix = "rook-ceph-rgw-"
)

func TestCephObjectSuite(t *testing.T) {
	if installer.SkipTestSuite(installer.CephTestSuite) {
		t.Skip()
	}

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
		ClusterName:               "object-cluster",
		Namespace:                 namespace,
		OperatorNamespace:         installer.SystemNamespace(namespace),
		StorageClassName:          installer.StorageClassName(),
		UseHelm:                   false,
		UsePVC:                    installer.UsePVC(),
		Mons:                      3,
		SkipOSDCreation:           false,
		EnableAdmissionController: true,
		UseCrashPruner:            true,
		EnableVolumeReplication:   false,
		RookVersion:               installer.LocalBuildTag,
		CephVersion:               installer.ReturnCephVersion(),
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
	objectStoreServicePrefix = objectStoreServicePrefixUniq
	runObjectE2ETest(s.helper, s.k8sh, s.Suite, s.settings.Namespace, tls)
	err := s.k8sh.Clientset.CoreV1().Secrets(s.settings.Namespace).Delete(context.TODO(), objectTLSSecretName, metav1.DeleteOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
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
	objectStoreServicePrefix = objectStoreServicePrefixUniq
	runObjectE2ETest(s.helper, s.k8sh, s.Suite, s.settings.Namespace, tls)
}

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
// Test for ObjectStore with and without TLS enabled
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, tlsEnable bool) {
	storeName := "test-store"
	if tlsEnable {
		storeName = objectStoreTLSName
	}

	logger.Infof("Running on Rook Cluster %s", namespace)
	createCephObjectStore(s.T(), helper, k8sh, namespace, storeName, 3, tlsEnable)

	// test that a second object store can be created (and deleted) while the first exists
	s.T().Run("run a second object store", func(t *testing.T) {
		otherStoreName := "other-" + storeName
		// The lite e2e test is perfect, as it only creates a cluster, checks that it is healthy,
		// and then deletes it.
		deleteStore := true
		runObjectE2ETestLite(t, helper, k8sh, namespace, otherStoreName, 1, deleteStore, tlsEnable)
	})
	if tlsEnable {
		// test that a third object store can be created (and deleted) while the first exists
		s.T().Run("run a third object store with broken tls", func(t *testing.T) {
			otherStoreName := "broken-" + storeName
			// The lite e2e test is perfect, as it only creates a cluster, checks that it is healthy,
			// and then deletes it.
			deleteStore := true
			objectStoreServicePrefix = objectStoreServicePrefixUniq
			runObjectE2ETestLite(t, helper, k8sh, namespace, otherStoreName, 1, deleteStore, tlsEnable)
			objectStoreServicePrefix = objectStoreServicePrefixUniq
		})
	}

	// now test operation of the first object store
	testObjectStoreOperations(s, helper, k8sh, namespace, storeName)
}

func testObjectStoreOperations(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	ctx := context.TODO()
	clusterInfo := client.AdminTestClusterInfo(namespace)
	t := s.T()

	t.Run("create CephObjectStoreUser", func(t *testing.T) {
		createCephObjectUser(s, helper, k8sh, namespace, storeName, userid, true, true)
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
		cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
		assert.Nil(t, cobErr)
		cobcErr := helper.BucketClient.CreateObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
		assert.Nil(t, cobcErr)

		created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
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

	t.Run("S3 access to OBC bucket", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		if objectStore.Spec.IsTLSEnabled() {
			s3client, err = rgw.NewInsecureS3Agent(s3AccessKey, s3SecretKey, s3endpoint, region, true)
		} else {
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, region, true, nil)
		}

		assert.Nil(t, err)
		logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

		t.Run("put object", func(t *testing.T) {
			_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey1, contentType)
			assert.Nil(t, poErr)
		})

		t.Run("get object", func(t *testing.T) {
			read, err := s3client.GetObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(t, err)
			assert.Equal(t, ObjBody, read)
		})

		t.Run("quota enforcement", func(t *testing.T) {
			_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey2, contentType)
			assert.Nil(t, poErr)
			logger.Infof("Testing the max object limit")
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
			assert.Error(t, poErr)
		})

		t.Run("update quota limits", func(t *testing.T) {
			poErr := helper.BucketClient.UpdateObc(obcName, bucketStorageClassName, bucketname, newMaxObject, true)
			assert.Nil(t, poErr)
			updated := utils.Retry(5, 2*time.Second, "OBC is updated", func() bool {
				return helper.BucketClient.CheckOBMaxObject(obcName, newMaxObject)
			})
			assert.True(t, updated)
			logger.Infof("Testing the updated object limit")
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
			assert.NoError(t, poErr)
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey4, contentType)
			assert.Error(t, poErr)
		})

		t.Run("delete objects", func(t *testing.T) {
			_, delobjErr := s3client.DeleteObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(t, delobjErr)
			_, delobjErr = s3client.DeleteObjectInBucket(bucketname, ObjectKey2)
			assert.Nil(t, delobjErr)
			_, delobjErr = s3client.DeleteObjectInBucket(bucketname, ObjectKey3)
			assert.Nil(t, delobjErr)
			logger.Info("Objects deleted on bucket successfully")
		})
	})

	t.Run("Regression check: OBC does not revert to Pending phase", func(t *testing.T) {
		// A bug exists in older versions of lib-bucket-provisioner that will revert a bucket and claim
		// back to "Pending" phase after being created and initially "Bound" by looping infinitely in
		// the bucket provision/creation loop. Verify that the OBC is "Bound" and stays that way.
		// The OBC reconcile loop runs again immediately b/c the OBC is modified to refer to its OB.
		// Wait a short amount of time before checking just to be safe.
		time.Sleep(15 * time.Second)
		assert.True(t, helper.BucketClient.CheckOBC(obcName, "bound"))
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

		dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
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

	t.Run("CephObjectStore should delete now that dependents are gone", func(t *testing.T) {
		// wait initially since it will almost never detect on the first try without this.
		time.Sleep(3 * time.Second)

		assertObjectStoreDeletion(t, k8sh, namespace, storeName)
	})

	// TODO : Add case for brownfield/cleanup s3 client}
}
