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
	"io/ioutil"
	"path"
	"testing"

	"github.com/rook/rook/pkg/ceph/mon"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"

	"os"

	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "fs get",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "fs get","fs_name": fsName,})
	CephFilesystemGetResponseRaw = `{"mdsmap":{"epoch":6,"flags":1,"ever_allowed_features":0,"explicitly_allowed_features":0,"created":"2016-11-30 08:35:06.416438","modified":"2016-11-30 08:35:06.416438","tableserver":0,"root":0,"session_timeout":60,"session_autoclose":300,"max_file_size":1099511627776,"last_failure":0,"last_failure_osd_epoch":0,"compat":{"compat":{},"ro_compat":{},"incompat":{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs","feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap","feature_8":"file layout v2"}},"max_mds":1,"in":[0],"up":{"mds_0":4107},"failed":[],"damaged":[],"stopped":[],"info":{"gid_4107":{"gid":4107,"name":"1","rank":0,"incarnation":4,"state":"up:active","state_seq":3,"addr":"127.0.0.1:6804\/2981621686","standby_for_rank":-1,"standby_for_fscid":-1,"standby_for_name":"","standby_replay":false,"export_targets":[],"features":1152921504336314367}},"data_pools":[1],"metadata_pool":2,"enabled":true,"fs_name":"myfs1","balancer":""},"id":1}`
)

func TestDefaultDesiredState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	fsr := model.FilesystemRequest{Name: "myfs1", PoolName: "myfs1-pool"}
	fs := NewFS(context, fsr.Name, fsr.PoolName)
	err := fs.AddToDesiredState()
	assert.Nil(t, err)
	assert.Equal(t, fsr.PoolName, etcdClient.GetValue(fmt.Sprintf("/rook/services/ceph/fs/desired/%s/pool", fsr.Name)))

	err = RemoveFileSystem(context, fsr)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/fs/desired").Count())
}

func TestMarkApplied(t *testing.T) {
	leader := &Leader{}
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	fileSystems, err := leader.loadFileSystems(context, true)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fileSystems))

	fs := NewFS(context, "myfs", "mypool")

	err = fs.markApplied()
	assert.Nil(t, err)

	assert.Equal(t, "mypool", etcdClient.GetValue("/rook/services/ceph/fs/applied/myfs/pool"))

	fileSystems, err = leader.loadFileSystems(context, true)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(fileSystems))
	assert.Equal(t, "myfs", fileSystems["myfs"].ID)
	assert.Equal(t, "mypool", fileSystems["myfs"].Pool)
}

func TestAddFileToDesired(t *testing.T) {
	leader := &Leader{}
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	fileSystems, err := leader.loadFileSystems(context, false)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fileSystems))

	fs := NewFS(context, "myfs", "yourpool")
	err = fs.AddToDesiredState()
	assert.Nil(t, err)

	assert.Equal(t, "yourpool", etcdClient.GetValue("/rook/services/ceph/fs/desired/myfs/pool"))

	fileSystems, err = leader.loadFileSystems(context, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(fileSystems))
	assert.Equal(t, "myfs", fileSystems["myfs"].ID)
	assert.Equal(t, "yourpool", fileSystems["myfs"].Pool)
}

func TestAddRemoveFileSystem(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	executor := &exectest.MockExecutor{}
	configDir, _ := ioutil.TempDir("", "")
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv}, Executor: executor, ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)

	fs := NewFS(context, "myfs", "yourpool")
	err := fs.AddToDesiredState()
	assert.Nil(t, err)

	cephtest.CreateClusterInfo(etcdClient, path.Join(configDir, "rookcluster"), []string{"a"})

	_, err = mon.CreateClusterInfo(context, "secret")
	assert.Nil(t, err)

	// fail when there are no nodes
	leader := &Leader{}
	err = leader.Configure(context)
	assert.NotNil(t, err)

	// add a couple nodes to choose from
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4"}
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.2.3.4"}
	etcdClient.WatcherResponses["/rook/_notify/a/mds/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/mds/status"] = "succeeded"

	// succeed when there are nodes
	err = leader.Configure(context)
	assert.Nil(t, err)

	// the chosen MDS node can vary, find which was chosen then use that for later asserts
	chosenMdsNodes := etcdClient.GetChildDirs("/rook/services/ceph/mds/desired/node")
	assert.Equal(t, 1, chosenMdsNodes.Count())
	var chosenNode string
	for n := range chosenMdsNodes.Iter() {
		chosenNode = n
		break
	}
	assert.Equal(t, "1", etcdClient.GetValue(fmt.Sprintf("/rook/services/ceph/mds/desired/node/%s/id", chosenNode)))
	assert.Equal(t, "1", etcdClient.GetValue(fmt.Sprintf("/rook/services/ceph/mds/applied/node/%s/id", chosenNode)))

	// remove the file system
	err = fs.DeleteFromDesiredState()
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/desired/node").Count())
	assert.Equal(t, "1", etcdClient.GetValue(fmt.Sprintf("/rook/services/ceph/mds/applied/node/%s/id", chosenNode)))

	// deletion of a file system has more complicated interaction with ceph, set up a
	// MockMonCommand to pass back mocked ceph data and verify the calls we are making.
	monCmdCount := 0
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("RUN %s %+v", command, args)
			result := ""
			switch monCmdCount {
			case 0:
				assert.Equal(t, args[0], "fs")
				assert.Equal(t, args[1], "set")
				assert.Equal(t, args[2], "myfs")
				assert.Equal(t, args[3], "cluster_down")
			case 1:
				assert.Equal(t, args[0], "fs")
				assert.Equal(t, args[1], "get")
				result = CephFilesystemGetResponseRaw
			case 2:
				assert.Equal(t, args[0], "mds")
				assert.Equal(t, args[1], "fail")
				assert.Equal(t, args[2], "4107")
			case 3:
				assert.Equal(t, args[0], "fs")
				assert.Equal(t, args[1], "rm")
				assert.Equal(t, args[2], "myfs")
			}

			monCmdCount++
			return result, nil
		},
	}
	context.Executor = executor

	err = leader.Configure(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/desired/node").Count())
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/applied/node").Count())
}
