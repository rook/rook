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

	flag.StringVar(&e.K8sVersion, "k8sVersion", "V1dot5", "Version of kubernetes to test rook in; V1dot5 is the only version of kubernetes currently supported")
	flag.StringVar(&e.Platform, "platform", "Kubernetes", "Platform to install rook on; Kubernetes is the only platform currently supported")
	flag.StringVar(&e.RookTag, "rookTag", "ca", "Docker tag of the rook operator to install, must be in quay.io or local environment")

	flag.Parse()

	return e
}
