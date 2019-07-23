package k8sutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestParseServiceType(t *testing.T) {
	validServiceTypes := []string{"ClusterIP", "NodePort", "LoadBalancer", "ExternalName"}
	for _, serviceType := range validServiceTypes {
		assert.Equal(t, v1.ServiceType(serviceType), ParseServiceType(serviceType))
	}

	invalidServiceTypes := []string{"", "nodeport", "notarealservice"}
	for _, serviceType := range invalidServiceTypes {
		assert.Equal(t, v1.ServiceType(""), ParseServiceType(serviceType))
	}
}
