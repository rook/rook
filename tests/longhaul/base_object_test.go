package longhaul

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/icrowley/fake"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())

}

// Set up rook cluster if necessary
// create object store if necessary
// create object store user if necessary
// returns k8s helper, install helper and s3 helper for the object store that was just created.
func setUpRook(t func() *testing.T, namespace string) (*utils.K8sHelper, *installer.InstallHelper, *clients.TestClient) {
	kh, err := utils.CreateK8sHelper(t)
	assert.Nil(t(), err)

	i := installer.NewK8sRookhelper(kh.Clientset, t)
	if !kh.IsRookInstalled(namespace) {
		isRookInstalled, err := i.InstallRookOnK8sWithHostPathAndDevices(namespace, "bluestore", "/temp/rookBackup", true, 3)
		require.NoError(t(), err)
		require.True(t(), isRookInstalled)
	}
	helper, err := clients.CreateTestClient(kh, namespace)
	if err != nil {
		logger.Errorf("Cannot create rook test client, er -> %v", err)
		t().FailNow()
	}

	return kh, i, helper
}

func createObjectStoreAndUser(t func() *testing.T, kh *utils.K8sHelper, tc *clients.TestClient, namespace string, storeName string, userId string, userName string) *utils.S3Helper {
	if !isObjectStorePresent(kh, namespace, storeName) {
		dnsName := fmt.Sprintf("%s.%s", storeName, namespace)
		tc.GetObjectClient().ObjectCreate(namespace, storeName, 3, dnsName, false, kh)
	}

	ou, err := tc.GetObjectClient().ObjectGetUser(storeName, userId)
	if err != nil || *ou.DisplayName != userName {
		tc.GetObjectClient().ObjectCreateUser(storeName, userId, userName)
	}

	conninfo, conninfoError := tc.GetObjectClient().ObjectGetUser(storeName, userId)
	require.Nil(t(), conninfoError)
	s3endpoint, _ := kh.GetRGWServiceURL(storeName, namespace)
	s3client := utils.CreateNewS3Helper(s3endpoint, *conninfo.AccessKey, *conninfo.SecretKey)

	return s3client

}

func isObjectStorePresent(kh *utils.K8sHelper, namespace string, storeName string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=rook-ceph-rgw"}
	podList, err := kh.Clientset.Pods(namespace).List(listOpts)
	if err == nil {
		for _, pod := range podList.Items {
			lables := pod.GetObjectMeta().GetLabels()
			if lables["rook_object_store"] == storeName {
				return true
			}
		}
	}

	return false
}

func performObjectStoreOperations(installer *installer.InstallHelper, s3 *utils.S3Helper, bucketName string) {
	var wg sync.WaitGroup
	for i := 1; i <= installer.Env.LoadConcurrentRuns; i++ {
		wg.Add(1)
		go bucketOperations(s3, bucketName, &wg, installer.Env.LoadTime)
	}
	wg.Wait()
}

func bucketOperations(s3 *utils.S3Helper, bucketName string, wg *sync.WaitGroup, runtime int) {
	defer wg.Done()
	start := time.Now()
	elapsed := time.Since(start).Seconds()
	for elapsed < float64(runtime) {
		key1 := fake.CharactersN(30)
		key2 := fake.CharactersN(30)
		key3 := fake.CharactersN(30)
		key4 := fake.CharactersN(30)
		s3.PutObjectInBucket(bucketName, fake.CharactersN(200), key1, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(200), key2, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(200), key3, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(200), key4, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(200), key1, "plain/text")
		s3.GetObjectInBucket(bucketName, key1)
		s3.GetObjectInBucket(bucketName, key2)
		s3.GetObjectInBucket(bucketName, key3)
		s3.DeleteObjectInBucket(bucketName, key4)
		elapsed = time.Since(start).Seconds()
	}

}

func randomBool() bool {
	return rand.Intn(2) == 0
}
