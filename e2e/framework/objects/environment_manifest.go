package objects

import (
	"flag"
)

type EnvironmentManifest struct {
	K8sVersion         string
	Platform           string
	RookTag            string
	SkipInstallRook    string
	LoadConcurrentRuns int
}

func NewManifest() EnvironmentManifest {
	e := EnvironmentManifest{}

	flag.StringVar(&e.K8sVersion, "k8s_version", "v1.6", "Version of kubernetes to test rook in; v1.5 | v1.6 are the only version of kubernetes currently supported")
	flag.StringVar(&e.Platform, "rook_platform", "Kubernetes", "Platform to install rook on; Kubernetes is the only platform currently supported")
	flag.StringVar(&e.RookTag, "rook_version", "master-latest", "Docker tag of the rook operator to install, must be in quay.io or local environment")
	flag.StringVar(&e.SkipInstallRook, "skip_install_rook", "false", "Indicate if Rook need to installed - false if tests are being running at Rook that is pre-installed")
	flag.IntVar(&e.LoadConcurrentRuns, "load_parallel_runs", 20, "number of routines for load test")
	flag.Parse()

	return e
}
