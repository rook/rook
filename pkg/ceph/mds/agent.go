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
package mds

import (
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	mdsAgentName    = "mds"
	keyringTemplate = `
[mds.%s]
	key = %s
	caps mon = "allow profile mds"
	caps osd = "allow *"
	caps mds = "allow"
`
)

type mdsAgent struct {
	context *clusterd.Context
	mdsProc *proc.MonitoredProc
}

func NewAgent() *mdsAgent {
	return &mdsAgent{}
}

func (a *mdsAgent) Name() string {
	return mdsAgentName
}

// set the desired state in etcd
func (a *mdsAgent) Initialize(context *clusterd.Context) error {
	a.context = context
	return nil
}

// configure an MDS on the local node if it is in the desired state
func (a *mdsAgent) ConfigureLocalService(context *clusterd.Context) error {
	mdsID, err := getMDSID(context.EtcdClient, context.NodeID)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			// the mds is not in the desired state on this node, so ensure it is not running
			return a.DestroyLocalService(context)
		}

		return fmt.Errorf("failed to load mds id. %+v", err)
	}

	cluster, err := mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info. %+v", err)
	}

	_, err = mon.GenerateConnectionConfigFile(context, cluster, getMDSConfDir(context.ConfigDir, mdsID),
		fmt.Sprintf("mds.%s", mdsID), getMDSKeyringPath(context.ConfigDir, mdsID))
	if err != nil {
		return fmt.Errorf("failed to create mds config file. %+v", err)
	}

	err = createKeyringAndSave(context, cluster.Name, mdsID, context.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	err = a.startMDS(context, cluster, mdsID)
	if err != nil {
		return fmt.Errorf("failed to start mds process. %+v", err)
	}

	return nil
}

func (a *mdsAgent) DestroyLocalService(context *clusterd.Context) error {
	if a.mdsProc == nil {
		logger.Debugf("no need to stop mds that is not running")
		return nil
	}

	if err := a.mdsProc.Stop(); err != nil {
		return fmt.Errorf("failed to stop mds. %+v", err)
	}

	logger.Infof("stopped ceph mds")
	a.mdsProc = nil

	// TODO: Clean up the mds folder
	return nil
}

func getKeyringProperties(id string) (string, []string) {
	username := fmt.Sprintf("mds.%s", id)
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}
	return username, access
}

// create a keyring for the mds client with a limited set of privileges
func createKeyringAndSave(context *clusterd.Context, clusterName, id, configDir string) error {
	username, access := getKeyringProperties(id)
	keyringPath := getMDSKeyringPath(configDir, id)
	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, id, key)
	}

	return mon.CreateKeyring(context, clusterName, username, keyringPath, access, keyringEval)
}

// create a keyring for the mds client with a limited set of privileges
func CreateKeyring(context *clusterd.Context, hostName, id string) (string, error) {
	// get-or-create-key for the user account
	username, access := getKeyringProperties(id)
	keyring, err := client.AuthGetOrCreateKey(context, hostName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return keyring, nil
}

func (a *mdsAgent) startMDS(context *clusterd.Context, cluster *mon.ClusterInfo, id string) error {

	config := &Config{ID: id, InProc: false, ClusterInfo: cluster}
	mdsProc, err := startMDS(context, config)
	if err != nil {
		return fmt.Errorf("failed to start mds %s: %+v", id, err)
	}

	if mdsProc != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.mdsProc = mdsProc
	}

	return nil
}

func getMDSID(etcdClient etcd.KeysAPI, nodeID string) (string, error) {
	res, err := etcdClient.Get(ctx.Background(), getMDSIDKey(nodeID, false), nil)
	if err != nil {
		return "", err
	}

	return res.Node.Value, nil
}

func setMDSID(etcdClient etcd.KeysAPI, nodeID, mdsID string) error {
	_, err := etcdClient.Set(ctx.Background(), getMDSIDKey(nodeID, false), mdsID, nil)
	if err != nil {
		return err
	}

	return nil
}

func getMDSDesiredNodesKey(applied bool) string {
	a := clusterd.DesiredKey
	if applied {
		a = clusterd.AppliedKey
	}

	return path.Join(mon.CephKey, mdsAgentName, a, "node")
}

func getMDSIDKey(nodeID string, applied bool) string {
	return path.Join(getMDSDesiredNodesKey(applied), nodeID, "id")
}

func getMDSFileSystemKey(nodeID string, applied bool) string {
	return path.Join(getMDSDesiredNodesKey(applied), nodeID, "filesystem")
}
