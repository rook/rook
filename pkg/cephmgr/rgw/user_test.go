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
	"os"
	"testing"

	cephtest "github.com/rook/rook/pkg/cephmgr/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"

	"github.com/stretchr/testify/assert"
)

func TestBuiltinUser(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{EtcdClient: etcdClient,
		ProcMan: proc.New(executor), Executor: executor, ConfigDir: "/tmp/rgw"}
	response := `{
        "user_id": "testadmin",
        "display_name": "my display name",
        "max_buckets": 1000,
        "auid": 0,
        "subusers": [],
        "keys": [
            {
                "user": "rookadmin",
                "access_key": "myaccessid",
                "secret_key": "mybigsecretkey"
            }
        ],
        "temp_url_keys": [],
        "type": "rgw"
    }`
	executor.MockExecuteCommandWithOutput = func(actionName, command string, args ...string) (string, error) {
		assert.Equal(t, "user", args[3])
		assert.Equal(t, "create", args[4])
		assert.Equal(t, 10, len(args))

		return response, nil
	}
	defer os.RemoveAll("/tmp/rgw")

	// mock a monitor
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	// create a valid user
	err := createBuiltinUser(context)
	assert.Nil(t, err)
	assert.Equal(t, "myaccessid", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/id"))
	assert.Equal(t, "mybigsecretkey", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/_secret"))

	// test various forms of invalid responses for the user
	response = "{}"
	err = createBuiltinUser(context)
	assert.NotNil(t, err)

	// no keys
	response = `"{"keys": []}`
	err = createBuiltinUser(context)
	assert.NotNil(t, err)

	// no access key
	response = `"{"keys": [
        {
            "accss_key": "foo",
            "secret_key": "bar"
        }
    ]}`
	err = createBuiltinUser(context)
	assert.NotNil(t, err)

	// no secret key
	response = `"{"keys": [
        {
            "access_key": "foo",
            "secrt_key": "bar"
        }
    ]}`
	err = createBuiltinUser(context)
	assert.NotNil(t, err)
	assert.Equal(t, "myaccessid", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/id"))
	assert.Equal(t, "mybigsecretkey", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/_secret"))

	// valid key
	response = `{
        "keys": [
            {
                "access_key": "foo",
                "secret_key": "bar"
            }
        ]
    }`
	err = createBuiltinUser(context)
	assert.Nil(t, err)
	assert.Equal(t, "foo", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/id"))
	assert.Equal(t, "bar", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/_secret"))
}
