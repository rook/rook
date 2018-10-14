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

package longhaul

import (
	"strconv"
	"sync"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Rook Object Store longhaul test
// Start Rook, create an object store and object user
// Create object store bucket and perform CURD operations.
// This Test creates 1 to n object stores - first object store is present throughout the run,
// but all other object stores are deleted after each run.
// NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
// to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test) and
//--load_volumes(number of volumes that are created per test
func TestObjectLongHaul(t *testing.T) {
	suite.Run(t, new(ObjectLongHaulSuite))
}

type ObjectLongHaulSuite struct {
	suite.Suite
	kh        *utils.K8sHelper
	installer *installer.CephInstaller
	tc        *clients.TestClient
	namespace string
	op        installer.TestSuite
}

func (s *ObjectLongHaulSuite) SetupSuite() {
	s.namespace = "longhaul-ns"
	s.op, s.kh, s.installer = StartLoadTestCluster(s.T, s.namespace)
	s.tc = clients.CreateTestClient(s.kh, s.installer.Manifests)
}

func (s *ObjectLongHaulSuite) TestObjectLonghaulRun() {
	var wg sync.WaitGroup
	storeName := "longhaulstore"
	wg.Add(installer.Env.LoadVolumeNumber)
	for i := 1; i <= installer.Env.LoadVolumeNumber; i++ {
		if i == 1 {
			go ObjectStoreOperations(s, &wg, s.namespace, storeName+strconv.Itoa(i), false)
		} else {
			go ObjectStoreOperations(s, &wg, s.namespace, storeName+strconv.Itoa(i), randomBool())
		}

	}
	wg.Wait()
}

func ObjectStoreOperations(s *ObjectLongHaulSuite, wg *sync.WaitGroup, namespace string, storeName string, deleteStore bool) {
	defer wg.Done()
	bucketName := "loadbucket"
	s3 := createObjectStoreAndUser(s.T, s.kh, s.tc, s.namespace, storeName, "longhaul", "LongHaulTest")
	isFound, err := s3.IsBucketPresent(bucketName)
	if err == nil {
		if !isFound {
			s3.CreateBucket(bucketName)
		}
	} else {
		logger.Infof("Error which check if %s bucket exists, err -> %v", bucketName, err)
		s.T().Fail()
	}

	performObjectStoreOperations(s3, bucketName)
	if deleteStore {
		delOpts := metav1.DeleteOptions{}
		s.tc.ObjectClient.Delete(namespace, storeName)
		s.kh.Clientset.CoreV1().Services(namespace).Delete("rgw-external-"+storeName, &delOpts)
	}
	s3 = nil
}

func (s *ObjectLongHaulSuite) TearDownSuite() {
	s.tc = nil
	s.kh = nil
	s = nil
}
