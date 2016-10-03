package cephmgr

import (
	"log"
	"path"

	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/clusterd"
)

// Load the state of the OSDs from etcd.
// Returns whether the service has updates to be applied.
func getOSDState(context *clusterd.Context) (bool, error) {
	return len(context.Inventory.Nodes) > 0, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func configureOSDs(context *clusterd.Context, nodes []string) error {

	if len(nodes) == 0 {
		// No nodes for OSDs
		return nil
	}

	// Trigger all of the nodes to configure their OSDs
	osdNodes := []string{}
	for _, nodeID := range nodes {
		key := path.Join(cephKey, osdAgentName, desiredKey, nodeID, "ready")
		_, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil)
		if err != nil {
			log.Printf("failed to trigger osd %s", nodeID)
			continue
		}

		osdNodes = append(osdNodes, nodeID)
	}

	// At least half of the OSDs must succeed
	return clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, osdNodes, osdAgentName, 1+(len(osdNodes)/2))
}
