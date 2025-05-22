/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	rgwPrefix = "rook-ceph-rgw"
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the secret name
	objectTLSSecretName = rgwPrefix + "-tls-test-store-csr"
)

var (
	userid                 = "rook-user"
	userdisplayname        = "A rook RGW user"
	bucketname             = "smokebkt"
	ObjBody                = "Test Rook Object Data"
	ObjectKey1             = "rookObj1"
	ObjectKey2             = "rookObj2"
	ObjectKey3             = "rookObj3"
	ObjectKey4             = "rookObj4"
	contentType            = "plain/text"
	obcName                = "smoke-delete-bucket"
	maxObject              = "2"
	newMaxObject           = "3"
	bucketStorageClassName = "rook-smoke-delete-bucket"
	maxBucket              = 1
	maxSize                = "100000"
	userCap                = "*"
)

// Test Object StoreCreation on Rook that was installed via helm
func runObjectE2ETestLite(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace, storeName string, replicaSize int, deleteStore bool, enableTLS bool, swiftAndKeystone bool) {
	andDeleting := ""
	if deleteStore {
		andDeleting = "and deleting"
	}
	logger.Infof("test creating %s object store %q in namespace %q", andDeleting, storeName, namespace)

	createCephObjectStore(t, helper, k8sh, installer, namespace, storeName, replicaSize, enableTLS, swiftAndKeystone)

	if deleteStore {
		t.Run("delete object store", func(t *testing.T) {
			deleteObjectStore(t, k8sh, namespace, storeName)
			assertObjectStoreDeletion(t, k8sh, namespace, storeName)
		})
		// remove user secret
	}
}

func RgwServiceName(storeName string) string {
	return rgwPrefix + "-" + storeName
}

// create a CephObjectStore and wait for it to report ready status
func createCephObjectStore(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace, storeName string, replicaSize int, tlsEnable bool, swiftAndKeystone bool) {
	logger.Infof("Create Object Store %q with replica count %d", storeName, replicaSize)
	if tlsEnable {
		t.Run("generate TLS certs", func(t *testing.T) {
			generateRgwTlsCertSecret(t, helper, k8sh, namespace, storeName, RgwServiceName(storeName))
		})
	}
	t.Run("create CephObjectStore", func(t *testing.T) {
		// nolint:gosec // G115 no overflow in test
		err := helper.ObjectClient.Create(namespace, storeName, int32(replicaSize), tlsEnable, swiftAndKeystone)
		assert.Nil(t, err)
	})

	t.Run("wait for RGWs to be running", func(t *testing.T) {
		// check that ObjectStore is created
		logger.Infof("Check that RGW pods are Running")
		for i := 0; i < 24 && k8sh.CheckPodCountAndState(rgwPrefix, namespace, 1, "Running") == false; i++ {
			logger.Infof("(%d) RGW pod check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.True(t, k8sh.CheckPodCountAndState(rgwPrefix, namespace, replicaSize, "Running"))
		logger.Info("RGW pods are running")
		assert.NoError(t, k8sh.WaitForLabeledDeploymentsToBeReady("app="+rgwPrefix, namespace))
		logger.Infof("Object store %q created successfully", storeName)
	})

	ctx := context.TODO()

	// Check object store status
	t.Run("verify object store status", func(t *testing.T) {
		retryCount := 40
		i := 0
		for i = 0; i < retryCount; i++ {
			objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
			assert.Nil(t, err)
			// TODO: check that object store status is good, and also check that status of
			// deployment is good based on health checks

			if objectStore.Status == nil {
				logger.Infof("(%d) object status check sleeping for 5 seconds ...%+v", i, objectStore.Status)
				time.Sleep(5 * time.Second)
				continue
			}
			logger.Info("objectstore status is", objectStore.Status)
			// ConditionConnected supports Rook v1.10 clusters that still had health check
			// TODO: remove that half of check after Rook v1.12 release
			if objectStore.Status.Phase != cephv1.ConditionReady && objectStore.Status.Phase != cephv1.ConditionConnected {
				logger.Infof("(%d) bucket status check sleeping for 5 seconds ...%+v", i, objectStore.Status.Phase)
				time.Sleep(5 * time.Second)
				continue
			}

			// Info field has the endpoint in it
			assert.NotEmpty(t, objectStore.Status.Info)
			assert.NotEmpty(t, objectStore.Status.Info["endpoint"])
			break
		}
		if i == retryCount {
			t.Fatal("bucket status check failed. status is not connected")
		}
	})

	t.Run("verify RGW liveness probes show healthy", func(t *testing.T) {
		err := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 90*time.Second, true, func(ctx context.Context) (done bool, err error) {
			deployName := RgwServiceName(storeName) + "-a"
			d, err := k8sh.Clientset.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			if err != nil {
				logger.Infof("waiting for rgw deployment %q to be ready; failed to get deployment: %v", deployName, err)
				return false, nil
			}
			if d.Status.UnavailableReplicas != 0 {
				logger.Infof("waiting rgw deployment %q to be ready; %d replicas are unavailable", deployName, d.Status.UnavailableReplicas)
				return false, nil
			}
			return true, nil
		})
		assert.NoError(t, err)
	})

	t.Run("verify RGW service is up", func(t *testing.T) {
		assert.True(t, k8sh.IsServiceUp(RgwServiceName(storeName), namespace))
	})

	t.Run("check if the dashboard-admin user exists in all existing object stores", func(t *testing.T) {
		objectStores, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).List(ctx, metav1.ListOptions{})
		assert.Nil(t, err)

		for _, objectStore := range objectStores.Items {
			output, err := installer.Execute("radosgw-admin", []string{"user", "info", "--uid=dashboard-admin", fmt.Sprintf("--rgw-realm=%s", objectStore.GetName())}, namespace)
			logger.Infof("output: %s", output)
			if err != nil {
				// Just log the error until we get a more reliable way to wait for the user to be created
				logger.Errorf("failed to get dashboard-admin from object store %s. %+v", objectStore.GetName(), err)
			}
		}
	})
}

