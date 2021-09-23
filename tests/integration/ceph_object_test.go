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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		UseCSI:                    true,
		EnableAdmissionController: true,
		UseCrashPruner:            true,
		RookVersion:               installer.LocalBuildTag,
		CephVersion:               installer.PacificVersion,
	}
	s.settings.ApplyEnvVars()
	s.installer, s.k8sh = StartTestCluster(s.T, s.settings)
	if s.k8sh.VersionAtLeast("v1.16.0") {
		s.settings.EnableVolumeReplication = true
	}
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
	runObjectE2ETest(s.helper, s.k8sh, s.Suite, s.settings.Namespace, tls)
}

func (s *ObjectSuite) TestWithoutTLS() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}

	tls := false
	runObjectE2ETest(s.helper, s.k8sh, s.Suite, s.settings.Namespace, tls)
}

// Smoke Test for ObjectStore - Test check the following operations on ObjectStore in order
// Create object store, Create User, Connect to Object Store, Create Bucket, Read/Write/Delete to bucket,
// Check issues in MGRs, Delete Bucket and Delete user
// Test for ObjectStore with and without TLS enabled
func runObjectE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, tlsEnable bool) {
	storeName := "test-store"
	if tlsEnable {
		storeName = "tls-test-store"
	}

	logger.Infof("Running on Rook Cluster %s", namespace)
	createCephObjectStore(s, helper, k8sh, namespace, storeName, 3, tlsEnable)
	testObjectStoreOperations(s, helper, k8sh, namespace, storeName)
}

func objectStoreCleanUp(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	logger.Infof("Delete Object Store (will fail if users and buckets still exist)")
	err := helper.ObjectClient.Delete(namespace, storeName)
	assert.Nil(s.T(), err)
	logger.Infof("Done deleting object store")
}

func createCephObjectUser(
	s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string,
	checkPhase, checkQuotaAndCaps bool) {
	s.T().Helper()
	maxObjectInt, err := strconv.Atoi(maxObject)
	assert.Nil(s.T(), err)
	cosuErr := helper.ObjectUserClient.Create(userID, userdisplayname, storeName, userCap, maxSize, maxBucket, maxObjectInt)
	assert.Nil(s.T(), cosuErr)
	logger.Infof("Waiting 5 seconds for the object user to be created")
	time.Sleep(5 * time.Second)
	logger.Infof("Checking to see if the user secret has been created")
	for i := 0; i < 6 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID) == false; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}

	checkCephObjectUser(s, helper, k8sh, namespace, storeName, userID, checkPhase, checkQuotaAndCaps)
}

func checkCephObjectUser(
	s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string,
	checkPhase, checkQuotaAndCaps bool,
) {
	s.T().Helper()

	logger.Infof("checking object store \"%s/%s\" user %q", namespace, storeName, userID)
	assert.True(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID))

	userInfo, err := helper.ObjectUserClient.GetUser(namespace, storeName, userID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), userID, userInfo.UserID)
	assert.Equal(s.T(), userdisplayname, *userInfo.DisplayName)

	if checkPhase {
		// status.phase doesn't exist before Rook v1.6
		phase, err := k8sh.GetResource("--namespace", namespace, "cephobjectstoreuser", userID, "--output", "jsonpath={.status.phase}")
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), k8sutil.ReadyStatus, phase)
	}
	if checkQuotaAndCaps {
		// following fields in CephObjectStoreUser CRD doesn't exist before Rook v1.7.3
		maxObjectInt, err := strconv.Atoi(maxObject)
		assert.Nil(s.T(), err)
		maxSizeInt, err := strconv.Atoi(maxSize)
		assert.Nil(s.T(), err)
		assert.Equal(s.T(), maxBucket, userInfo.MaxBuckets)
		assert.Equal(s.T(), int64(maxObjectInt), *userInfo.UserQuota.MaxObjects)
		assert.Equal(s.T(), int64(maxSizeInt), *userInfo.UserQuota.MaxSize)
		assert.Equal(s.T(), userCap, userInfo.Caps[0].Perm)
	}
}

func createCephObjectStore(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string, replicaSize int, tlsEnable bool) {
	logger.Infof("Create Object Store %q with replica count %d", storeName, replicaSize)
	rgwServiceName := "rook-ceph-rgw-" + storeName
	if tlsEnable {
		generateRgwTlsCertSecret(s, helper, k8sh, namespace, storeName, rgwServiceName)
	}
	t := s.T()
	t.Run("create CephObjectStore", func(t *testing.T) {
		err := helper.ObjectClient.Create(namespace, storeName, 3, tlsEnable)
		assert.Nil(s.T(), err)

		// check that ObjectStore is created
		logger.Infof("Check that RGW pods are Running")
		for i := 0; i < 24 && k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, 1, "Running") == false; i++ {
			logger.Infof("(%d) RGW pod check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-rgw", namespace, 1, "Running"))
		logger.Info("RGW pods are running")
		logger.Infof("Object store %q created successfully", storeName)
	})
}

