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

	"github.com/pkg/errors"
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
		clusterInfo: client.AdminTestClusterInfo("mycluster"),
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

func TestCommitConfigChanges(t *testing.T) {
	// control the return values from calling get/update on period
	type commandReturns struct {
		periodGetOutput    string // empty implies error
		periodUpdateOutput string // empty implies error
		periodCommitError  bool
	}

	// control whether we should expect certain 'get' calls
	type expectCommands struct {
		// note: always expect period get to be called
		periodUpdate bool
		periodCommit bool
	}

	// vars used to check if commands were called
	var (
		periodGetCalled    = false
		periodUpdateCalled = false
		periodCommitCalled = false
	)

	setupTest := func(returns commandReturns) *clusterd.Context {
		// reset vars for checking if commands were called
		periodGetCalled = false
		periodUpdateCalled = false
		periodCommitCalled = false

		executor := &exectest.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" {
					if args[0] == "period" {
						if args[1] == "get" {
							periodGetCalled = true
							if returns.periodGetOutput == "" {
								return "", errors.New("fake period get error")
							}
							return returns.periodGetOutput, nil
						}
						if args[1] == "update" {
							if args[2] == "--commit" {
								periodCommitCalled = true
								if returns.periodCommitError {
									return "", errors.New("fake period update --commit error")
								}
								return "", nil // success
							}
							periodUpdateCalled = true
							if returns.periodUpdateOutput == "" {
								return "", errors.New("fake period update (no --commit) error")
							}
							return returns.periodUpdateOutput, nil
						}
					}
				}

				t.Fatalf("unhandled command: %s %v", command, args)
				panic("unhandled command")
			},
		}

		return &clusterd.Context{
			Executor: executor,
		}
	}

	expectNoErr := false // want no error
	expectErr := true    // want an error

	tests := []struct {
		name           string
		commandReturns commandReturns
		expectCommands expectCommands
		wantErr        bool
	}{
		// a bit more background: creating a realm creates the first period epoch. When Rook creates
		// zonegroup and zone, it results in many changes to the period.
		{"real-world first reconcile (many changes, should commit period)",
			commandReturns{
				periodGetOutput:    firstPeriodGet,
				periodUpdateOutput: firstPeriodUpdate,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: true,
			},
			expectNoErr,
		},
		// note: this also tests that we support the output changing in the future to increment "epoch"
		{"real-world second reconcile (no changes, should not commit period)",
			commandReturns{
				periodGetOutput:    secondPeriodGet,
				periodUpdateOutput: secondPeriodUpdateWithoutChanges,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: false,
			},
			expectNoErr,
		},
		{"second reconcile with changes",
			commandReturns{
				periodGetOutput:    secondPeriodGet,
				periodUpdateOutput: secondPeriodUpdateWithChanges,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: true,
			},
			expectNoErr,
		},
		{"invalid get json",
			commandReturns{
				periodGetOutput:    `{"ids": [}`, // json obj with incomplete array that won't parse
				periodUpdateOutput: firstPeriodUpdate,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: false,
			},
			expectErr,
		},
		{"invalid update json",
			commandReturns{
				periodGetOutput:    firstPeriodGet,
				periodUpdateOutput: `{"ids": [}`,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: false,
			},
			expectErr,
		},
		{"fail period get",
			commandReturns{
				periodGetOutput:    "", // error
				periodUpdateOutput: firstPeriodUpdate,
			},
			expectCommands{
				periodUpdate: false,
				periodCommit: false,
			},
			expectErr,
		},
		{"fail period update",
			commandReturns{
				periodGetOutput:    firstPeriodGet,
				periodUpdateOutput: "", // error
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: false,
			},
			expectErr,
		},
		{"fail period commit",
			commandReturns{
				periodGetOutput:    firstPeriodGet,
				periodUpdateOutput: firstPeriodUpdate,
				periodCommitError:  true,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: true,
			},
			expectErr,
		},
		{"configs are removed",
			commandReturns{
				periodGetOutput:    secondPeriodUpdateWithChanges,
				periodUpdateOutput: secondPeriodUpdateWithoutChanges,
			},
			expectCommands{
				periodUpdate: true,
				periodCommit: true,
			},
			expectNoErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTest(tt.commandReturns)
			objCtx := NewContext(ctx, &client.ClusterInfo{Namespace: "my-cluster"}, "my-store")

			err := CommitConfigChanges(objCtx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.True(t, periodGetCalled)
			assert.Equal(t, tt.expectCommands.periodUpdate, periodUpdateCalled)
			assert.Equal(t, tt.expectCommands.periodCommit, periodCommitCalled)
		})
	}
}

