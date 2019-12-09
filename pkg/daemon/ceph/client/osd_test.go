/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var (
	fakeOsdTree = `{
		"nodes": [
		  {
			"id": -3,
			"name": "minikube",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [
			  2,
			  1,
			  0
			]
		  },
		  {
			"id": -2,
			"name": "minikube-2",
			"type": "host",
			"type_id": 1,
			"pool_weights": {},
			"children": [
			  3,
			  4,
			  5
			]
		  }
		]
	  }`

	fakeOSdList = `[0,1,2]`
)

func TestHostTree(t *testing.T) {
	executor := &exectest.MockExecutor{}
	emptyTreeResult := false
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "tree":
			if emptyTreeResult {
				return `not a json`, nil
			}
			return fakeOsdTree, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	tree, err := HostTree(&clusterd.Context{Executor: executor}, "rook")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tree.Nodes))
	assert.Equal(t, "minikube", tree.Nodes[0].Name)
	assert.Equal(t, 3, len(tree.Nodes[0].Children))

	emptyTreeResult = true
	tree, err = HostTree(&clusterd.Context{Executor: executor}, "rook")
	assert.Error(t, err)
	assert.Equal(t, 0, len(tree.Nodes))

}

func TestOsdListNum(t *testing.T) {
	executor := &exectest.MockExecutor{}
	emptyOsdListNumResult := false
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "osd" && args[1] == "ls":
			if emptyOsdListNumResult {
				return `not a json`, nil
			}
			return fakeOSdList, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	list, err := OsdListNum(&clusterd.Context{Executor: executor}, "rook")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(list))

	emptyOsdListNumResult = true
	list, err = OsdListNum(&clusterd.Context{Executor: executor}, "rook")
	assert.Error(t, err)
	assert.Equal(t, 0, len(list))
}
