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
package mgr

import (
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	ctx "golang.org/x/net/context"
)

const (
	MgrKey   = "cephmgr"
	stateKey = "state"
)

type Leader struct {
}

func NewLeader() *Leader {
	return &Leader{}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
// Initialize CephMGR. Must be idempotent.
func (r *Leader) Configure(context *clusterd.Context) error {
	err := r.enable(context)
	if err != nil {
		return fmt.Errorf("failed to enable the cephmgr. %+v", err)
	}

	return nil
}

// Configure a single instance of the cephmgr in the cluster.
func EnableCephMgr(etcdClient etcd.KeysAPI) error {
	logger.Infof("Enabling cephmgr")
	key := path.Join(mon.CephKey, MgrKey, clusterd.DesiredKey, stateKey)
	_, err := etcdClient.Set(ctx.Background(), key, "1", nil)
	return err
}

// Remove the single instance of the cephmgr from the cluster.
func RemoveObjectStore(etcdClient etcd.KeysAPI) error {
	logger.Infof("Removing cephmgr")
	key := path.Join(mon.CephKey, MgrKey, clusterd.DesiredKey)
	_, err := etcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return fmt.Errorf("failed to remove cephmgr from desired state. %+v", err)
	}

	_, err = etcdClient.Delete(ctx.Background(), getMgrNodesKey(false), &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return fmt.Errorf("failed to remove cephmgr nodes from desired state. %+v", err)
	}

	return nil
}

// Make the cephmgr in the applied state
func markApplied(context *clusterd.Context) error {
	logger.Infof("cephmgr applied")
	key := path.Join(mon.CephKey, MgrKey, clusterd.AppliedKey, stateKey)
	_, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil)
	return err
}

// Remove the cephmgr from the applied state
func markUnapplied(context *clusterd.Context) error {
	logger.Infof("cephmgr removed")
	key := path.Join(mon.CephKey, MgrKey, clusterd.AppliedKey)
	_, err := context.EtcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	return err
}

func (r *Leader) enable(context *clusterd.Context) error {
	// load cluster info
	// TODO: Support multiple mgrs
	mgrCount := 1
	mgrName := "1"
	logger.Infof("Enabling cephgr on %d node(s)", mgrCount)
	cluster, err := mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info. %+v", err)
	}

	// create the keyring if needed
	err = createKeyringOnInit(context, cluster.Name, mgrName)
	if err != nil {
		return fmt.Errorf("failed to init cephmgr keyring. %+v", err)
	}

	// start an instance of cephmgr on every node
	count := len(context.Inventory.Nodes)
	nodes, err := r.getDesiredMgrNodes(context, count)
	if err != nil {
		return fmt.Errorf("failed to get desired cephmgr nodes. %+v", err)
	}

	// trigger the cephmgr to start on each node
	logger.Infof("Triggering cephmgr on nodes: %+v", nodes)
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, nodes, MgrKey, len(nodes))
	if err != nil {
		return fmt.Errorf("failed to deploy cephmgr agents. %+v", err)
	}

	// set the cephmgr as applied
	for _, node := range nodes {
		if err := setMgrState(context.EtcdClient, node, true); err != nil {
			return fmt.Errorf("failed to set cephmgr agent as applied. %+v", err)
		}
	}

	return markApplied(context)
}

func (r *Leader) remove(context *clusterd.Context) error {

	mgrNodes, err := util.GetDirChildKeys(context.EtcdClient, getMgrNodesKey(true))
	if err != nil {
		return fmt.Errorf("failed to get desired cephmgr instances. %+v", err)
	}

	// trigger the cephmgr to be removed from each node
	nodes := mgrNodes.ToSlice()
	logger.Infof("Triggering removal of cephmgr from nodes: %+v", nodes)
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, nodes, MgrKey, len(nodes))
	if err != nil {
		return fmt.Errorf("failed to remove cephmgr agents. %+v", err)
	}

	// remove the cephmgr from applied
	for _, cephmgr := range nodes {
		if err := removeMgrState(context.EtcdClient, cephmgr, true); err != nil {
			return fmt.Errorf("failed to remove cephmgr agent as applied. %+v", err)
		}
	}

	return markUnapplied(context)
}

// create a keyring for the cephmgr client with a limited set of privileges
func createKeyringOnInit(context *clusterd.Context, clusterName, name string) error {
	key := getKeyringKey()
	_, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err == nil {
		// the keyring has already been created
		return nil
	}

	if !util.IsEtcdKeyNotFound(err) {
		return fmt.Errorf("cephmgr keyring could not be retrieved. %+v", err)
	}

	// create the keyring
	keyring, err := CreateKeyring(context, clusterName, name)
	if err != nil {
		return fmt.Errorf("failed to create keyring. %+v", err)
	}

	// save the keyring to etcd
	_, err = context.EtcdClient.Set(ctx.Background(), key, keyring, nil)
	if err != nil {
		return fmt.Errorf("failed to save cephmgr keyring. %+v", err)
	}
	return nil
}

func getKeyringKey() string {
	return path.Join(mon.CephKey, MgrKey, clusterd.DesiredKey, "keyring")
}

func (r *Leader) getDesiredMgrNodes(context *clusterd.Context, count int) ([]string, error) {

	nodes, err := util.GetDirChildKeys(context.EtcdClient, getMgrNodesKey(false))
	if err != nil {
		return nil, fmt.Errorf("failed to load desired cephmgr nodes. %+v", err)
	}

	// Assign cephmgr to nodes if not already assigned
	for nodeID := range context.Inventory.Nodes {
		// we have enough cephmgr instances
		if nodes.Count() >= count {
			break
		}

		// cannot use the same node for more than one cephmgr
		if nodes.Contains(nodeID) {
			continue
		}

		nodes.Add(nodeID)
		if err := setMgrState(context.EtcdClient, nodeID, false); err != nil {
			return nil, err
		}
	}

	if nodes.Count() < count {
		return nil, fmt.Errorf("not enough nodes for cephmgr services. required=%d, actual=%d", count, nodes.Count())
	}

	return nodes.ToSlice(), nil
}

func getKeyringProperties(name string) (string, []string) {
	username := fmt.Sprintf("mgr.%s", name)
	access := []string{"mon", "allow *"}
	return username, access
}

// create a keyring for the mds client with a limited set of privileges
func CreateKeyring(context *clusterd.Context, clusterName, name string) (string, error) {
	// get-or-create-key for the user account
	username, access := getKeyringProperties(name)
	keyring, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return keyring, nil
}
