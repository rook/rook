package longhaul

import (
	"strconv"
	"sync"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Rook Object Store longhaul test
// Start Rook, create an object store and object user
// Create object store bucket and perform CURD operations.
// This Test creates 1 to n object stores - first object store is present throughout the run,
// but all other object stores are deleted after each run.
//NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
//to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test) and
//--load_volumes(number of volumes that are created per test
func TestObjectLongHaul(t *testing.T) {
	suite.Run(t, new(ObjectLongHaulSuite))
}

type ObjectLongHaulSuite struct {
	suite.Suite
	kh        *utils.K8sHelper
	installer *installer.InstallHelper
	tc        *clients.TestClient
	op        contracts.Setup
}

func (s *ObjectLongHaulSuite) SetupSuite() {
	var err error
	s.op, s.kh, s.installer = NewBaseLoadTestOperations(s.T, "longhaul-ns")
	s.tc, err = clients.CreateTestClient(s.kh, "longhaul-ns")
	require.Nil(s.T(), err)

}

func (s *ObjectLongHaulSuite) TestObjectLonghaulRun() {
	var wg sync.WaitGroup
	storeName := "longhaulstore"
	wg.Add(s.installer.Env.LoadVolumeNumber)
	for i := 1; i <= s.installer.Env.LoadVolumeNumber; i++ {
		if i == 1 {
			go ObjectStoreOperations(s, &wg, "longhaul-ns", storeName+strconv.Itoa(i), false)
		} else {
			go ObjectStoreOperations(s, &wg, "longhaul-ns", storeName+strconv.Itoa(i), randomBool())
		}

	}
	wg.Wait()
}

func ObjectStoreOperations(s *ObjectLongHaulSuite, wg *sync.WaitGroup, namespace string, storeName string, deleteStore bool) {
	defer wg.Done()
	bucketName := "loadbucket"
	s3 := createObjectStoreAndUser(s.T, s.kh, s.tc, "longhaul-ns", storeName, "longhaul", "LongHaulTest")
	isFound, err := s3.IsBucketPresent(bucketName)
	if err == nil {
		if !isFound {
			s3.CreateBucket(bucketName)
		}
	} else {
		logger.Infof("Error which check if %s bucket exists, err -> %v", bucketName, err)
		s.T().Fail()
	}

	performObjectStoreOperations(s.installer, s3, bucketName)
	if deleteStore {
		delOpts := metav1.DeleteOptions{}
		s.tc.GetObjectClient().ObjectDelete(namespace, storeName, 3, false, s.kh)
		s.kh.Clientset.CoreV1().Services(namespace).Delete("rgw-external-"+storeName, &delOpts)
	}
	s3 = nil
}

func (s *ObjectLongHaulSuite) TearDownSuite() {
	s.tc = nil
	s.kh = nil
	s = nil
}
