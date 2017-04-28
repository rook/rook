package objects

import (
	"flag"
)

type EnvironmentManifest struct {
	K8sVersion string
	Platform   string
	RookTag    string
}

func NewManifest() EnvironmentManifest {
	e := EnvironmentManifest{}

	flag.StringVar(&e.K8sVersion, "k8s_version", "v1.6", "Version of kubernetes to test rook in; v1.5 | v1.6 are the only version of kubernetes currently supported")
	flag.StringVar(&e.Platform, "rook_platform", "Kubernetes", "Platform to install rook on; Kubernetes is the only platform currently supported")
	flag.StringVar(&e.RookTag, "rook_version", "quay.io/rook/rookd:master-latest", "Docker tag of the rook operator to install, must be in quay.io or local environment")

	flag.Parse()

	return e
}
