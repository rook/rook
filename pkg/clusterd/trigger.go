/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package clusterd

import (
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"
)

const (
	triggeredValue = "triggered"
)

// Trigger agents that they need to apply new state.
// The method will wait for all nodes to respond.
// Returns success if at least "requiredSuccessCount" of the nodes succeed.
// Waits at most 60s per node.
func TriggerAgentsAndWaitForCompletion(etcdClient etcd.KeysAPI, nodeIDs []string, agentKey string, requiredSuccessCount int) error {
	return TriggerAgentsAndWait(etcdClient, nodeIDs, agentKey, requiredSuccessCount, 120)
}

// Trigger agents that they need to apply new state.
// The method will wait for all nodes to respond.
// Returns success if at least "requiredSuccessCount" of the nodes succeed.
// Waits at most the designated number of seconds.
func TriggerAgentsAndWait(etcdClient etcd.KeysAPI, nodeIDs []string, agentKey string, requiredSuccessCount, waitSeconds int) error {

	// Trigger the agents
	err := TriggerAgents(etcdClient, nodeIDs, agentKey)
	if err != nil {
		return fmt.Errorf("failed to trigger %s agents. %v", agentKey, err)
	}

	// Wait for the agents to complete the component installation
	numSuccessful, err := WaitForNodeConfigCompletion(etcdClient, agentKey, nodeIDs, waitSeconds)
	if numSuccessful < requiredSuccessCount {
		return err
	}

	return nil
}

// Trigger agents that they need to apply new state.
// The method will not wait for any nodes to respond.
// Returns success if the etcd trigger keys were set.
func TriggerAgents(etcdClient etcd.KeysAPI, nodeIDs []string, agentKey string) error {

	// foreach node, process its list of changes
	for _, nodeID := range nodeIDs {

		if err := SetNodeConfigStatus(etcdClient, nodeID, agentKey, NodeConfigStatusTriggered); err != nil {
			logger.Errorf("Failed to set status value. node=%s, agent=%s, err=: %v", nodeID, agentKey, err)
			return err
		}

		// trigger the agent to configure the service
		key := path.Join(GetNodeProgressKey(nodeID), StatusValue)
		if _, err := etcdClient.Set(ctx.Background(), key, triggeredValue, nil); err != nil {
			logger.Errorf("Failed to trigger changes on node %s. err: %v", nodeID, err)
			return err
		}
	}

	return nil
}