// example real-world output from 'radosgw-admin period get' after initial realm, zonegroup, and
// zone creation and before 'radosgw-admin period update --commit'
const firstPeriodGet = `{
    "id": "5338e008-26db-4013-92f5-c51505a917e2",
    "epoch": 1,
    "predecessor_uuid": "",
    "sync_status": [],
    "period_map": {
        "id": "5338e008-26db-4013-92f5-c51505a917e2",
        "zonegroups": [],
        "short_zone_ids": []
    },
    "master_zonegroup": "",
    "master_zone": "",
    "period_config": {
        "bucket_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        },
        "user_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        }
    },
    "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
    "realm_name": "my-store",
    "realm_epoch": 1
}`

// example real-world output from 'radosgw-admin period update' after initial realm, zonegroup, and
// zone creation and before 'radosgw-admin period update --commit'
const firstPeriodUpdate = `{
    "id": "94ba560d-a560-431d-8ed4-85a2891f9122:staging",
    "epoch": 1,
    "predecessor_uuid": "5338e008-26db-4013-92f5-c51505a917e2",
    "sync_status": [],
    "period_map": {
        "id": "5338e008-26db-4013-92f5-c51505a917e2",
        "zonegroups": [
            {
                "id": "1580fd1d-a065-4484-82ff-329e9a779999",
                "name": "my-store",
                "api_name": "my-store",
                "is_master": "true",
                "endpoints": [
                    "http://10.105.59.166:80"
                ],
                "hostnames": [],
                "hostnames_s3website": [],
                "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "zones": [
                    {
                        "id": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                        "name": "my-store",
                        "endpoints": [
                            "http://10.105.59.166:80"
                        ],
                        "log_meta": "false",
                        "log_data": "false",
                        "bucket_index_max_shards": 11,
                        "read_only": "false",
                        "tier_type": "",
                        "sync_from_all": "true",
                        "sync_from": [],
                        "redirect_zone": ""
                    }
                ],
                "placement_targets": [
                    {
                        "name": "default-placement",
                        "tags": [],
                        "storage_classes": [
                            "STANDARD"
                        ]
                    }
                ],
                "default_placement": "default-placement",
                "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
                "sync_policy": {
                    "groups": []
                }
            }
        ],
        "short_zone_ids": [
            {
                "key": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "val": 1698422904
            }
        ]
    },
    "master_zonegroup": "1580fd1d-a065-4484-82ff-329e9a779999",
    "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
    "period_config": {
        "bucket_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        },
        "user_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        }
    },
    "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
    "realm_name": "my-store",
    "realm_epoch": 2
}`

// example real-world output from 'radosgw-admin period get' after the first period commit
const secondPeriodGet = `{
    "id": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
    "epoch": 1,
    "predecessor_uuid": "5338e008-26db-4013-92f5-c51505a917e2",
    "sync_status": [],
    "period_map": {
        "id": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
        "zonegroups": [
            {
                "id": "1580fd1d-a065-4484-82ff-329e9a779999",
                "name": "my-store",
                "api_name": "my-store",
                "is_master": "true",
                "endpoints": [
                    "http://10.105.59.166:80"
                ],
                "hostnames": [],
                "hostnames_s3website": [],
                "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "zones": [
                    {
                        "id": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                        "name": "my-store",
                        "endpoints": [
                            "http://10.105.59.166:80"
                        ],
                        "log_meta": "false",
                        "log_data": "false",
                        "bucket_index_max_shards": 11,
                        "read_only": "false",
                        "tier_type": "",
                        "sync_from_all": "true",
                        "sync_from": [],
                        "redirect_zone": ""
                    }
                ],
                "placement_targets": [
                    {
                        "name": "default-placement",
                        "tags": [],
                        "storage_classes": [
                            "STANDARD"
                        ]
                    }
                ],
                "default_placement": "default-placement",
                "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
                "sync_policy": {
                    "groups": []
                }
            }
        ],
        "short_zone_ids": [
            {
                "key": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "val": 1698422904
            }
        ]
    },
    "master_zonegroup": "1580fd1d-a065-4484-82ff-329e9a779999",
    "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
    "period_config": {
        "bucket_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        },
        "user_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        }
    },
    "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
    "realm_name": "my-store",
    "realm_epoch": 2
}`

