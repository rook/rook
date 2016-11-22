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
package rgw

import (
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	RGWKey         = "rgw"
	ObjectStoreKey = "object"
)

type Leader struct {
}

func NewLeader() *Leader {
	return &Leader{}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
// Initialize CephFS. Must be idempotent.
func (r *Leader) Configure(context *clusterd.Context, factory client.ConnectionFactory) error {

	// Check if object store is desired
	desired, err := getObjectStoreState(context, false)
	if err != nil {
		return fmt.Errorf("failed to get desired state. %+v", err)
	}

	// Check if object store is applied
	applied, err := getObjectStoreState(context, true)
	if err != nil {
		return fmt.Errorf("failed to get applied state. %+v", err)
	}

	if desired && !applied {
		err := r.enable(context, factory)
		if err != nil {
			return fmt.Errorf("failed to enable the object store. %+v", err)
		}

	} else if !desired && applied {
		err := r.remove(context, factory)
		if err != nil {
			return fmt.Errorf("failed to remove the object store. %+v", err)
		}
	}

	return nil
}

// Configure the single instance of object storage in the cluster.
func EnableObjectStore(context *clusterd.Context) error {
	logger.Infof("Enabling object store")
	key := path.Join(mon.CephKey, ObjectStoreKey, clusterd.DesiredKey)
	_, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil)
	return err
}

// Remove the single instance of the object store from the cluster. All buckets will be purged..
func RemoveObjectStore(context *clusterd.Context) error {
	logger.Infof("Removing object store")
	key := path.Join(mon.CephKey, ObjectStoreKey, clusterd.DesiredKey)
	_, err := context.EtcdClient.Delete(ctx.Background(), key, nil)
	if err != nil {
		return fmt.Errorf("failed to remove object store from desired state. %+v", err)
	}

	_, err = context.EtcdClient.Delete(ctx.Background(), getRGWNodesKey(false), &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return fmt.Errorf("failed to remove rgw nodes from desired state. %+v", err)
	}

	return nil
}

// Configure the single instance of object storage in the cluster.
func getObjectStoreState(context *clusterd.Context, applied bool) (bool, error) {
	var state string
	if applied {
		state = clusterd.AppliedKey
	} else {
		state = clusterd.DesiredKey
	}

	key := path.Join(mon.CephKey, ObjectStoreKey, state)
	val, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("failed to get object store state. %+v", err)
	}

	return val.Node.Value == "1", nil
}

// Make the object store in the applied state
func markApplied(context *clusterd.Context) error {
	logger.Infof("object store applied")
	key := path.Join(mon.CephKey, ObjectStoreKey, clusterd.AppliedKey)
	_, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil)
	return err
}

// Remove the object store from the applied state
func markUnapplied(context *clusterd.Context) error {
	logger.Infof("object store removed")
	key := path.Join(mon.CephKey, ObjectStoreKey, clusterd.AppliedKey)
	_, err := context.EtcdClient.Delete(ctx.Background(), key, nil)
	return err
}

func (r *Leader) enable(context *clusterd.Context, factory client.ConnectionFactory) error {
	// start an instance of rgw on every node
	count := len(context.Inventory.Nodes)
	nodes, err := r.getDesiredRGWNodes(context, count)
	if err != nil {
		return fmt.Errorf("failed to get desired rgw nodes. %+v", err)
	}

	// trigger the rgw to start on each node
	logger.Infof("Triggering rgw on nodes: %+v", nodes)
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, nodes, rgwAgentName, len(nodes))
	if err != nil {
		return fmt.Errorf("failed to deploy rgw agents. %+v", err)
	}

	// set the rgw as applied
	for _, node := range nodes {
		if err := setRGWState(context.EtcdClient, node, true); err != nil {
			return fmt.Errorf("failed to set rgw agent as applied. %+v", err)
		}
	}

	return markApplied(context)
}

func (r *Leader) remove(context *clusterd.Context, factory client.ConnectionFactory) error {

	rgwNodes, err := util.GetDirChildKeys(context.EtcdClient, getRGWNodesKey(true))
	if err != nil {
		return fmt.Errorf("failed to get desired rgw instances. %+v", err)
	}

	// trigger the rgw to be removed from each node
	nodes := rgwNodes.ToSlice()
	logger.Infof("Triggering removal of rgw from nodes: %+v", nodes)
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, nodes, rgwAgentName, len(nodes))
	if err != nil {
		return fmt.Errorf("failed to remove rgw agents. %+v", err)
	}

	// remove the rgw from applied
	for _, rgw := range nodes {
		if err := removeRGWState(context.EtcdClient, rgw, true); err != nil {
			return fmt.Errorf("failed to remove rgw agent as applied. %+v", err)
		}
	}

	return markUnapplied(context)
}

func (r *Leader) getDesiredRGWNodes(context *clusterd.Context, count int) ([]string, error) {

	nodes, err := util.GetDirChildKeys(context.EtcdClient, getRGWNodesKey(false))
	if err != nil {
		return nil, fmt.Errorf("failed to load desired rgw nodes. %+v", err)
	}

	// Assign rgw to nodes if not already assigned
	for nodeID := range context.Inventory.Nodes {
		// we have enough rgw instances
		if nodes.Count() >= count {
			break
		}

		// cannot use the same node for more than one rgw
		if nodes.Contains(nodeID) {
			continue
		}

		nodes.Add(nodeID)
		if err := setRGWState(context.EtcdClient, nodeID, false); err != nil {
			return nil, err
		}
	}

	if nodes.Count() < count {
		return nil, fmt.Errorf("not enough nodes for rgw services. required=%d, actual=%d", count, nodes.Count())
	}

	return nodes.ToSlice(), nil
}
