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

	"time"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Rook Block Storage fencing longhaul test
// Create a PVC, mount a pod with readOnly= false and write some data and unmount pod
// Then Mound n pods concurrently with readOnly=true and read  data.
//NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
//to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test)
func TestBlockWithFencingLongHaul(t *testing.T) {
	suite.Run(t, new(BlockLongHaulSuiteWithFencing))
}

type BlockLongHaulSuiteWithFencing struct {
	suite.Suite
	kh         *utils.K8sHelper
	installer  *installer.InstallHelper
	testClient *clients.TestClient
	namespace  string
	op         contracts.Setup
}

//Test set up - does the following in order
//create pool and storage class, Create a PVC and mount a pod with ReadOnly=false.
//Write some data to the pvc and unmount the pod
func (s *BlockLongHaulSuiteWithFencing) SetupSuite() {
	var err error
	s.namespace = "longhaul-ns"
	s.op, s.kh, s.installer = StartBaseLoadTestOperations(s.T, s.namespace)
	createStorageClassAndPool(s.T, s.kh, s.namespace, "rook-ceph-block", "rook-pool")
	s.testClient, err = clients.CreateTestClient(s.kh, s.namespace)
	require.Nil(s.T(), err)
	if _, err := s.kh.GetPVCStatus(defaultNamespace, "block-pv-one"); err != nil {
		logger.Infof("Creating PVC and mounting it to pod with readOnly set to false")
		installer.BlockResourceOperation(s.kh, installer.GetBlockPvcDef("block-pv-one", "rook-ceph-block", "ReadWriteOnce"), "create")
		mountUnmountPVCOnPod(s.kh, "block-rw", "block-pv-one", "false", "create")
		require.True(s.T(), s.kh.IsPodRunning("block-rw", defaultNamespace))

		s.testClient.BlockClient.Write("block-rw", "/tmp/rook1", "this is long running test", "longhaul", defaultNamespace)
		mountUnmountPVCOnPod(s.kh, "block-rw", "block-pv-one", "false", "delete")
		require.True(s.T(), s.kh.IsPodTerminated("block-rw", defaultNamespace))
		time.Sleep(5 * time.Second)
	}

}

//Mount n number of pods on the same PVC created earlier with readOnly=True.
func (s *BlockLongHaulSuiteWithFencing) TestBlockWithFencingLonghaulRun() {

	var wg sync.WaitGroup
	wg.Add(s.installer.Env.LoadVolumeNumber)
	for i := 1; i <= s.installer.Env.LoadVolumeNumber; i++ {
		go blockVolumeFencingOperations(s, &wg, "block-read"+strconv.Itoa(i), "block-pv-one")
	}
	wg.Wait()
}

func blockVolumeFencingOperations(s *BlockLongHaulSuiteWithFencing, wg *sync.WaitGroup, podName string, pvcName string) {
	defer wg.Done()
	mountUnmountPVCOnPod(s.kh, podName, pvcName, "true", "create")
	require.True(s.T(), s.kh.IsPodRunning(podName, defaultNamespace))
	read, rErr := s.testClient.BlockClient.Read(podName, "/tmp/rook1", "longhaul", "default")
	require.Nil(s.T(), rErr)
	assert.Contains(s.T(), read, "this is long running test")
	mountUnmountPVCOnPod(s.kh, podName, pvcName, "true", "delete")
	require.True(s.T(), s.kh.IsPodTerminated(podName, defaultNamespace))
}

func (s *BlockLongHaulSuiteWithFencing) TearDownSuite() {
	s.kh = nil
	s.testClient = nil
	s = nil
}
