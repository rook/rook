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
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/go-cmp/cmp"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/bucketowner"
	topickafka "github.com/rook/rook/tests/integration/object/topic/kafka"
	"github.com/rook/rook/tests/integration/object/user/userkeys"
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
	createCephObjectStore(s.T(), helper, k8sh, installer, namespace, storeName, 3, tlsEnable, swiftAndKeystone)

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

	bucketowner.TestObjectBucketClaimBucketOwner(s.T(), k8sh, installer, logger, tlsEnable)
	userkeys.TestObjectStoreUserKeys(s.T(), k8sh, installer, logger, tlsEnable)
	topickafka.TestBucketTopicKafka(s.T(), k8sh, installer, logger, tlsEnable)

	bucketNotificationTestStoreName := "bucket-notification-" + storeName
	createCephObjectStore(s.T(), helper, k8sh, installer, namespace, bucketNotificationTestStoreName, 1, tlsEnable, swiftAndKeystone)
	testBucketNotifications(s, helper, k8sh, namespace, bucketNotificationTestStoreName)
	if !tlsEnable {
		// TODO : need to fix COSI driver to support TLS
		logger.Info("Testing COSI driver")
		testCOSIDriver(s, helper, k8sh, installer, namespace)
	} else {
		logger.Info("Skipping COSI driver test as TLS is enabled")
	}
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

	t.Run("S3 access to OBC bucket", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		insecure := objectStore.Spec.IsTLSEnabled()
		s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil, insecure, nil)

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

		t.Run("user quota enforcement", func(t *testing.T) {
			_, poErr := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey2, contentType)
			assert.Nil(t, poErr)
			logger.Infof("Testing the max object limit")
			_, poErr = s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey3, contentType)
			assert.Error(t, poErr)
		})

		t.Run("update user quota limits", func(t *testing.T) {
			poErr := helper.BucketClient.UpdateObc(obcName, bucketStorageClassName, bucketname, newMaxObject, true)
			assert.Nil(t, poErr)
			updated := utils.Retry(20, 2*time.Second, "OBC is updated", func() bool {
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

	// this test deviates from the others in this package in that it does not
	// rely on kubectl and uses the k8s api directly
	t.Run("OBC bucket quota enforcement", func(t *testing.T) {
		bucketName := "bucket-quota-test"
		var obName string
		var s3client *rgw.S3Agent

		t.Run("create obc with bucketMaxObjects", func(t *testing.T) {
			newObc := v1alpha1.ObjectBucketClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bucketName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ObjectBucketClaimSpec{
					BucketName:       bucketName,
					StorageClassName: bucketStorageClassName,
					AdditionalConfig: map[string]string{
						"bucketMaxObjects": "1",
					},
				},
			}
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Create(ctx, &newObc, metav1.CreateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, 2*time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, 2*time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketMaxObjects is set
			assert.True(t, obc.Spec.AdditionalConfig["bucketMaxObjects"] == "1")

			// as the tests are running external to k8s, the internal svc can't be used
			labelSelector := "rgw=" + storeName
			services, err := k8sh.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			assert.Nil(t, err)
			assert.Equal(t, 1, len(services.Items))
			s3endpoint := services.Items[0].Spec.ClusterIP + ":80"

			secret, err := k8sh.Clientset.CoreV1().Secrets(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			s3AccessKey := string(secret.Data["AWS_ACCESS_KEY_ID"])
			s3SecretKey := string(secret.Data["AWS_SECRET_ACCESS_KEY"])

			insecure := objectStore.Spec.IsTLSEnabled()
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil, insecure, nil)
			assert.Nil(t, err)
			logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)
		})

		t.Run("bucketMaxObjects quota is enforced", func(t *testing.T) {
			// first object should succeed
			_, err := s3client.PutObjectInBucket(bucketName, ObjBody, ObjectKey1, contentType)
			assert.Nil(t, err)
			// second object should fail as bucket quota is 1
			_, err = s3client.PutObjectInBucket(bucketName, ObjBody, ObjectKey2, contentType)
			assert.Error(t, err)
			// cleanup bucket
			_, err = s3client.DeleteObjectInBucket(bucketName, ObjectKey1)
			assert.Nil(t, err)
			_, err = s3client.DeleteObjectInBucket(bucketName, ObjectKey2)
			assert.Nil(t, err)
		})

		t.Run("change quota to bucketMaxSize", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)

			obc.Spec.AdditionalConfig = map[string]string{"bucketMaxSize": "4Ki"}

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, 2*time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, 2*time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketMaxSize is set
			assert.True(t, obc.Spec.AdditionalConfig["bucketMaxSize"] == "4Ki")
		})

		t.Run("bucketMaxSize quota is enforced", func(t *testing.T) {
			// first ~3KiB Object should succeed
			_, err := s3client.PutObjectInBucket(bucketName, strings.Repeat("1", 3072), ObjectKey1, contentType)
			assert.Nil(t, err)
			// second ~2KiB Object should fail as bucket quota is is 4KiB
			_, err = s3client.PutObjectInBucket(bucketName, strings.Repeat("2", 2048), ObjectKey2, contentType)
			assert.Error(t, err)
			// cleanup bucket
			_, err = s3client.DeleteObjectInBucket(bucketName, ObjectKey1)
			assert.Nil(t, err)
			_, err = s3client.DeleteObjectInBucket(bucketName, ObjectKey2)
			assert.Nil(t, err)
		})

		t.Run("delete bucket", func(t *testing.T) {
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Delete(ctx, bucketName, metav1.DeleteOptions{})
			assert.Nil(t, err)

			absent := utils.Retry(20, 2*time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(20, 2*time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})
	})

	t.Run("OBC bucket policy management", func(t *testing.T) {
		bucketName := "bucket-policy-test"
		var obName string
		var s3client *rgw.S3Agent
		bucketPolicy1 := `
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "AWS": "arn:aws:iam:::user/foo"
            },
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:ListBucket",
                "s3:GetBucketLocation"
            ],
            "Resource": [
                "arn:aws:s3:::bucket-policy-test/*"
            ]
        }
    ]
}
		`

		bucketPolicy2 := `
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "AWS": "arn:aws:iam:::user/bar"
            },
            "Action": [
                "s3:GetObject",
                "s3:ListBucket",
                "s3:GetBucketLocation"
            ],
            "Resource": [
                "arn:aws:s3:::bucket-policy-test/*"
            ]
        }
    ]
}
		`

		t.Run("create obc with bucketPolicy", func(t *testing.T) {
			newObc := v1alpha1.ObjectBucketClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bucketName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ObjectBucketClaimSpec{
					BucketName:       bucketName,
					StorageClassName: bucketStorageClassName,
					AdditionalConfig: map[string]string{
						"bucketPolicy": bucketPolicy1,
					},
				},
			}
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Create(ctx, &newObc, metav1.CreateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, 2*time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, 2*time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketPolicy is set
			assert.Equal(t, bucketPolicy1, obc.Spec.AdditionalConfig["bucketPolicy"])

			// as the tests are running external to k8s, the internal svc can't be used
			labelSelector := "rgw=" + storeName
			services, err := k8sh.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			assert.Nil(t, err)
			assert.Equal(t, 1, len(services.Items))
			s3endpoint := services.Items[0].Spec.ClusterIP + ":80"

			secret, err := k8sh.Clientset.CoreV1().Secrets(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			s3AccessKey := string(secret.Data["AWS_ACCESS_KEY_ID"])
			s3SecretKey := string(secret.Data["AWS_SECRET_ACCESS_KEY"])

			insecure := objectStore.Spec.IsTLSEnabled()
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil, insecure, nil)
			assert.Nil(t, err)
			logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)
		})

		t.Run("policy was applied verbatim to bucket", func(t *testing.T) {
			policyResp, err := s3client.Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
				Bucket: &bucketName,
			})
			require.NoError(t, err)
			assert.Equal(t, bucketPolicy1, *policyResp.Policy)
		})

		t.Run("update obc bucketPolicy", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)

			obc.Spec.AdditionalConfig["bucketPolicy"] = bucketPolicy2

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that updated bucketPolicy is set
			assert.Equal(t, bucketPolicy2, obc.Spec.AdditionalConfig["bucketPolicy"])
		})

		t.Run("policy update applied verbatim to bucket", func(t *testing.T) {
			var livePolicy string
			utils.Retry(20, time.Second, "policy changed", func() bool {
				policyResp, err := s3client.Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
					Bucket: &bucketName,
				})
				if err != nil {
					return false
				}

				livePolicy = *policyResp.Policy
				return bucketPolicy2 == livePolicy
			})
			assert.Equal(t, bucketPolicy2, livePolicy)
		})

		t.Run("remove obc bucketPolicy", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)

			obc.Spec.AdditionalConfig = map[string]string{}

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketPolicy is unset
			assert.NotContains(t, obc.Spec.AdditionalConfig, "bucketPolicy")
		})

		t.Run("policy was removed from bucket", func(t *testing.T) {
			var err error
			utils.Retry(20, time.Second, "policy is gone", func() bool {
				_, err = s3client.Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
					Bucket: &bucketName,
				})
				return err != nil
			})
			require.Error(t, err)
			require.Implements(t, (*awserr.Error)(nil), err)
			aerr, _ := err.(awserr.Error)
			assert.Equal(t, aerr.Code(), "NoSuchBucketPolicy")
		})

		t.Run("delete bucket", func(t *testing.T) {
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Delete(ctx, bucketName, metav1.DeleteOptions{})
			assert.Nil(t, err)

			absent := utils.Retry(20, time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(20, time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})
	})

	t.Run("OBC bucket lifecycle management", func(t *testing.T) {
		bucketName := "bucket-lifecycle-test"
		var obName string
		var s3client *rgw.S3Agent
		bucketLifecycle1 := `
{
  "Rules":[
    {
      "ID": "AbortIncompleteMultipartUploads",
      "Status": "Enabled",
      "Prefix": "",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 1
      }
    }
  ]
}
		`

		// rules must be sorted by ID to be idempotent
		bucketLifecycle2 := `
{
  "Rules": [
    {
      "ID": "AbortIncompleteMultipartUploads",
      "Status": "Enabled",
      "Prefix": "",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 1
      }
    },
    {
      "ID": "ExpireAfter30Days",
      "Status": "Enabled",
      "Prefix": "",
      "Expiration": {
        "Days": 30
      }
    }
  ]
}
		`

		t.Run("create obc with bucketLifecycle", func(t *testing.T) {
			newObc := v1alpha1.ObjectBucketClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bucketName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ObjectBucketClaimSpec{
					BucketName:       bucketName,
					StorageClassName: bucketStorageClassName,
					AdditionalConfig: map[string]string{
						"bucketLifecycle": bucketLifecycle1,
					},
				},
			}
			_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Create(ctx, &newObc, metav1.CreateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, 2*time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, 2*time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketLifecycle is set
			assert.Equal(t, bucketLifecycle1, obc.Spec.AdditionalConfig["bucketLifecycle"])

			// as the tests are running external to k8s, the internal svc can't be used
			labelSelector := "rgw=" + storeName
			services, err := k8sh.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			assert.Nil(t, err)
			assert.Equal(t, 1, len(services.Items))
			s3endpoint := services.Items[0].Spec.ClusterIP + ":80"

			secret, err := k8sh.Clientset.CoreV1().Secrets(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			s3AccessKey := string(secret.Data["AWS_ACCESS_KEY_ID"])
			s3SecretKey := string(secret.Data["AWS_SECRET_ACCESS_KEY"])

			insecure := objectStore.Spec.IsTLSEnabled()
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil, insecure, nil)
			assert.Nil(t, err)
			logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)
		})

		t.Run("lifecycle was applied verbatim to bucket", func(t *testing.T) {
			liveLc, err := s3client.Client.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
				Bucket: &bucketName,
			})
			require.NoError(t, err)

			confLc := &s3.GetBucketLifecycleConfigurationOutput{}
			err = json.Unmarshal([]byte(bucketLifecycle1), confLc)
			require.NoError(t, err)

			assert.Equal(t, confLc, liveLc)
		})

		t.Run("update obc bucketLifecycle", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)

			obc.Spec.AdditionalConfig["bucketLifecycle"] = bucketLifecycle2

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that updated bucketLifecycle is set
			assert.Equal(t, bucketLifecycle2, obc.Spec.AdditionalConfig["bucketLifecycle"])
		})

		t.Run("lifecycle update applied verbatim to bucket", func(t *testing.T) {
			var liveLc *s3.GetBucketLifecycleConfigurationOutput

			confLc := &s3.GetBucketLifecycleConfigurationOutput{}
			err = json.Unmarshal([]byte(bucketLifecycle2), confLc)
			require.NoError(t, err)

			utils.Retry(20, time.Second, "lifecycle changed", func() bool {
				liveLc, err = s3client.Client.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
					Bucket: &bucketName,
				})
				if err != nil {
					return false
				}

				return cmp.Equal(confLc, liveLc)
			})
			assert.Equal(t, confLc, liveLc)
		})

		t.Run("remove obc bucketLifecycle", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)

			obc.Spec.AdditionalConfig = map[string]string{}

			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
			obcBound := utils.Retry(20, time.Second, "OBC is Bound", func() bool {
				liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveObc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound
			})
			assert.True(t, obcBound)

			// wait until obc is Bound to lookup the ob name
			obc, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
			assert.Nil(t, err)
			obName = obc.Spec.ObjectBucketName

			obBound := utils.Retry(20, time.Second, "OB is Bound", func() bool {
				liveOb, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				if err != nil {
					return false
				}

				return liveOb.Status.Phase == v1alpha1.ObjectBucketStatusPhaseBound
			})
			assert.True(t, obBound)

			// verify that bucketLifecycle is unset
			assert.NotContains(t, obc.Spec.AdditionalConfig, "bucketLifecycle")
		})

		t.Run("lifecycle was removed from bucket", func(t *testing.T) {
			cephcluster, err := k8sh.RookClientset.CephV1().CephClusters(namespace).Get(ctx, settings.ClusterName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("failed to get CephCluster: %v", err)
			}
			logger.Infof("CephCluster version: %s", cephcluster.Status.CephVersion.Version)
			if strings.HasPrefix(cephcluster.Status.CephVersion.Version, "19.2.3") {
				t.Skip("Waiting for rgw fix from regression in v19.2.3")
			}
			utils.Retry(20, time.Second, "lifecycle is gone", func() bool {
				_, err = s3client.Client.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
					Bucket: &bucketName,
				})
				if aerr, ok := err.(awserr.Error); ok {
					return aerr.Code() == "NoSuchLifecycleConfiguration"
				}
				return false
			})
			require.Error(t, err)
			require.Implements(t, (*awserr.Error)(nil), err)
			aerr, _ := err.(awserr.Error)
			assert.Equal(t, aerr.Code(), "NoSuchLifecycleConfiguration")
		})

		t.Run("delete bucket", func(t *testing.T) {
			err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Delete(ctx, bucketName, metav1.DeleteOptions{})
			assert.Nil(t, err)

			absent := utils.Retry(20, time.Second, "OBC is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, bucketName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)

			absent = utils.Retry(20, time.Second, "OB is absent", func() bool {
				_, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets().Get(ctx, obName, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})
	})

	t.Run("Regression check: OBC does not revert to Pending phase", func(t *testing.T) {
		// A bug exists in older versions of lib-bucket-provisioner that will revert a bucket and claim
		// back to "Pending" phase after being created and initially "Bound" by looping infinitely in
		// the bucket provision/creation loop. Verify that the OBC is "Bound" and stays that way.
		// The OBC reconcile loop runs again immediately b/c the OBC is modified to refer to its OB.
		// Wait a short amount of time before checking just to be safe.
		created := utils.Retry(15, 2*time.Second, "OBC is created", func() bool {
			return helper.BucketClient.CheckOBC(obcName, "bound")
		})
		assert.True(t, created)
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