func deleteObjectStore(t *testing.T, k8sh *utils.K8sHelper, namespace, storeName string) {
	err := k8sh.DeleteResourceAndWait(false, "-n", namespace, "CephObjectStore", storeName)
	assert.NoError(t, err)
	// wait initially for the controller to detect deletion. Almost always enough, but not
	// waiting immediately after this will almost always fail the first check in the loop
	time.Sleep(10 * time.Second)
}

func assertObjectStoreDeletion(t *testing.T, k8sh *utils.K8sHelper, namespace, storeName string) {
	store := &cephv1.CephObjectStore{}
	i := 0
	retry := 10
	sleepTime := 3 * time.Second
	for i = 0; i < retry; i++ {
		storeStr, err := k8sh.GetResource("-n", namespace, "CephObjectStore", storeName, "-o", "json")
		// if cephobjectstore is not found, just return the test
		// no need to check deletion phases as it is already deleted
		if err != nil && strings.Contains(storeStr, errors.NewNotFound(v1.Resource("cephobjectstores.ceph.rook.io"), storeName).ErrStatus.Message) {
			return
		}

		assert.NoError(t, err)
		logger.Infof("store: \n%s", storeStr)

		err = json.Unmarshal([]byte(storeStr), &store)
		assert.NoError(t, err)

		cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
		if cond == nil {
			logger.Infof("waiting for CephObjectStore %q to have a deletion condition", storeName)
			time.Sleep(sleepTime)
			continue
		}
		if cond.Status == v1.ConditionFalse && cond.Reason == cephv1.ObjectHasNoDependentsReason {
			// no longer blocked by dependents
			time.Sleep(5 * time.Second) // Let's give some time to the object to be updated
			break
		}
		logger.Infof("waiting 3 more seconds for CephObjectStore %q to be unblocked by dependents", storeName)
		time.Sleep(sleepTime)
	}
	assert.NotEqual(t, retry, i)
	assert.Equal(t, cephv1.ConditionDeleting, store.Status.Phase) // phase == "Deleting"
	// verify deletion is NOT blocked b/c object has dependents
	cond := cephv1.FindStatusCondition(store.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
	assert.Equal(t, v1.ConditionFalse, cond.Status)
	assert.Equal(t, cephv1.ObjectHasNoDependentsReason, cond.Reason)

	err := k8sh.WaitUntilResourceIsDeleted("CephObjectStore", namespace, storeName)
	assert.NoError(t, err)
}

func createCephObjectUser(
	s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string, checkQuotaAndCaps bool,
) {
	maxObjectInt, err := strconv.Atoi(maxObject)
	assert.Nil(s.T(), err)
	logger.Infof("creating CephObjectStore user %q for store %q in namespace %q", userID, storeName, namespace)
	cosuErr := helper.ObjectUserClient.Create(userID, userdisplayname, storeName, userCap, maxSize, maxBucket, maxObjectInt)
	assert.Nil(s.T(), cosuErr)
	logger.Infof("Waiting 5 seconds for the object user %q to be created", userID)
	time.Sleep(5 * time.Second)
	logger.Infof("Checking to see if user %q secret has been created", userID)
	for i := 0; i < 6 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID) == false; i++ {
		logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}

	checkCephObjectUser(s, helper, k8sh, namespace, storeName, userID, checkQuotaAndCaps)
}

func checkCephObjectUser(
	s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper,
	namespace, storeName, userID string, checkQuotaAndCaps bool,
) {
	logger.Infof("checking object store \"%s/%s\" user %q", namespace, storeName, userID)
	assert.True(s.T(), helper.ObjectUserClient.UserSecretExists(namespace, storeName, userID))

	userInfo, err := helper.ObjectUserClient.GetUser(namespace, storeName, userID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), userID, userInfo.UserID)
	assert.Equal(s.T(), userdisplayname, *userInfo.DisplayName)

	phase, err := k8sh.GetResource("--namespace", namespace, "cephobjectstoreuser", userID, "--output", "jsonpath={.status.phase}")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), k8sutil.ReadyStatus, phase)
}

func objectStoreCleanUp(s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	logger.Infof("Delete Object Store (will fail if users and buckets still exist)")
	err := helper.ObjectClient.Delete(namespace, storeName)
	assert.Nil(s.T(), err)
	logger.Infof("Done deleting object store")
}

func generateRgwTlsCertSecret(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName, rgwServiceName string) {
	ctx := context.TODO()
	root, err := utils.FindRookRoot()
	require.NoError(t, err, "failed to get rook root")
	tlscertdir := t.TempDir()
	cmdArgs := utils.CommandArgs{
		Command: filepath.Join(root, "tests/scripts/generate-tls-config.sh"),
		CmdArgs: []string{tlscertdir, rgwServiceName, namespace},
	}
	cmdOut := utils.ExecuteCommand(cmdArgs)
	require.NoError(t, cmdOut.Err)
	tlsKeyIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".key"))
	require.NoError(t, err)
	tlsCertIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".crt"))
	require.NoError(t, err)
	tlsCaCertIn, err := os.ReadFile(filepath.Join(tlscertdir, rgwServiceName+".ca"))
	require.NoError(t, err)
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
	require.Nil(t, err)
}
