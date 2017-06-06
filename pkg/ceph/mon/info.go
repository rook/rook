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
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"
)

const (
	CephKey = "/rook/services/ceph"
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

func createOrGetClusterInfo(context *clusterd.Context, adminSecret string) (*ClusterInfo, error) {
	// load any existing cluster info that may have previously been created
	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if cluster == nil {
		// the cluster info is not yet set, go ahead and set it now
		cluster, err = CreateClusterInfo(context, adminSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster info: %+v", err)
		}

		logger.Infof("Created new cluster info: %+v", cluster)
		err = saveClusterInfo(cluster, context.EtcdClient)
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
func CreateClusterInfo(context *clusterd.Context, adminSecret string) (*ClusterInfo, error) {
	return CreateNamedClusterInfo(context, adminSecret, "rookcluster")
}

// create new cluster info (FSID, shared keys)
func CreateNamedClusterInfo(context *clusterd.Context, adminSecret, clusterName string) (*ClusterInfo, error) {
	fsid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	dir := path.Join(context.ConfigDir, clusterName)
	if err = os.MkdirAll(dir, 0744); err != nil {
		return nil, fmt.Errorf("failed to create dir %s. %+v", dir, err)
	}

	// generate the mon secret
	monSecret, err := genSecret(context.Executor, dir, "mon.", []string{"--cap", "mon", "'allow *'"})
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	if adminSecret == "" {
		args := []string{"--set-uid=0", "--cap", "mon", "'allow *'", "--cap", "osd", "'allow *'", "--cap", "mds", "'allow'"}
		adminSecret, err = genSecret(context.Executor, dir, client.AdminUsername, args)
		if err != nil {
			return nil, err
		}
	}

	return &ClusterInfo{
		FSID:          fsid.String(),
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          clusterName,
	}, nil
}

func genSecret(executor exec.Executor, configDir, name string, args []string) (string, error) {
	path := path.Join(configDir, fmt.Sprintf("%s.keyring", name))
	path = strings.Replace(path, "..", ".", 1)
	base := []string{
		"--create-keyring",
		path,
		"--gen-key",
		"-n", name,
	}
	args = append(base, args...)
	_, err := executor.ExecuteCommandWithOutput("gen secret", "ceph-authtool", args...)
	if err != nil {
		return "", fmt.Errorf("failed to gen secret. %+v", err)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file. %+v", err)
	}
	return extractKey(string(contents))
}

func extractKey(contents string) (string, error) {
	secret := sys.Awk(sys.Grep(string(contents), "key"), 3)
	if secret == "" {
		return "", fmt.Errorf("failed to parse secret")
	}
	return secret, nil
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
