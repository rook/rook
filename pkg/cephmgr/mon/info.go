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
package mon

import (
	"fmt"
	"path"
	"strings"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/util"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"
)

const (
	CephKey          = "/rook/services/ceph"
	cephInstanceName = "default"
)

type ClusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
	Name          string
	Monitors      map[string]*CephMonitorConfig
}

func (c *ClusterInfo) MonEndpoints() string {
	var endpoints []string
	for _, mon := range c.Monitors {
		endpoints = append(endpoints, fmt.Sprintf("%s-%s", mon.Name, mon.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

func createOrGetClusterInfo(factory client.ConnectionFactory, etcdClient etcd.KeysAPI, adminSecret string) (*ClusterInfo, error) {
	// load any existing cluster info that may have previously been created
	cluster, err := LoadClusterInfo(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if cluster == nil {
		// the cluster info is not yet set, go ahead and set it now
		cluster, err = CreateClusterInfo(factory, adminSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster info: %+v", err)
		}

		logger.Infof("Created new cluster info: %+v", cluster)
		err = saveClusterInfo(cluster, etcdClient)
		if err != nil {
			return nil, fmt.Errorf("failed to save new cluster info: %+v", err)
		}
	} else {
		// the cluster has already been created
		logger.Debugf("Cluster already exists: %+v", cluster)
	}

	return cluster, nil
}

// create new cluster info (FSID, shared keys)
func CreateClusterInfo(factory client.ConnectionFactory, adminSecret string) (*ClusterInfo, error) {
	fsid, err := factory.NewFsid()
	if err != nil {
		return nil, err
	}

	monSecret, err := factory.NewSecretKey()
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	if adminSecret == "" {
		adminSecret, err = factory.NewSecretKey()
		if err != nil {
			return nil, err
		}
	}

	return &ClusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          "rookcluster",
	}, nil
}

// save the given cluster info to the key value store
func saveClusterInfo(c *ClusterInfo, etcdClient etcd.KeysAPI) error {
	_, err := etcdClient.Set(ctx.Background(), path.Join(CephKey, "fsid"), c.FSID, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(ctx.Background(), path.Join(CephKey, "name"), c.Name, nil)
	if err != nil {
		return err
	}

	secretsKey := path.Join(CephKey, "_secrets")

	_, err = etcdClient.Set(ctx.Background(), path.Join(secretsKey, "monitor"), c.MonitorSecret, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(ctx.Background(), path.Join(secretsKey, "admin"), c.AdminSecret, nil)
	if err != nil {
		return err
	}

	return nil
}

// attempt to load any previously created and saved cluster info
func LoadClusterInfo(etcdClient etcd.KeysAPI) (*ClusterInfo, error) {
	resp, err := etcdClient.Get(ctx.Background(), path.Join(CephKey, "fsid"), nil)
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

	secretsKey := path.Join(CephKey, "_secrets")

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
	resp, err := etcdClient.Get(ctx.Background(), path.Join(CephKey, "name"), nil)
	if err != nil {
		return "", err
	}
	return resp.Node.Value, nil
}
