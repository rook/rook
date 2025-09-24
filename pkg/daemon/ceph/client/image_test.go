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
package client

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestListImageLogLevelInfo(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	var images []CephBlockImage
	var err error
	listCalled := false
	emptyListResult := false
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			listCalled = true
			if emptyListResult {
				return `[]`, nil
			} else {
				return `[{"image":"image1","size":1048576,"format":2},{"image":"image2","size":2048576,"format":2},{"image":"image3","size":3048576,"format":2}]`, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	clusterInfo := AdminTestClusterInfo("mycluster")
	images, err = ListImagesInPool(context, clusterInfo, "pool1")
	assert.Nil(t, err)
	assert.NotNil(t, images)
	assert.True(t, len(images) == 3)
	assert.True(t, listCalled)
	listCalled = false

	emptyListResult = true
	images, err = ListImagesInPool(context, clusterInfo, "pool1")
	assert.Nil(t, err)
	assert.NotNil(t, images)
	assert.True(t, len(images) == 0)
	assert.True(t, listCalled)
	listCalled = false
}

func TestListImageLogLevelDebug(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	var images []CephBlockImage
	var err error
	libradosDebugOut := `2017-08-24 19:42:10.693348 7fd64513e0c0  1 librados: starting msgr at -
2017-08-24 19:42:10.693372 7fd64513e0c0  1 librados: starting objecter
2017-08-24 19:42:10.784686 7fd64513e0c0  1 librados: setting wanted keys
2017-08-24 19:42:10.784688 7fd64513e0c0  1 librados: calling monclient init
2017-08-24 19:42:10.789337 7fd64513e0c0  1 librados: init done
2017-08-24 19:42:10.789354 7fd64513e0c0 10 librados: wait_for_osdmap waiting
2017-08-24 19:42:10.790039 7fd64513e0c0 10 librados: wait_for_osdmap done waiting
2017-08-24 19:42:10.790079 7fd64513e0c0 10 librados: read oid=rbd_directory nspace=
2017-08-24 19:42:10.792235 7fd64513e0c0 10 librados: Objecter returned from read r=0
2017-08-24 19:42:10.792307 7fd64513e0c0 10 librados: call oid=rbd_directory nspace=
2017-08-24 19:42:10.793495 7fd64513e0c0 10 librados: Objecter returned from call r=0
2017-08-24 19:42:11.684960 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.884609 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.884628 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.985068 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.985084 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.986275 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.986339 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.986498 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.987363 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:11.988165 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:12.385448 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:12.386804 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
2017-08-24 19:42:12.386877 7fd621ffb700 10 librados: set snap write context: seq = 0 and snaps = []
`

	listCalled := false
	emptyListResult := false
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			listCalled = true
			if emptyListResult {
				return fmt.Sprintf(`%s[]`, libradosDebugOut), nil
			} else {
				return fmt.Sprintf(`%s[{"image":"image1","size":1048576,"format":2},{"image":"image2","size":2048576,"format":2},{"image":"image3","size":3048576,"format":2}]`, libradosDebugOut), nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	clusterInfo := AdminTestClusterInfo("mycluster")
	images, err = ListImagesInPool(context, clusterInfo, "pool1")
	assert.Nil(t, err)
	assert.NotNil(t, images)
	assert.True(t, len(images) == 3)
	assert.True(t, listCalled)
	listCalled = false

	emptyListResult = true
	images, err = ListImagesInPool(context, clusterInfo, "pool1")
	assert.Nil(t, err)
	assert.NotNil(t, images)
	assert.True(t, len(images) == 0)
	assert.True(t, listCalled)
	listCalled = false
}

func TestGetWatchers(t *testing.T) {
	rbdStatus := RBDStatus{
		Watchers: []struct {
			Address string "json:\"address\""
		}{
			{
				Address: "192.168.39.137:0/3762982934",
			},
			{
				Address: "192.168.39.136:0/3762982934",
			},
		},
	}

	res := rbdStatus.GetWatchers()
	assert.Equal(t, 2, len(res))
}
