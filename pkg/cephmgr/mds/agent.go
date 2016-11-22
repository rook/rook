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
	"regexp"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
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
	factory client.ConnectionFactory
	mdsProc *proc.MonitoredProc
}

func NewAgent(factory client.ConnectionFactory) *mdsAgent {
	return &mdsAgent{factory: factory}
}

func (a *mdsAgent) Name() string {
	return mdsAgentName
}

// set the desired state in etcd
func (a *mdsAgent) Initialize(context *clusterd.Context) error {
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

	// Connect to the ceph cluster
	conn, err := mon.ConnectToClusterAsAdmin(context, a.factory, cluster)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster. %+v", err)
	}
	defer conn.Shutdown()

	err = createMDSKeyring(conn, mdsID, context.ConfigDir)
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

// create a keyring for the mds client with a limited set of privileges
func createMDSKeyring(conn client.Connection, id, configDir string) error {
	username := fmt.Sprintf("mds.%s", id)
	keyringPath := getMDSKeyringPath(configDir, id)
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}
	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, id, key)
	}

	return mon.CreateKeyring(conn, username, keyringPath, access, keyringEval)
}

func (a *mdsAgent) startMDS(context *clusterd.Context, cluster *mon.ClusterInfo, id string) error {

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting mds %s", id)
	mdsNameArg := fmt.Sprintf("--name=mds.%s", id)
	mdsProc, err := context.ProcMan.Start(
		fmt.Sprintf("mds%s", id),
		"mds",
		regexp.QuoteMeta(mdsNameArg),
		proc.ReuseExisting,
		"--foreground",
		mdsNameArg,
		fmt.Sprintf("--cluster=%s", cluster.Name),
		fmt.Sprintf("--conf=%s", getMDSConfFilePath(context.ConfigDir, id, cluster.Name)),
		fmt.Sprintf("--keyring=%s", getMDSKeyringPath(context.ConfigDir, id)),
		"-i", id)
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

func getMDSConfDir(dir, id string) string {
	return path.Join(dir, fmt.Sprintf("mds%s", id))
}

func getMDSConfFilePath(dir, id, clusterName string) string {
	return path.Join(getMDSConfDir(dir, id), fmt.Sprintf("%s.config", clusterName))
}

func getMDSKeyringPath(dir, id string) string {
	return path.Join(getMDSConfDir(dir, id), "keyring")
}
