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
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const (
	sizeMB = MiB // 1 MB
)

func TestRoundup_size_MiB(t *testing.T) {
	assert.Equal(t, uint64(1), roundupSizeMiB(MiB))
	assert.Equal(t, uint64(2), roundupSizeMiB(2*MiB))
	assert.Equal(t, uint64(2), roundupSizeMiB(MiB+1))
	assert.Equal(t, uint64(1), roundupSizeMiB(MiB-1))
}

func TestCreateImage(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	// mock an error during the create image call.  rbd tool returns error information to the output stream,
	// separate from the error object, so verify that information also makes it back to us (because it is useful).
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			return "mocked detailed ceph error output stream", errors.New("some mocked error")
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	clusterInfo := AdminTestClusterInfo("mycluster")
	_, err := CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB)) // 1MB
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "mocked detailed ceph error output stream"))

	// rbd tool interprets sizes as MB, so anything smaller than that should get rounded up to the minimum
	// (except for 0, that's OK)
	createCalled := false
	expectedSizeArg := ""
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			createCalled = true
			assert.Equal(t, expectedSizeArg, args[3])
			return "", nil
		case command == "rbd" && args[0] == "info":
			assert.Equal(t, "pool1/image1", args[1])
			return `{"name":"image1","size":1048576,"objects":1,"order":20,"object_size":1048576,"block_name_prefix":"pool1_data.229226b8b4567",` +
				`"format":2,"features":["layering"],"op_features":[],"flags":[],"create_timestamp":"Fri Oct  5 19:46:20 2018"}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	// 0 byte --> 0 MB
	expectedSizeArg = "0"
	image, err := CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(0))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// 1 byte --> 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// (1 MB - 1 byte) --> 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB-1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	assert.Equal(t, "image1", image.Name)
	assert.Equal(t, uint64(sizeMB), image.Size)
	createCalled = false

	// (1 MB + 1 byte) --> 2 MB
	expectedSizeArg = "2"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB+1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// (2 MB - 1 byte) --> 2 MB
	expectedSizeArg = "2"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB*2-1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// 2 MB
	expectedSizeArg = "2"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB*2))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// (2 MB + 1 byte) --> 3MB
	expectedSizeArg = "3"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "", uint64(sizeMB*2+1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// Pool with data pool
	expectedSizeArg = "1"
	image, err = CreateImage(context, clusterInfo, "image1", "pool1", "datapool1", uint64(sizeMB))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false
}

func TestExpandImage(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		switch {
		case args[1] != "kube/some-image":
			return "", errors.Errorf("no image %s", args[1])

		case command == "rbd" && args[0] == "resize":
			return "everything is okay", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	clusterInfo := AdminTestClusterInfo("mycluster")
	err := ExpandImage(context, clusterInfo, "error-name", "kube", "mon1,mon2,mon3", "/tmp/keyring", 1000000)
	assert.Error(t, err)

	err = ExpandImage(context, clusterInfo, "some-image", "kube", "mon1,mon2,mon3", "/tmp/keyring", 1000000)
	assert.NoError(t, err)
}

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
