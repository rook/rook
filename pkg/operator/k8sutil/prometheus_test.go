package k8sutil

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetServiceMonitor(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	filePath := path.Join(gopath, "src/github.com/rook/rook/cluster/examples/kubernetes/ceph/monitoring/service-monitor.yaml")
	servicemonitor, err := GetServiceMonitor(filePath)
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mgr", servicemonitor.GetName())
	assert.Equal(t, "rook-ceph", servicemonitor.GetNamespace())
	assert.NotNil(t, servicemonitor.Spec.NamespaceSelector.MatchNames)
	assert.NotNil(t, servicemonitor.Spec.Endpoints)
}

func TestGetPrometheusRule(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	filePath := path.Join(gopath, "src/github.com/rook/rook/cluster/examples/kubernetes/ceph/monitoring/prometheus-ceph-v14-rules.yaml")
	rules, err := GetPrometheusRule(filePath)
	assert.Nil(t, err)
	assert.Equal(t, "prometheus-ceph-rules", rules.GetName())
	assert.Equal(t, "rook-ceph", rules.GetNamespace())
	// Labels should be present as they are used by prometheus for identifying rules
	assert.NotNil(t, rules.GetLabels())
	assert.NotNil(t, rules.Spec.Groups)
}
