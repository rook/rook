/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package object

import (
	"encoding/json"
	"testing"
	"time"

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestExtractJson(t *testing.T) {
	s := "invalid json"
	_, err := extractJSON(s)
	assert.Error(t, err)

	s = `{"test": "test"}`
	match, err := extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `this line can't be parsed as json
{"test": "test"}`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `this line can't be parsed as json
{"test":
"test"}`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	s = `{"test": "test"}
this line can't be parsed as json`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))

	// complex example with array inside an object
	s = `this line can't be parsed as json
{
	"array":
		[
			"test",
			"test"
		]
}
this line can't be parsed as json
`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))
	assert.Equal(t, `{
	"array":
		[
			"test",
			"test"
		]
}`, match)

	s = `[{"test": "test"}]`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))
	assert.Equal(t, `[{"test": "test"}]`, match)

	s = `this line can't be parsed as json
[{"test": "test"}]`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))
	assert.Equal(t, `[{"test": "test"}]`, match)

	// complex example with array of objects
	s = `this line can't be parsed as json
[
	{
		"one": 1,
		"two": 2
	},
	{
		"three": 3,
		"four": 4
	}
]
this line can't be parsed as json
`
	match, err = extractJSON(s)
	assert.NoError(t, err)
	assert.True(t, json.Valid([]byte(match)))
	assert.Equal(t, `[
	{
		"one": 1,
		"two": 2
	},
	{
		"three": 3,
		"four": 4
	}
]`, match)
}

func TestRunAdminCommandNoMultisite(t *testing.T) {
	objContext := &Context{
		Context:     &clusterd.Context{RemoteExecutor: exec.RemotePodCommandExecutor{ClientSet: test.New(t, 3)}},
		clusterInfo: client.AdminClusterInfo("mycluster"),
	}

	t.Run("no network provider - we run the radosgw-admin command from the operator", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if args[0] == "zone" {
					return `{
		"id": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"name": "realm-a",
		"current_period": "df665ecb-1762-47a9-9c66-f938d251c02a",
		"epoch": 2
	}`, nil
				}
				return "", nil
			},
		}

		objContext.Context.Executor = executor
		_, err := RunAdminCommandNoMultisite(objContext, true, []string{"zone", "get"}...)
		assert.NoError(t, err)
	})

	t.Run("with multus - we use the remote executor", func(t *testing.T) {
		objContext.CephClusterSpec = v1.ClusterSpec{Network: v1.NetworkSpec{Provider: "multus"}}
		_, err := RunAdminCommandNoMultisite(objContext, true, []string{"zone", "get"}...)
		assert.Error(t, err)

		// This is not the best but it shows we go through the right codepath
		assert.EqualError(t, err, "no pods found with selector \"rook-ceph-mgr\"")
	})
}
