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
package cephmgr

import (
	"path"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	cephName         = "ceph"
	cephKey          = "/rook/services/ceph"
	cephInstanceName = "default"
	desiredKey       = "desired"
	appliedKey       = "applied"
)

type ClusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
	Name          string
	Monitors      map[string]*CephMonitorConfig
}

// create a new ceph service
func NewCephService(factory client.ConnectionFactory, devices string, forceFormat bool, location, adminSecret string) *clusterd.ClusterService {
	return &clusterd.ClusterService{
		Name:   cephName,
		Leader: newCephLeader(factory, adminSecret),
		Agents: []clusterd.ServiceAgent{
			&monAgent{factory: factory},
			newOSDAgent(factory, devices, forceFormat, location),
		},
	}
}

// attempt to load any previously created and saved cluster info
func LoadClusterInfo(etcdClient etcd.KeysAPI) (*ClusterInfo, error) {
	resp, err := etcdClient.Get(ctx.Background(), path.Join(cephKey, "fsid"), nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	fsid := resp.Node.Value

	name, err := GetClusterName(etcdClient)
	if err != nil {
		return nil, err
	}

	secretsKey := path.Join(cephKey, "_secrets")

	resp, err = etcdClient.Get(ctx.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return nil, err
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(ctx.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return nil, err
	}
	adminSecret := resp.Node.Value

	cluster := &ClusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          name,
	}

	// Get the monitors that have been applied in a previous orchestration
	cluster.Monitors, err = GetDesiredMonitors(etcdClient)

	return cluster, nil
}

func GetClusterName(etcdClient etcd.KeysAPI) (string, error) {
	resp, err := etcdClient.Get(ctx.Background(), path.Join(cephKey, "name"), nil)
	if err != nil {
		return "", err
	}
	return resp.Node.Value, nil
}
