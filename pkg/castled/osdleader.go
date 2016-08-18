package castled

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

// Interface implemented by a service that has been elected leader
type osdLeader struct {
	cluster     *clusterInfo
	privateIPv4 string
	etcdClient  etcd.KeysAPI
}

// Load the state of the service from etcd. Typically a service will populate the desired/discovered state and the applied state
// from etcd, then compute the difference and cache it.
// Returns whether the service has updates to be applied.
func (m *osdLeader) LoadState(context *orchestrator.ClusterContext) (bool, error) {
	return true, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (m *osdLeader) ApplyState(context *orchestrator.ClusterContext) error {

	return nil
}

// Get the changed state for the service
func (m *osdLeader) GetChangedState() interface{} {
	return nil
}
