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

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	DNSName         = "rook-ceph-rgw"
	RGWPort         = 53390
	rgwAgentName    = "rgw"
	keyringTemplate = `[client.radosgw.gateway]
	key = %s
	caps mon = "allow rw"
	caps osd = "allow *"
`
)

type rgwAgent struct {
	context *clusterd.Context
	rgwProc *proc.MonitoredProc
}

func NewAgent() *rgwAgent {
	return &rgwAgent{}
}

func (a *rgwAgent) Name() string {
	return rgwAgentName
}

// set the desired state in etcd
func (a *rgwAgent) Initialize(context *clusterd.Context) error {
	a.context = context
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

func (a *rgwAgent) startRGW(context *clusterd.Context, cluster *mon.ClusterInfo) error {

	// retrieve the keyring
	val, err := context.EtcdClient.Get(ctx.Background(), getKeyringKey(), nil)
	if err != nil {
		return fmt.Errorf("failed to retrieve the keyring")
	}
	keyring := val.Node.Value

	config := &Config{Keyring: keyring, ClusterInfo: cluster, Host: DNSName, Port: RGWPort}
	err = generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate rgw config files. %+v", err)
	}

	rgwProc, err := startRGW(context, config)
	if err != nil {
		return fmt.Errorf("failed to start rgw daemon: %+v", err)
	}

	if rgwProc != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.rgwProc = rgwProc
	}

	return nil
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
