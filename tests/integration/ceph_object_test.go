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

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	bucketlifecycle "github.com/rook/rook/tests/integration/object/bucket/lifecycle"
	bucketowner "github.com/rook/rook/tests/integration/object/bucket/owner"
	bucketpolicy "github.com/rook/rook/tests/integration/object/bucket/policy"
	bucketquota "github.com/rook/rook/tests/integration/object/bucket/quota"
	bucketrw "github.com/rook/rook/tests/integration/object/bucket/rw"
	"github.com/rook/rook/tests/integration/object/cosi"
	"github.com/rook/rook/tests/integration/object/dependents"
	"github.com/rook/rook/tests/integration/object/notification"
	topickafka "github.com/rook/rook/tests/integration/object/topic/kafka"
	usercaps "github.com/rook/rook/tests/integration/object/user/caps"
	userkeys "github.com/rook/rook/tests/integration/object/user/keys"
	useropmask "github.com/rook/rook/tests/integration/object/user/opmask"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/zonepools"
)

func TestCephObjectSuite(t *testing.T) {
	s := new(ObjectSuite)
	defer func(s *ObjectSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}

type ObjectSuite struct {
	suite.Suite
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

	runObjectE2ETest(s.T(), s.k8sh, s.installer, s.settings.Namespace, true)
}

func (s *ObjectSuite) TestWithoutTLS() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("object store tests skipped on openshift")
	}

	runObjectE2ETest(s.T(), s.k8sh, s.installer, s.settings.Namespace, false)
}

func runObjectE2ETest(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace string, tlsEnable bool) {
	// only the packages that create CephObjectStoreUsers in their own namespace
	// need to be allowed; the rest reach the store through OBCs, which this does
	// not gate
	sharedObjectStore := sharedstore.Create(t, k8sh, installer, sharedstore.Config{
		Namespace: namespace,
		StoreName: "sharedstore",
		Instances: 1,
		TLSEnable: tlsEnable,
		AllowUsersInNamespaces: []string{
			bucketowner.Namespace,
			cosi.Namespace,
			usercaps.Namespace,
			userkeys.Namespace,
			useropmask.Namespace,
		},
	})
	defer sharedObjectStore.Destroy()

	zonepools.TestZonePools(t, k8sh, sharedObjectStore)
	bucketlifecycle.TestObjectBucketClaimLifecycle(t, k8sh, sharedObjectStore)
	bucketowner.TestObjectBucketClaimBucketOwner(t, k8sh, sharedObjectStore)
	bucketpolicy.TestObjectBucketClaimPolicy(t, k8sh, sharedObjectStore)
	bucketquota.TestObjectBucketClaimQuota(t, k8sh, sharedObjectStore)
	bucketrw.TestObjectBucketClaimReadWrite(t, k8sh, sharedObjectStore)
	userkeys.TestObjectStoreUserKeys(t, k8sh, sharedObjectStore)
	topickafka.TestBucketTopicKafka(t, k8sh, sharedObjectStore)
	useropmask.TestObjectStoreUserOpMask(t, k8sh, sharedObjectStore)
	usercaps.TestObjectStoreUserCaps(t, k8sh, sharedObjectStore)
	// the ceph-cosi driver cannot reach a TLS object store endpoint, so this
	// suite skips itself in the TLS pass
	cosi.TestCephCOSIDriver(t, k8sh, sharedObjectStore)
	notification.TestBucketNotification(t, k8sh, sharedObjectStore)

	// last: this builds and deletes a store of its own, so keep it clear of the
	// packages sharing the fixture store
	dependents.TestCephObjectStoreDependents(t, k8sh, installer, namespace, tlsEnable)
}
