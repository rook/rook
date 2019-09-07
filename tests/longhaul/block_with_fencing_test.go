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
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Rook Block Storage fencing longhaul test
// Create a PVC, mount a pod with readOnly= false and write some data and unmount pod
// Then Mound n pods concurrently with readOnly=true and read  data.
// NOTE: This tests doesn't clean up the cluster or volume after the run, the tests is designed
// to reuse the same cluster and volume for multiple runs or over a period of time.
// It is recommended to run this test with -count test param (to repeat th test n number of times)
// along with --load_parallel_runs params(number of concurrent operations per test)
func TestBlockWithFencingLongHaul(t *testing.T) {
	suite.Run(t, new(BlockLongHaulSuiteWithFencing))
}

type BlockLongHaulSuiteWithFencing struct {
	suite.Suite
	kh         *utils.K8sHelper
	installer  *installer.CephInstaller
	testClient *clients.TestClient
	namespace  string
	op         installer.TestSuite
}

// Test set up - does the following in order
// create pool and storage class, Create a PVC and mount a pod with ReadOnly=false.
// Write some data to the pvc and unmount the pod
func (s *BlockLongHaulSuiteWithFencing) SetupSuite() {
	s.namespace = "longhaul-ns"
	s.op, s.kh, s.installer = StartLoadTestCluster(s.T, s.namespace)
	s.testClient = clients.CreateTestClient(s.kh, s.installer.Manifests)
	createStorageClassAndPool(s.T, s.testClient, s.kh, s.namespace, "rook-ceph-block", "rook-pool")
	if _, err := s.kh.GetPVCStatus(defaultNamespace, "block-pv-one"); err != nil {
		logger.Infof("Creating PVC and mounting it to pod with readOnly set to false")
		err = s.testClient.BlockClient.CreatePvc("block-pv-one", "rook-ceph-block", "ReadWriteOnce", "1M")
		require.Nil(s.T(), err)
		mountUnmountPVCOnPod(s.kh, "block-rw", "block-pv-one", "false", "apply")
		require.True(s.T(), s.kh.IsPodRunning("block-rw", defaultNamespace))

		filename := "longhaul"
		assert.Nil(s.T(), s.kh.WriteToPod(defaultNamespace, "block-rw", filename, "this is long running test"))
		mountUnmountPVCOnPod(s.kh, "block-rw", "block-pv-one", "false", "delete")
		require.True(s.T(), s.kh.IsPodTerminated("block-rw", defaultNamespace))
		time.Sleep(5 * time.Second)
	}
}

// Mount n number of pods on the same PVC created earlier with readOnly=True.
func (s *BlockLongHaulSuiteWithFencing) TestBlockWithFencingLonghaulRun() {

	var wg sync.WaitGroup
	wg.Add(installer.Env.LoadVolumeNumber)
	for i := 1; i <= installer.Env.LoadVolumeNumber; i++ {
		go blockVolumeFencingOperations(s, &wg, "block-read"+strconv.Itoa(i), "block-pv-one")
	}
	wg.Wait()
}

func blockVolumeFencingOperations(s *BlockLongHaulSuiteWithFencing, wg *sync.WaitGroup, podName string, pvcName string) {
	defer wg.Done()
	mountUnmountPVCOnPod(s.kh, podName, pvcName, "true", "apply")
	require.True(s.T(), s.kh.IsPodRunning(podName, defaultNamespace))
	message := "this is long running test"
	require.Nil(s.T(), s.kh.ReadFromPod("default", podName, "longhaul", message))

	mountUnmountPVCOnPod(s.kh, podName, pvcName, "true", "delete")
	require.True(s.T(), s.kh.IsPodTerminated(podName, defaultNamespace))
}

func (s *BlockLongHaulSuiteWithFencing) TearDownSuite() {
	s.kh = nil
	s.testClient = nil
	s = nil
}
