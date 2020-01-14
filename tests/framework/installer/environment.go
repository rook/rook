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

package installer

import (
	"flag"
)

// EnvironmentManifest contains information about system under test
type EnvironmentManifest struct {
	HostType           string
	Helm               string
	RookImageName      string
	ToolboxImageName   string
	BaseTestDir        string
	SkipInstallRook    bool
	LoadVolumeNumber   int
	LoadConcurrentRuns int
	LoadTime           int
	LoadSize           string
	EnableChaos        bool
	Logs               string
}

var Env EnvironmentManifest

func init() {
	Env = EnvironmentManifest{}
	flag.StringVar(&Env.HostType, "host_type", "localhost", "Host where tests are run eg - localhost, GCE or AWS")
	flag.StringVar(&Env.Helm, "helm", "helm", "Path to helm binary")
	flag.StringVar(&Env.RookImageName, "rook_image", "rook/ceph", "Docker image name for the rook container to install, must be in docker hub or local environment")
	flag.StringVar(&Env.ToolboxImageName, "toolbox_image", "rook/ceph", "Docker image name of the toolbox container to install, must be in docker hub or local environment")
	flag.StringVar(&Env.BaseTestDir, "base_test_dir", "/data", "Base test directory, for use only when kubernetes master is running on localhost")
	flag.BoolVar(&Env.SkipInstallRook, "skip_install_rook", false, "Indicate if Rook need to installed - false if tests are being running at Rook that is pre-installed")
	flag.IntVar(&Env.LoadConcurrentRuns, "load_parallel_runs", 20, "number of routines for load test")
	flag.IntVar(&Env.LoadVolumeNumber, "load_volumes", 1, "number of volumes(file,object or block) to be created for load test")
	flag.IntVar(&Env.LoadTime, "load_time", 1800, "number of seconds each thread perform operations for")
	flag.StringVar(&Env.LoadSize, "load_size", "medium", "load size for each thread performing operations - small,medium or large.")
	flag.BoolVar(&Env.EnableChaos, "enable_chaos", false, "used to determine if random pods in a namespace are to be killed during load test.")
	flag.StringVar(&Env.Logs, "logs", "", "Gather rook logs, eg - all")
}
