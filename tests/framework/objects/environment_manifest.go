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

package objects

import (
	"flag"
)

type EnvironmentManifest struct {
	K8sVersion         string
	Platform           string
	RookImageName      string
	ToolboxImageName   string
	SkipInstallRook    string
	LoadConcurrentRuns int
}

func NewManifest() EnvironmentManifest {
	e := EnvironmentManifest{}

	flag.StringVar(&e.K8sVersion, "k8s_version", "v1.6", "Version of kubernetes to test rook in; v1.5 | v1.6 are the only version of kubernetes currently supported")
	flag.StringVar(&e.Platform, "rook_platform", "Kubernetes", "Platform to install rook on; Kubernetes is the only platform currently supported")
	flag.StringVar(&e.RookImageName, "rook_image", "rook/rook", "Docker image name for the rook container to install, must be in docker hub or local environment")
	flag.StringVar(&e.ToolboxImageName, "toolbox_image", "rook/toolbox", "Docker image name of the toolbox container to install, must be in docker hub or local environment")
	flag.StringVar(&e.SkipInstallRook, "skip_install_rook", "false", "Indicate if Rook need to installed - false if tests are being running at Rook that is pre-installed")
	flag.IntVar(&e.LoadConcurrentRuns, "load_parallel_runs", 20, "number of routines for load test")
	flag.Parse()

	return e
}
