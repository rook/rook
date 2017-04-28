package enums

import (
	"fmt"
	"strings"
)

type K8sVersion int

const (
	V1dot5 K8sVersion = iota + 1
	V1dot6
)

var versions = [...]string{
	"v1.5",
	"v1.6",
	"None",
}

func (version K8sVersion) String() string {
	return versions[version-1]
}

func GetK8sVersionFromString(name string) (K8sVersion, error) {
	switch {
	case strings.EqualFold(name, V1dot6.String()):
		return V1dot6, nil
	default:
		return V1dot5, fmt.Errorf("Unsupported Kubernetes version: " + name)
	}
}