func testObjectStoreOperations(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	ctx := context.TODO()
	clusterInfo := client.AdminClusterInfo(namespace)
	t := s.T()
	t.Run(fmt.Sprintf("create CephObjectStoreUser %q", storeName), func(t *testing.T) {
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

	// Check object store status
	t.Run(fmt.Sprintf("verify ceph object store %q status", storeName), func(t *testing.T) {
		retryCount := 30
		i := 0
		for i = 0; i < retryCount; i++ {
			objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
			assert.Nil(s.T(), err)
			if objectStore.Status == nil || objectStore.Status.BucketStatus == nil {
				logger.Infof("(%d) object status check sleeping for 5 seconds ...%+v", i, objectStore.Status)
				time.Sleep(5 * time.Second)
				continue
			}
			logger.Info("objectstore status is", objectStore.Status)
			if objectStore.Status.BucketStatus.Health == cephv1.ConditionFailure {
				logger.Infof("(%d) bucket status check sleeping for 5 seconds ...%+v", i, objectStore.Status.BucketStatus)
				time.Sleep(5 * time.Second)
				continue
			}
			assert.Equal(s.T(), cephv1.ConditionConnected, objectStore.Status.BucketStatus.Health)
			// Info field has the endpoint in it
			assert.NotEmpty(s.T(), objectStore.Status.Info)
			assert.NotEmpty(s.T(), objectStore.Status.Info["endpoint"])
			break
		}
		if i == retryCount {
			t.Fatal("bucket status check failed. status is not connected")
		}
	})

	context := k8sh.MakeContext()
	objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
	assert.Nil(s.T(), err)
	rgwcontext, err := rgw.NewMultisiteContext(context, clusterInfo, objectStore)
	assert.Nil(s.T(), err)
	t.Run("create ObjectBucketClaim with reclaim policy delete", func(t *testing.T) {
		cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
		assert.Nil(s.T(), cobErr)
		cobcErr := helper.BucketClient.CreateObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
		assert.Nil(s.T(), cobcErr)

		created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
			return helper.BucketClient.CheckOBC(obcName, "bound")
		})
		assert.True(s.T(), created)
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
		assert.NotEqual(s.T(), 4, i)
		assert.Equal(s.T(), bucketname, bkt.Name)
		logger.Info("OBC, Secret and ConfigMap created")
	})

	t.Run("test S3 get/put/delete", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		if objectStore.Spec.IsTLSEnabled() {
			s3client, err = rgw.NewTestOnlyS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true)
		} else {
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil)
		}

		assert.Nil(s.T(), err)
		logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

		t.Run("put object on OBC bucket", func(t *testing.T) {
			_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey1, contentType)
			assert.Nil(s.T(), poErr)
		})

		t.Run("get object on OBC bucket", func(t *testing.T) {
			read, err := s3client.GetObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(s.T(), err)
			assert.Equal(s.T(), ObjBody, read)
		})

		t.Run("test quota enforcement on OBC bucket", func(t *testing.T) {
			_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey2, contentType)
			assert.Nil(s.T(), poErr)
			logger.Infof("Testing the max object limit")
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
			assert.Error(s.T(), poErr)
		})

		t.Run("test update quota on OBC bucket", func(t *testing.T) {
			poErr := helper.BucketClient.UpdateObc(obcName, bucketStorageClassName, bucketname, newMaxObject, true)
			assert.Nil(s.T(), poErr)
			updated := utils.Retry(5, 2*time.Second, "OBC is updated", func() bool {
				return helper.BucketClient.CheckOBMaxObject(obcName, newMaxObject)
			})
			assert.True(s.T(), updated)
			logger.Infof("Testing the updated object limit")
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
			assert.NoError(s.T(), poErr)
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey4, contentType)
			assert.Error(s.T(), poErr)
		})

		t.Run("delete objects on OBC bucket", func(t *testing.T) {
			_, delobjErr := s3client.DeleteObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(s.T(), delobjErr)
			_, delobjErr = s3client.DeleteObjectInBucket(bucketname, ObjectKey2)
			assert.Nil(s.T(), delobjErr)
			_, delobjErr = s3client.DeleteObjectInBucket(bucketname, ObjectKey3)
			assert.Nil(s.T(), delobjErr)
			logger.Info("Objects deleted on bucket successfully")
		})
	})

	t.Run("Regression check: Verify bucket does not revert to Pending phase", func(t *testing.T) {
		// A bug exists in older versions of lib-bucket-provisioner that will revert a bucket and claim
		// back to "Pending" phase after being created and initially "Bound" by looping infinitely in
		// the bucket provision/creation loop. Verify that the OBC is "Bound" and stays that way.
		// The OBC reconcile loop runs again immediately b/c the OBC is modified to refer to its OB.
		// Wait a short amount of time before checking just to be safe.
		time.Sleep(15 * time.Second)
		assert.True(s.T(), helper.BucketClient.CheckOBC(obcName, "bound"))
	})

	t.Run("delete CephObjectStore should be blocked by OBC bucket and CephObjectStoreUser", func(t *testing.T) {
		err := k8sh.DeleteResourceAndWait(false, "-n", namespace, "CephObjectStore", storeName)
		assert.NoError(t, err)
		// wait initially for the controller to detect deletion. Almost always enough, but not
		// waiting will almost always fail the first check in the loop
		time.Sleep(2 * time.Second)

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
		assert.Nil(s.T(), dobcErr)
		logger.Info("Checking to see if the obc, secret and cm have all been deleted")
		for i = 0; i < 4 && !helper.BucketClient.CheckOBC(obcName, "deleted"); i++ {
			logger.Infof("(%d) obc deleted check, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(s.T(), 4, i)

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
		assert.NotEqual(s.T(), 4, i)
		assert.Equal(s.T(), rgwErr, rgw.RGWErrorNotFound)

		dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
		assert.Nil(s.T(), dobErr)
	})

	t.Run("delete CephObjectStoreUser", func(t *testing.T) {
		dosuErr := helper.ObjectUserClient.Delete(namespace, userid)
		assert.Nil(s.T(), dosuErr)
		logger.Info("Object store user deleted successfully")
		logger.Info("Checking to see if the user secret has been deleted")
		i := 0
		for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == true; i++ {
			logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.False(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))
	})

	t.Run("check that mgrs are not in a crashloop", func(t *testing.T) {
		assert.True(s.T(), k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
	})

	t.Run("CephObjectStore should delete now that dependents are gone", func(t *testing.T) {
		// wait initially since it will almost never detect on the first try without this.
		time.Sleep(3 * time.Second)

		store := &cephv1.CephObjectStore{}
		i := 0
		retry := 10
		for i = 0; i < retry; i++ {
			storeStr, err := k8sh.GetResource("-n", namespace, "CephObjectStore", storeName, "-o", "json")
			assert.NoError(t, err)
			logger.Infof("store: \n%s", storeStr)

			err = json.Unmarshal([]byte(storeStr), &store)
			assert.NoError(t, err)

			cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
			if cond.Status == v1.ConditionFalse && cond.Reason == cephv1.ObjectHasNoDependentsReason {
				// Let's give some time to the object to be updated
				time.Sleep(5 * time.Second)
				break
			}
			logger.Info("waiting 3 more seconds for CephObjectStore to be unblocked by dependents")
			time.Sleep(3 * time.Second)
		}
		assert.NotEqual(t, retry, i)

		assert.Equal(t, cephv1.ConditionDeleting, store.Status.Phase) // phase == "Deleting"
		// verify deletion is NOT blocked b/c object has dependents
		cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
		assert.Equal(t, v1.ConditionFalse, cond.Status)
		assert.Equal(t, cephv1.ObjectHasNoDependentsReason, cond.Reason)

		err := k8sh.WaitUntilResourceIsDeleted("CephObjectStore", namespace, storeName)
		assert.NoError(t, err)
	})

	// TODO : Add case for brownfield/cleanup s3 client}
}

func generateRgwTlsCertSecret(s suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName, rgwServiceName string) {
	ctx := context.TODO()
	root, err := utils.FindRookRoot()
	require.NoError(s.T(), err, "failed to get rook root")
	tlscertdir, err := ioutil.TempDir(root, "tlscertdir")
	require.NoError(s.T(), err, "failed to create directory for TLS certs")
	defer os.RemoveAll(tlscertdir)
	cmdArgs := utils.CommandArgs{Command: filepath.Join(root, "tests/scripts/generate-tls-config.sh"),
		CmdArgs: []string{tlscertdir, rgwServiceName, namespace}}
	cmdOut := utils.ExecuteCommand(cmdArgs)
	require.NoError(s.T(), cmdOut.Err)
	tlsKeyIn, err := ioutil.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".key"))
	require.NoError(s.T(), err)
	tlsCertIn, err := ioutil.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".crt"))
	require.NoError(s.T(), err)
	tlsCaCertIn, err := ioutil.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".ca"))
	require.NoError(s.T(), err)
	secretCertOut := fmt.Sprintf("%s%s%s", tlsKeyIn, tlsCertIn, tlsCaCertIn)
	tlsK8sSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"cert": []byte(secretCertOut),
		},
	}
	_, err = k8sh.Clientset.CoreV1().Secrets(namespace).Create(ctx, tlsK8sSecret, metav1.CreateOptions{})
	require.Nil(s.T(), err)
}