// example real-world output from 'radosgw-admin period update' after the first period commit,
// and with no changes since the first commit
// note: output was modified to increment the epoch to make sure this code works in case the "epoch"
//       behavior changes in radosgw-admin in the future
const secondPeriodUpdateWithoutChanges = `{
    "id": "94ba560d-a560-431d-8ed4-85a2891f9122:staging",
    "epoch": 2,
    "predecessor_uuid": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
    "sync_status": [],
    "period_map": {
        "id": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
        "zonegroups": [
            {
                "id": "1580fd1d-a065-4484-82ff-329e9a779999",
                "name": "my-store",
                "api_name": "my-store",
                "is_master": "true",
                "endpoints": [
                    "http://10.105.59.166:80"
                ],
                "hostnames": [],
                "hostnames_s3website": [],
                "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "zones": [
                    {
                        "id": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                        "name": "my-store",
                        "endpoints": [
                            "http://10.105.59.166:80"
                        ],
                        "log_meta": "false",
                        "log_data": "false",
                        "bucket_index_max_shards": 11,
                        "read_only": "false",
                        "tier_type": "",
                        "sync_from_all": "true",
                        "sync_from": [],
                        "redirect_zone": ""
                    }
                ],
                "placement_targets": [
                    {
                        "name": "default-placement",
                        "tags": [],
                        "storage_classes": [
                            "STANDARD"
                        ]
                    }
                ],
                "default_placement": "default-placement",
                "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
                "sync_policy": {
                    "groups": []
                }
            }
        ],
        "short_zone_ids": [
            {
                "key": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "val": 1698422904
            }
        ]
    },
    "master_zonegroup": "1580fd1d-a065-4484-82ff-329e9a779999",
    "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
    "period_config": {
        "bucket_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        },
        "user_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        }
    },
    "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
    "realm_name": "my-store",
    "realm_epoch": 3
}`

// example output from 'radosgw-admin period update' after the first period commit,
// and with un-committed changes since the first commit (endpoint added to zonegroup and zone)
const secondPeriodUpdateWithChanges = `{
    "id": "94ba560d-a560-431d-8ed4-85a2891f9122:staging",
    "epoch": 1,
    "predecessor_uuid": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
    "sync_status": [],
    "period_map": {
        "id": "600c23a6-2452-4fc0-96b4-0c78b9b7c439",
        "zonegroups": [
            {
                "id": "1580fd1d-a065-4484-82ff-329e9a779999",
                "name": "my-store",
                "api_name": "my-store",
                "is_master": "true",
                "endpoints": [
                    "http://10.105.59.166:80",
                    "https://10.105.59.166:443"
                ],
                "hostnames": [],
                "hostnames_s3website": [],
                "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "zones": [
                    {
                        "id": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                        "name": "my-store",
                        "endpoints": [
                            "http://10.105.59.166:80",
                            "https://10.105.59.166:443"
                        ],
                        "log_meta": "false",
                        "log_data": "false",
                        "bucket_index_max_shards": 11,
                        "read_only": "false",
                        "tier_type": "",
                        "sync_from_all": "true",
                        "sync_from": [],
                        "redirect_zone": ""
                    }
                ],
                "placement_targets": [
                    {
                        "name": "default-placement",
                        "tags": [],
                        "storage_classes": [
                            "STANDARD"
                        ]
                    }
                ],
                "default_placement": "default-placement",
                "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
                "sync_policy": {
                    "groups": []
                }
            }
        ],
        "short_zone_ids": [
            {
                "key": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
                "val": 1698422904
            }
        ]
    },
    "master_zonegroup": "1580fd1d-a065-4484-82ff-329e9a779999",
    "master_zone": "cea71d3a-9d22-45fb-a4e8-04fc6a494a50",
    "period_config": {
        "bucket_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        },
        "user_quota": {
            "enabled": false,
            "check_on_raw": false,
            "max_size": -1,
            "max_size_kb": 0,
            "max_objects": -1
        }
    },
    "realm_id": "94ba560d-a560-431d-8ed4-85a2891f9122",
    "realm_name": "my-store",
    "realm_epoch": 3
}`
