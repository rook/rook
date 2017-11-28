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

//EnvironmentManifest contains informaton about system under test
type EnvironmentManifest struct {
	HostType           string
	RookImageName      string
	ToolboxImageName   string
	SkipInstallRook    string
	LoadVolumeNumber   int
	LoadConcurrentRuns int
	LoadTime           int
}

//NewManifest creates a new instance of EnvironmentManifest
func NewManifest() EnvironmentManifest {
	e := EnvironmentManifest{}
	flag.StringVar(&e.HostType, "host_type", "localhost", "Host were tests are run eg - localhost,GCE or AWS")
	flag.StringVar(&e.RookImageName, "rook_image", "rook/rook", "Docker image name for the rook container to install, must be in docker hub or local environment")
	flag.StringVar(&e.ToolboxImageName, "toolbox_image", "rook/toolbox", "Docker image name of the toolbox container to install, must be in docker hub or local environment")
	flag.StringVar(&e.SkipInstallRook, "skip_install_rook", "false", "Indicate if Rook need to installed - false if tests are being running at Rook that is pre-installed")
	flag.IntVar(&e.LoadConcurrentRuns, "load_parallel_runs", 20, "number of routines for load test")
	flag.IntVar(&e.LoadVolumeNumber, "load_volumes", 1, "number of volumes(file,object or block) to be created for load test")
	flag.IntVar(&e.LoadTime, "load_time", 1800, "number of seconds each thread perform operations for")
	flag.Parse()

	return e
}
