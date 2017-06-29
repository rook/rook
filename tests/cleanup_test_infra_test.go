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

package tests

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/test/enums"
	"github.com/rook/rook/pkg/test/manager"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestRookInfraCleanUp(t *testing.T) {
	suite.Run(t, new(CleanUpTestSuite))
}

type CleanUpTestSuite struct {
	suite.Suite
	rookPlatform enums.RookPlatformType
	k8sVersion   enums.K8sVersion
}

func (suite *CleanUpTestSuite) TestRookInfraCleanUpTest() {
	var err error

	suite.rookPlatform, err = enums.GetRookPlatFormTypeFromString(Env.Platform)

	require.Nil(suite.T(), err)

	suite.k8sVersion, err = enums.GetK8sVersionFromString(Env.K8sVersion)

	require.Nil(suite.T(), err)

	fmt.Println(suite.rookPlatform)
	fmt.Println(suite.k8sVersion)

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(suite.rookPlatform, true, suite.k8sVersion)

	require.Nil(suite.T(), err)

	rookInfra.TearDownInfrastructureCreatedEnvironment()
}
