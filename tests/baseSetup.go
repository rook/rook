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
	"strings"

	"github.com/rook/rook/pkg/test/enums"
	"github.com/rook/rook/pkg/test/manager"
	"github.com/rook/rook/pkg/test/objects"
)

var (
	Env      objects.EnvironmentManifest
	Platform enums.RookPlatformType
)

//One init function per package - initializes Rook infra and installs rook(if needed based on flags)
func init() {
	Env = objects.NewManifest()
	var err error

	rookPlatform, err := enums.GetRookPlatFormTypeFromString(Env.Platform)
	if err != nil {
		panic(fmt.Errorf("Cannot get platform %v", err))
	}

	k8sVersion, _ := enums.GetK8sVersionFromString(Env.K8sVersion)

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(rookPlatform, true, k8sVersion)
	if err != nil {
		panic(fmt.Errorf("Error during Rook Infra Setup %v", err))
	}
	skipRookInstall := strings.EqualFold(Env.SkipInstallRook, "true")
	rookInfra.ValidateAndSetupTestPlatform(skipRookInstall)

	if !skipRookInstall {
		err = rookInfra.InstallRook(env.RookImageName, env.ToolboxImageName)
		if err != nil {
			panic(fmt.Errorf("Error during Rook Infra Setup %v", err))
		}
	}
	Platform = rookInfra.GetRookPlatform()
}
