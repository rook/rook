package castled

import (
	"log"
	"path"

	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/util"
)

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
		key := path.Join(cephKey, osdAgentName, desiredKey, nodeID)
		err := util.CreateEtcdDir(context.EtcdClient, key)
		if err != nil {
			log.Printf("failed to trigger osd %s", nodeID)
			continue
		}

		osdNodes = append(osdNodes, nodeID)
	}

	// At least half of the OSDs must succeed
	return orchestrator.TriggerAgentsAndWaitForCompletion(context.EtcdClient, osdNodes, osdAgentName, 1+(len(osdNodes)/2))
}
