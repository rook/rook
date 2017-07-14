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

	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/objects"
)

var (
	// Env var containing environment manifest
	Env  objects.EnvironmentManifest
	rook *installer.InstallHelper
	//Platform where rook needs to installed eg:k8s or standalone
	Platform enums.RookPlatformType
)

//One init function per package - initializes Rook infra and installs rook(if needed based on flags)
func init() {
	Env = objects.NewManifest()
	fmt.Println(Env)
	var err error
	platform, err := enums.GetRookPlatFormTypeFromString(Env.Platform)
	if err != nil {
		panic(fmt.Errorf("Cannot get platform %v", err))
	}
	Platform = platform
	rook, err := installer.NewK8sRookhelper()

	skipRookInstall := strings.EqualFold(Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}
	err = rook.InstallRookOnK8s(Env.K8sVersion)
	if err != nil {
		panic(fmt.Errorf("failed to install rook: %v", err))
	}

}

// CleanUp function to install Rook
func CleanUp() {
	skipRookInstall := strings.EqualFold(Env.SkipInstallRook, "true")
	if skipRookInstall {
		return
	}
	rook.UninstallRookFromK8s(Env.K8sVersion)

}
