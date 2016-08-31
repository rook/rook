package castled

import "github.com/quantum/clusterd/pkg/orchestrator"

// Load the state of the OSDs from etcd.
// Returns whether the service has updates to be applied.
func getOSDState(context *orchestrator.ClusterContext) (bool, error) {
	return len(context.Inventory.Nodes) > 0, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func configureOSDs(context *orchestrator.ClusterContext) error {

	if len(context.Inventory.Nodes) == 0 {
		// No nodes for OSDs
		return nil
	}

	// Trigger all of the nodes to configure their OSDs
	osdNodes := []string{}
	for nodeID := range context.Inventory.Nodes {
		osdNodes = append(osdNodes, nodeID)
	}

	// At least half of the OSDs must succeed
	return orchestrator.TriggerAgentsAndWaitForCompletion(context.EtcdClient, osdNodes, osdAgentName, 1+(len(osdNodes)/2))
}
