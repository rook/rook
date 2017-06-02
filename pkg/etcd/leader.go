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
package etcd

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"

	"github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/etcd/bootstrap"
	"github.com/rook/rook/pkg/etcd/policy"
	ctx "golang.org/x/net/context"
)

const (
	etcdmgrKey = "/rook/services/etcd"
)

type etcdMgrLeader struct {
	context bootstrap.EtcdMgrContext
}

func (e *etcdMgrLeader) RefreshKeys() []*clusterd.RefreshKey {
	return []*clusterd.RefreshKey{}
}

func (e *etcdMgrLeader) HandleRefresh(event *clusterd.RefreshEvent) {
	logger.Debugf("etcdmgr leader received refresh event")

	var unhealthyNodes []*clusterd.UnhealthyNode
	for _, node := range event.NodesUnhealthy {
		unhealthyNodes = append(unhealthyNodes, node)
	}

	err := e.ConfigureEtcdServices(event.Context, unhealthyNodes)
	if err != nil {
		logger.Warningf("failed to refresh etcdmgr: %+v", err)
	}

	logger.Infof("etcdmgr leader completed refresh event")
}

// ConfigureEtcdServices
func (e *etcdMgrLeader) ConfigureEtcdServices(context *clusterd.Context, unhealthyNodes []*clusterd.UnhealthyNode) error {
	logger.Tracef("entered etcdMgr.ConfigureEtcdservices")

	currentEtcdMembers, _, err := e.context.Members()
	if err != nil {
		return err
	}
	// currentEtcdMembers are full URLs (scheme, ip, port). We want to convert them to a list of node IDs.
	currentEtcdMemberIDs, err := getNodeIDs(currentEtcdMembers, context.Inventory.Nodes)
	if err != nil {
		logger.Errorf("error in converting etcd member urls to node ids.")
		return err
	}

	logger.Infof("current etcd cluster members: %v | Nodes: %v | IDs: %v | unhealthy nodes: %v",
		currentEtcdMembers, context.Inventory.Nodes, currentEtcdMemberIDs, unhealthyNodes)
	currentEtcdQuorumSize := len(currentEtcdMembers)
	currentClusterSize := len(context.Inventory.Nodes) - len(unhealthyNodes)
	logger.Infof("currentEtcdQuorumSize: %v | currentClusterSize: %v ", currentEtcdQuorumSize, currentClusterSize)
	desiredEtcdQuorumSize := policy.CalculateDesiredEtcdCount(currentClusterSize)
	var candidates []string
	delta := desiredEtcdQuorumSize - currentEtcdQuorumSize
	var clusterNodes []string
	for node := range context.Inventory.Nodes {
		clusterNodes = append(clusterNodes, node)
	}

	var unhealthyIDs []string
	for _, node := range unhealthyNodes {
		unhealthyIDs = append(unhealthyIDs, node.ID)
	}

	logger.Infof("unhealthy nodeIDs: %+v", unhealthyIDs)
	if delta != 0 {
		logger.Infof("desiredEtcdQuorumSize: %v, delta: %v, clusterNodes: %v", desiredEtcdQuorumSize, delta, clusterNodes)
		logger.Infof("currentEtcdMemberIDs: %+v", currentEtcdMemberIDs)
		candidates, err = policy.ChooseEtcdCandidatesToAddOrRemove(delta, currentEtcdMemberIDs, clusterNodes, unhealthyIDs)
		if err != nil {
			return err
		}
		logger.Infof("candidates: %+v", candidates)
	}

	if delta > 0 {
		logger.Infof("target nodes to run new instances of embedded etcds on: %+v", candidates)
		err := e.growEtcdQuorum(context, candidates)
		if err != nil {
			return fmt.Errorf("error in growing etcd quorum: %+v", err)
		}

	} else if delta < 0 {
		logger.Infof("target nodes to remove the instances of embedded etcds from: %+v", candidates)
		err := e.shrinkEtcdQuorum(context, candidates)
		if err != nil {
			return fmt.Errorf("error in shrinking etcd quorum: %+v", err)
		}
	}
	return nil
}

func (e *etcdMgrLeader) growEtcdQuorum(context *clusterd.Context, candidates []string) error {
	// We need to do the operations one by one to prevent quorum corruption
	for _, candidate := range candidates {
		// add target node to the current etcd cluster
		var targetIP string
		if node, ok := context.Inventory.Nodes[candidate]; ok {
			targetIP = node.PrivateIP
		} else {
			return errors.New("candidate ip not found in the inventory")
		}

		// set ip address for the target agent (will be used to bootstrap embedded etcd)
		ipKey := path.Join(etcdmgrKey, clusterd.DesiredKey, candidate, "ipaddress")
		logger.Infof("address key for new instance: %s", ipKey)
		_, err := context.EtcdClient.Set(ctx.Background(), ipKey, targetIP, nil)
		if err != nil {
			return err
		}

		logger.Infof("triggering the agent on %v to create an instance of embedded etcd", targetIP)
		err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, []string{candidate}, etcdMgrAgentName, len(candidates))
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *etcdMgrLeader) shrinkEtcdQuorum(context *clusterd.Context, candidates []string) error {
	// We need to do the operations one by one to prevent quorum corruption
	for _, candidate := range candidates {
		// remove target node from the current etcd cluster
		var targetEndpoint string
		if node, ok := context.Inventory.Nodes[candidate]; ok {
			targetEndpoint = getPeerEndpointFromIP(node.PrivateIP)
		} else {
			return errors.New("candidate endpoint not found in the inventory")
		}

		key := path.Join(etcdmgrKey, clusterd.DesiredKey, candidate, "ipaddress")
		logger.Infof("address key for new instance: %s", key)
		_, err := context.EtcdClient.Delete(ctx.Background(), key, &client.DeleteOptions{Dir: true, Recursive: true})
		if err != nil {
			return fmt.Errorf("error in removing desired key for node: %+v err: %+v", candidate, err)
		}

		err = RemoveMember(e.context, targetEndpoint)
		if err != nil {
			return fmt.Errorf("error in removing a member from the cluster. %+v", err)
		}

		// Note: For remove, we first try to remove it from cluster and then delete the corresponding instance. (different from add case)
		// handling a case when a node is not down but demoted.
		logger.Infof("node was successfully removed from the cluster. Now triggering the agent to cleanup the etcd instance...")
		// we set the timout to 10sec since the node might already be failed
		err = clusterd.TriggerAgentsAndWait(context.EtcdClient, []string{candidate}, etcdMgrAgentName, len(candidates), 10)
		if err != nil {
			return fmt.Errorf("error in cleaning up the target etcdmgr agent (this might have happened due to failure of the node): %+v", err)
		}
	}
	return nil
}

// TODO: can we make it more efficient?
func getNodeIDs(nodeURLs []string, Nodes map[string]*inventory.NodeConfig) ([]string, error) {
	logger.Debugf("nodeURLs: %+v", nodeURLs)
	nodeIDs := []string{}
	for _, u := range nodeURLs {
		uu, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		ip, _, _ := net.SplitHostPort(uu.Host)
		for nodeID, config := range Nodes {
			logger.Debugf("nodeID: %s", nodeID)
			if config.PrivateIP == ip {
				logger.Debugf("matched, ip: %v | nodeID: %v", ip, nodeID)
				nodeIDs = append(nodeIDs, nodeID)
			}
		}
	}
	return nodeIDs, nil
}
