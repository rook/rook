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
package osd

import (
	"path"

	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
)

type Leader struct {
}

func NewLeader() *Leader {
	return &Leader{}
}

// Load the state of the OSDs from etcd.
// Returns whether the service has updates to be applied.
func getOSDState(context *clusterd.Context) (bool, error) {
	return len(context.Inventory.Nodes) > 0, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (l *Leader) Configure(context *clusterd.Context, nodes []string) error {

	if len(nodes) == 0 {
		// No nodes for OSDs
		return nil
	}

	// Trigger all of the nodes to configure their OSDs
	osdNodes := []string{}
	for _, nodeID := range nodes {
		key := path.Join(mon.CephKey, osdAgentName, clusterd.DesiredKey, nodeID, "ready")
		_, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil)
		if err != nil {
			logger.Warningf("failed to trigger osd %s", nodeID)
			continue
		}

		osdNodes = append(osdNodes, nodeID)
	}

	// At least half of the OSDs must succeed
	return clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, osdNodes, osdAgentName, 1+(len(osdNodes)/2))
}
