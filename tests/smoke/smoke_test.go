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

package smoke

import (
	"testing"

	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestSmokeSuiteK8s(t *testing.T) {
	suite.Run(t, new(SmokeSuite))
}

type SmokeSuite struct {
	suite.Suite
	helper    *TestHelper
	installer *installer.InstallHelper
}

func (suite *SmokeSuite) SetupSuite() {
	kh, err := utils.CreatK8sHelper()
	require.NoError(suite.T(), err)

	suite.installer = installer.NewK8sRookhelper(kh.Clientset)

	err = suite.installer.InstallRookOnK8s()
	require.NoError(suite.T(), err)

	suite.helper, err = CreateSmokeTestClient(enums.Kubernetes, suite.installer.Env.K8sVersion, kh)
	require.Nil(suite.T(), err)
}

func (suite *SmokeSuite) TearDownSuite() {
	suite.installer.UninstallRookFromK8s()
}
