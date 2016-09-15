package clusterd

import (
	"log"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"
)

const (
	triggeredValue = "triggered"
)

// Simplified trigger agents option: assumes agent on each node is requesting installation and does not support multiple instances
func TriggerAgentsAndWaitForCompletion(etcdClient etcd.KeysAPI, nodeIDs []string, agentKey string, requiredSuccessCount int) error {
	if len(nodeIDs) == 0 {
		//log.Printf("No nodes to trigger for agent %s", agentKey)
		return nil
	}

	// foreach node, process its list of changes
	for _, nodeID := range nodeIDs {

		if err := SetNodeConfigStatus(etcdClient, nodeID, agentKey, NodeConfigStatusTriggered); err != nil {
			log.Printf("Failed to set status value. node=%s, agent=%s, err=: %v", nodeID, agentKey, err)
			return err
		}

		// Trigger the agent to install the service
		key := path.Join(GetNodeProgressKey(nodeID), StatusValue)
		if _, err := etcdClient.Set(ctx.Background(), key, triggeredValue, nil); err != nil {
			log.Printf("Failed to trigger changes on node %s. err: %v", nodeID, err)
			return err
		}
	}

	// Wait for the agent to complete the component installation
	numSuccessful, err := WaitForNodeConfigCompletion(etcdClient, agentKey, nodeIDs, 30)
	if numSuccessful < requiredSuccessCount {
		return err
	}

	return nil
}
