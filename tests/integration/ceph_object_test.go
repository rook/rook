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
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	bucketlifecycle "github.com/rook/rook/tests/integration/object/bucket/lifecycle"
	bucketowner "github.com/rook/rook/tests/integration/object/bucket/owner"
	bucketpolicy "github.com/rook/rook/tests/integration/object/bucket/policy"
	bucketquota "github.com/rook/rook/tests/integration/object/bucket/quota"
	bucketrw "github.com/rook/rook/tests/integration/object/bucket/rw"
	"github.com/rook/rook/tests/integration/object/cosi"
	"github.com/rook/rook/tests/integration/object/dependents"
	storelifecycle "github.com/rook/rook/tests/integration/object/lifecycle"
	"github.com/rook/rook/tests/integration/object/notification"
	topickafka "github.com/rook/rook/tests/integration/object/topic/kafka"
	usercaps "github.com/rook/rook/tests/integration/object/user/caps"
	userkeys "github.com/rook/rook/tests/integration/object/user/keys"
	useropmask "github.com/rook/rook/tests/integration/object/user/opmask"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
)

const objectStoreServicePrefixUniq = "rook-ceph-rgw-"

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

	runObjectE2ETest(s.k8sh, s.installer, &s.Suite, true)
}

func (s *ObjectSuite) TestWithoutTLS() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}

	runObjectE2ETest(s.k8sh, s.installer, &s.Suite, false)
}

func runObjectE2ETest(k8sh *utils.K8sHelper, installer *installer.CephInstaller, s *suite.Suite, tlsEnable bool) {
	sharedObjectStore := sharedstore.Create(s.T(), k8sh, installer, tlsEnable,
		bucketlifecycle.Namespace,
		bucketowner.Namespace,
		bucketpolicy.Namespace,
		bucketquota.Namespace,
		bucketrw.Namespace,
		userkeys.Namespace,
		topickafka.Namespace,
		useropmask.Namespace,
		usercaps.Namespace,
		cosi.Namespace,
		dependents.Namespace,
		notification.Namespace,
	)
	defer sharedObjectStore.Destroy()

	bucketlifecycle.TestObjectBucketClaimLifecycle(s.T(), k8sh, sharedObjectStore)
	bucketowner.TestObjectBucketClaimBucketOwner(s.T(), k8sh, sharedObjectStore)
	bucketpolicy.TestObjectBucketClaimPolicy(s.T(), k8sh, sharedObjectStore)
	bucketquota.TestObjectBucketClaimQuota(s.T(), k8sh, sharedObjectStore)
	bucketrw.TestObjectBucketClaimReadWrite(s.T(), k8sh, sharedObjectStore)
	userkeys.TestObjectStoreUserKeys(s.T(), k8sh, sharedObjectStore)
	topickafka.TestBucketTopicKafka(s.T(), k8sh, sharedObjectStore)
	useropmask.TestObjectStoreUserOpMask(s.T(), k8sh, sharedObjectStore)
	usercaps.TestObjectStoreUserCaps(s.T(), k8sh, sharedObjectStore)
	// the ceph-cosi driver cannot reach a TLS object store endpoint, so this
	// suite skips itself in the TLS pass
	cosi.TestCephCOSIDriver(s.T(), k8sh, sharedObjectStore)
	dependents.TestCephObjectStoreDependents(s.T(), k8sh, sharedObjectStore)
	notification.TestBucketNotification(s.T(), k8sh, sharedObjectStore)
	storelifecycle.TestCephObjectStoreLifecycle(s.T(), k8sh, sharedObjectStore)
}
