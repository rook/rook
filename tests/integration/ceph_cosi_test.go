package integration

import (
	"context"
	"testing"
	"time"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	objectStoreCOSI = "cosi-store"
	cosiUser        = "cosi"
	bucketClassName = "cosi-bucketclass"
	deletionPolicy  = "Delete"
	cosiBucketName  = "cosi-bucket"
)

func testCOSIDriver(s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, cephinstaller *installer.CephInstaller, namespace string) {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("ceph cosi driver tests skipped on openshift")
	}

	t := s.T()

	t.Run("COSI Controller and CRD installation", func(t *testing.T) {
		_, err := k8sh.Kubectl("create", "-k", "github.com/kubernetes-sigs/container-object-storage-interface-api")
		assert.NoError(t, err, "failed to create COSI CRDs")
		_, err = k8sh.Kubectl("create", "-k", "github.com/kubernetes-sigs/container-object-storage-interface-controller")
		assert.NoError(t, err, "failed to create COSI controller")
	})

	createCephObjectStore(s.T(), helper, k8sh, cephinstaller, namespace, objectStoreCOSI, 1, false, false)

	t.Run("Creating CephCOSIDriver CRD", func(t *testing.T) {
		err := helper.COSIClient.CreateCOSI()
		assert.NoError(t, err, "failed to create Ceph COSI Driver CRD")
	})

	operatorNamespace := cephinstaller.Manifests.Settings().OperatorNamespace
	t.Run("Check ceph cosi driver running", func(t *testing.T) {
		for i := 24; i < 24 && k8sh.CheckPodCountAndState("ceph-cosi-driver", operatorNamespace, 1, "Running") == false; i++ {
			logger.Infof("ceph-cosi-driver is not running, trying again")
			k8sh.CheckPodCountAndState("ceph-cosi-driver", namespace, 1, "Running")
		}
		assert.True(t, k8sh.CheckPodCountAndState("ceph-cosi-driver", operatorNamespace, 1, "Running"))
		assert.NoError(t, k8sh.WaitForLabeledDeploymentsToBeReady("app=ceph-cosi-driver", operatorNamespace))
	})

	createCephObjectUser(s, helper, k8sh, namespace, objectStoreCOSI, cosiUser, true)
	objectStoreUserSecretName := "rook-ceph-object-user" + "-" + objectStoreCOSI + "-" + cosiUser
	t.Run("Creating BucketClass", func(t *testing.T) {
		err := helper.COSIClient.CreateBucketClass(bucketClassName, objectStoreUserSecretName, deletionPolicy)
		assert.NoError(t, err, "failed to create BucketClass")
	})

	t.Run("Creating BucketClaim", func(t *testing.T) {
		err := helper.COSIClient.CreateBucketClaim(cosiBucketName, bucketClassName)
		assert.NoError(t, err, "failed to create BucketClaim")
	})

	var cosiBucket string
	t.Run("Check Bucket is ready", func(t *testing.T) {
		_, err := k8sh.GetResource("bucketclaim", "-n", operatorNamespace, cosiBucketName)
		assert.NoError(t, err, "failed to get BucketClaim")
		cosiBucket, err = k8sh.GetResource("bucketclaim", "-n", operatorNamespace, cosiBucketName, "-o", "jsonpath={.status.bucketName}")
		assert.NoError(t, err, "failed to get Bucket name")
		i := 0
		var bucketReady string
		for i = 0; i < 4; i++ {
			bucketReady, err = k8sh.GetResource("bucket", cosiBucket, "-o", "jsonpath={.status.bucketReady}")
			if bucketReady == "true" && err == nil {
				break
			}
			logger.Warningf("bucket %q is not ready, retrying... bucketReady=%q, err=%v", cosiBucket, bucketReady, err)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, "true", bucketReady, "Bucket is not ready")
	})

	t.Run("Check Bucket is created in Backend Ceph", func(t *testing.T) {
		ctx := context.TODO()
		// check if bucket is created in the backend
		context := k8sh.MakeContext()
		clusterInfo := client.AdminTestClusterInfo(namespace)
		objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, objectStoreCOSI, metav1.GetOptions{})
		assert.Nil(t, err)
		rgwcontext, err := rgw.NewMultisiteContext(context, clusterInfo, objectStore)
		assert.Nil(t, err)
		var bkt rgw.ObjectBucket
		i := 0
		for i = 0; i < 4; i++ {
			b, code, err := rgw.GetBucket(rgwcontext, cosiBucket)
			if b != nil && err == nil {
				bkt = *b
				break
			}
			logger.Warningf("cannot get bucket %q, retrying... bucket: %v. code: %d, err: %v", cosiBucket, b, code, err)
			logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, cosiBucket, bkt.Name)
	})

	t.Run("Deleting BucketClaim", func(t *testing.T) {
		err := helper.COSIClient.DeleteBucketClaim(cosiBucketName, bucketClassName)
		assert.NoError(t, err, "failed to delete BucketClaim")
	})

	t.Run("Deleting BucketClass", func(t *testing.T) {
		err := helper.COSIClient.DeleteBucketClass(bucketClassName, objectStoreUserSecretName, deletionPolicy)
		assert.NoError(t, err, "failed to delete BucketClass")
	})

	t.Run("Deleting object user for cosi", func(t *testing.T) {
		err := helper.ObjectUserClient.Delete(namespace, cosiUser)
		assert.NoError(t, err, "failed to delete cosi user")
	})

	t.Run("delete CephObjectStore", func(t *testing.T) {
		deleteObjectStore(t, k8sh, namespace, objectStoreCOSI)
		assertObjectStoreDeletion(t, k8sh, namespace, objectStoreCOSI)
	})

	t.Run("delete CephCOSIDriver CRD", func(t *testing.T) {
		err := helper.COSIClient.DeleteCOSI()
		assert.NoError(t, err, "failed to delete Ceph COSI Driver CRD")
	})

	t.Run("delete COSI Controller", func(t *testing.T) {
		_, err := k8sh.Kubectl("delete", "-k", "github.com/kubernetes-sigs/container-object-storage-interface-api")
		assert.NoError(t, err, "failed to create COSI CRDs")
		_, err = k8sh.Kubectl("delete", "-k", "github.com/kubernetes-sigs/container-object-storage-interface-controller")
		assert.NoError(t, err, "failed to create COSI controller")
	})
}
