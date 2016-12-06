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
	"io/ioutil"
	"os"
	"path"
	"regexp"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"strconv"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	DNSName         = "rook-rgw"
	RGWPort         = 53390
	rgwAgentName    = "rgw"
	keyringTemplate = `[client.radosgw.gateway]
	key = %s
	caps mon = "allow rw"
	caps osd = "allow *"
`
)

type rgwAgent struct {
	factory client.ConnectionFactory
	rgwProc *proc.MonitoredProc
}

func NewAgent(factory client.ConnectionFactory) *rgwAgent {
	return &rgwAgent{factory: factory}
}

func (a *rgwAgent) Name() string {
	return rgwAgentName
}

// set the desired state in etcd
func (a *rgwAgent) Initialize(context *clusterd.Context) error {
	return nil
}

// configure RGW on the local node if it is in the desired state
func (a *rgwAgent) ConfigureLocalService(context *clusterd.Context) error {
	// check if the rgw is in desired state on this node
	desired, err := getRGWState(context.EtcdClient, context.NodeID, false)
	if (err != nil && util.IsEtcdKeyNotFound(err)) || (err == nil && !desired) {
		// the rgw is not in the desired state on this node, so ensure it is not running
		return a.DestroyLocalService(context)
	} else if err != nil {
		return fmt.Errorf("failed to load rgw state. %+v", err)
	}

	// load cluster info
	cluster, err := mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info. %+v", err)
	}

	err = a.generateConfigFiles(context, cluster)
	if err != nil {
		return fmt.Errorf("failed to generate config. %+v", err)
	}

	// start rgw
	err = a.startRGW(context, cluster)
	if err != nil {
		return fmt.Errorf("failed to start rgw process. %+v", err)
	}

	return nil
}

func (a *rgwAgent) DestroyLocalService(context *clusterd.Context) error {
	if a.rgwProc == nil {
		logger.Debugf("no need to stop rgw that is not running")
		return nil
	}

	if err := a.rgwProc.Stop(); err != nil {
		return fmt.Errorf("failed to stop rgw. %+v", err)
	}

	logger.Infof("stopped ceph rgw")
	a.rgwProc = nil

	return nil
}

func (a *rgwAgent) generateConfigFiles(context *clusterd.Context, cluster *mon.ClusterInfo) error {
	// create the rgw data directory
	dataDir := path.Join(getRGWConfDir(context.ConfigDir), "data")
	if err := os.MkdirAll(dataDir, 0744); err != nil {
		logger.Warningf("failed to create rgw data directory %s: %+v", dataDir, err)
	}

	settings := map[string]string{
		"host":                           DNSName,
		"rgw port":                       strconv.Itoa(RGWPort),
		"rgw data":                       dataDir,
		"rgw dns name":                   fmt.Sprintf("%s:%d", DNSName, RGWPort),
		"rgw log nonexistent bucket":     "true",
		"rgw intent log object name utc": "true",
		"rgw enable usage log":           "true",
	}
	_, err := mon.GenerateConfigFile(context, cluster, getRGWConfDir(context.ConfigDir),
		"client.radosgw.gateway", getRGWKeyringPath(context.ConfigDir), false, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create mds config file. %+v", err)
	}

	// connect to the ceph cluster
	conn, err := mon.ConnectToClusterAsAdmin(context, a.factory, cluster)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster. %+v", err)
	}
	defer conn.Shutdown()

	// create rgw config
	err = createRGWKeyring(conn, context.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// write the mime types config
	mimeTypesPath := getMimeTypesPath(context.ConfigDir)
	logger.Debugf("Writing mime types to: %s", mimeTypesPath)
	if err := ioutil.WriteFile(mimeTypesPath, []byte(mimeTypes), 0644); err != nil {
		return fmt.Errorf("failed to write mime types to %s: %+v", mimeTypesPath, err)
	}

	return nil
}

// create a keyring for the rgw client with a limited set of privileges
func createRGWKeyring(conn client.Connection, configDir string) error {
	username := "client.radosgw.gateway"
	keyringPath := getRGWKeyringPath(configDir)
	access := []string{"osd", "allow rwx", "mon", "allow rw"}
	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, key)
	}

	return mon.CreateKeyring(conn, username, keyringPath, access, keyringEval)
}

func (a *rgwAgent) startRGW(context *clusterd.Context, cluster *mon.ClusterInfo) error {

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting rgw")
	rgwNameArg := "--name=client.radosgw.gateway"
	rgwProc, err := context.ProcMan.Start(
		"rgw",
		"rgw",
		regexp.QuoteMeta(rgwNameArg),
		proc.ReuseExisting,
		"--foreground",
		rgwNameArg,
		fmt.Sprintf("--cluster=%s", cluster.Name),
		fmt.Sprintf("--conf=%s", getRGWConfFilePath(context.ConfigDir, cluster.Name)),
		fmt.Sprintf("--keyring=%s", getRGWKeyringPath(context.ConfigDir)),
		fmt.Sprintf("--rgw-frontends=civetweb port=%d", RGWPort),
		fmt.Sprintf("--rgw-mime-types-file=%s", getMimeTypesPath(context.ConfigDir)))
	if err != nil {
		return fmt.Errorf("failed to start rgw: %+v", err)
	}

	if rgwProc != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.rgwProc = rgwProc
	}

	return nil
}

func getRGWConfFilePath(configDir, clusterName string) string {
	return path.Join(getRGWConfDir(configDir), fmt.Sprintf("%s.config", clusterName))
}

func getRGWConfDir(configDir string) string {
	return path.Join(configDir, "rgw")
}

func getRGWKeyringPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "keyring")
}

func getMimeTypesPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "mime.types")
}

func getRGWState(etcdClient etcd.KeysAPI, nodeID string, applied bool) (bool, error) {
	key := path.Join(getRGWNodeKey(nodeID, applied), "state")
	res, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return res.Node.Value == "1", nil
}

func setRGWState(etcdClient etcd.KeysAPI, nodeID string, applied bool) error {
	key := path.Join(getRGWNodeKey(nodeID, applied), "state")
	_, err := etcdClient.Set(ctx.Background(), key, "1", nil)
	if err != nil {
		return err
	}

	return nil
}

func removeRGWState(etcdClient etcd.KeysAPI, nodeID string, applied bool) error {
	key := path.Join(getRGWNodeKey(nodeID, applied))
	_, err := etcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return err
	}

	return nil
}

func getRGWNodesKey(applied bool) string {
	a := clusterd.DesiredKey
	if applied {
		a = clusterd.AppliedKey
	}

	return path.Join(mon.CephKey, rgwAgentName, a, "node")
}

func getRGWNodeKey(nodeID string, applied bool) string {
	return path.Join(getRGWNodesKey(applied), nodeID)
}
