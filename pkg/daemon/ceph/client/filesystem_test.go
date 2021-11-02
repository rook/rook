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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	exectest "github.com/rook/rook/pkg/util/exec/test"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "fs ls",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "fs ls"})
	cephFilesystemListResponseRaw = `[{"name":"myfs1","metadata_pool":"myfs1-metadata","metadata_pool_id":2,"data_pool_ids":[1],"data_pools":["myfs1-data"]}]`

	// this JSON was generated from the mon_command "fs get",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "fs get","fs_name": fsName,})
	cephFilesystemGetResponseRaw = `{"mdsmap":{"epoch":6,"flags":1,"ever_allowed_features":0,"explicitly_allowed_features":0,"created":"2016-11-30 08:35:06.416438","modified":"2016-11-30 08:35:06.416438","tableserver":0,"root":0,"session_timeout":60,"session_autoclose":300,"max_file_size":1099511627776,"last_failure":0,"last_failure_osd_epoch":0,"compat":{"compat":{},"ro_compat":{},"incompat":{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs","feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap","feature_8":"file layout v2"}},"max_mds":1,"in":[0],"up":{"mds_0":4107},"failed":[],"damaged":[],"stopped":[],"info":{"gid_4107":{"gid":4107,"name":"1","rank":0,"incarnation":4,"state":"up:active","state_seq":3,"addr":"127.0.0.1:6804\/2981621686","standby_for_rank":-1,"standby_for_fscid":-1,"standby_for_name":"","standby_replay":false,"export_targets":[],"features":1152921504336314367}},"data_pools":[1],"metadata_pool":2,"enabled":true,"fs_name":"myfs1","balancer":""},"id":1}`

	// this JSON was generated from the command "fs perf stats"
	cephFilesystemPerfStatsRaw = `{"version": 1, "global_counters": ["cap_hit", "read_latency", "write_latency", "metadata_latency", "dentry_lease"], "counters": [], "client_metadata": {"client.614146": {"IP": "10.1.1.100", "hostname"  : "ceph-host1", "root": "/", "mount_point": "/mnt/cephfs", "valid_metrics": ["cap_hit", "read_latency", "write_latency", "metadata_latency", "dentry_lease"]}}, "global_metrics": {"client.614146": [[0,  0], [0, 0], [0, 0], [0, 0], [0, 0]]}, "metrics": {"delayed_ranks": [], "mds.0": {"client.614146": []}}}`
)

func TestFilesystemListMarshal(t *testing.T) {
	var filesystems []CephFilesystem
	err := json.Unmarshal([]byte(cephFilesystemListResponseRaw), &filesystems)
	assert.Nil(t, err)

	// create the expected file systems listing object
	expectedFilesystems := []CephFilesystem{
		{
			Name:           "myfs1",
			MetadataPool:   "myfs1-metadata",
			MetadataPoolID: 2,
			DataPools:      []string{"myfs1-data"},
			DataPoolIDs:    []int{1}},
	}

	assert.Equal(t, expectedFilesystems, filesystems)
}

func TestFilesystemGetMarshal(t *testing.T) {
	var fs CephFilesystemDetails
	err := json.Unmarshal([]byte(cephFilesystemGetResponseRaw), &fs)
	assert.Nil(t, err)

	// create the expected file system details object
	expectedFS := CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			Enabled:        true,
			Root:           0,
			TableServer:    0,
			MaxMDS:         1,
			MetadataPool:   2,
			DataPools:      []int{1},
			In:             []int{0},
			Up:             map[string]int{"mds_0": 4107},
			Failed:         []int{},
			Damaged:        []int{},
			Stopped:        []int{},
			Info: map[string]MDSInfo{
				"gid_4107": {
					GID:     4107,
					Name:    "1",
					Rank:    0,
					State:   "up:active",
					Address: "127.0.0.1:6804/2981621686",
				},
			},
		},
	}

	assert.Equal(t, expectedFS, fs)
}

func TestFilesystemRemove(t *testing.T) {
	dataDeleted := false
	metadataDeleted := false
	crushDeleted := false
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	fs := CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			DataPools:      []int{1},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "osd" {
			if args[1] == "lspools" {
				pools := []*CephStoragePoolSummary{
					{Name: "mydata", Number: 1},
					{Name: "mymetadata", Number: 2},
				}
				output, err := json.Marshal(pools)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "pool" {
				if args[2] == "get" {
					return `{"pool_id":1}`, nil
				}
				if args[2] == "delete" {
					if args[3] == "mydata" {
						dataDeleted = true
						return "", nil
					}
					if args[3] == "mymetadata" {
						metadataDeleted = true
						return "", nil
					}
				}
			}
			if args[1] == "crush" {
				assert.Equal(t, "rule", args[2])
				assert.Equal(t, "rm", args[3])
				crushDeleted = true
				return "", nil
			}
		}
		emptyPool := "{\"images\":{\"count\":0,\"provisioned_bytes\":0,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
		if args[0] == "pool" {
			if args[1] == "stats" {
				return emptyPool, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := RemoveFilesystem(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName, false)
	assert.Nil(t, err)
	assert.True(t, metadataDeleted)
	assert.True(t, dataDeleted)
	assert.True(t, crushDeleted)
}

func TestFailAllStandbyReplayMDS(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	failedGids := make([]string, 0)
	fs := CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				failedGids = append(failedGids, args[2])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := FailAllStandbyReplayMDS(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName)
	assert.NoError(t, err)
	assert.ElementsMatch(t, failedGids, []string{"124"})

	fs = CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", errors.Errorf("unexpected execution of mds fail")
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	err = FailAllStandbyReplayMDS(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName)
	assert.NoError(t, err)

	fs = CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", errors.Errorf("expected execution of mds fail")
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	err = FailAllStandbyReplayMDS(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected execution of mds fail")
}

func TestGetMdsIdByRank(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	fs := CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	name, err := GetMdsIdByRank(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName, 0)
	assert.Equal(t, name, "myfs1-a")
	assert.NoError(t, err)

	// test errors
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				return "", errors.Errorf("test ceph fs get error")
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	name, err = GetMdsIdByRank(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName, 0)
	assert.Equal(t, "", name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test ceph fs get error")

	fs = CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_1": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_123": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	// test get mds by id failed error
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	name, err = GetMdsIdByRank(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName, 0)
	assert.Equal(t, "", name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get mds gid from rank 0")

	fs = CephFilesystemDetails{
		ID: 1,
		MDSMap: MDSMap{
			FilesystemName: "myfs1",
			MetadataPool:   2,
			Up: map[string]int{
				"mds_0": 123,
			},
			DataPools: []int{3},
			Info: map[string]MDSInfo{
				"gid_122": {
					GID:   123,
					State: "up:active",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "a"),
				},
				"gid_124": {
					GID:   124,
					State: "up:standby-replay",
					Name:  fmt.Sprintf("%s-%s", "myfs1", "b"),
				},
			},
		},
	}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "get" {
				output, err := json.Marshal(fs)
				assert.Nil(t, err)
				return string(output), nil
			}
			if args[1] == "rm" {
				return "", nil
			}
		}
		if args[0] == "mds" {
			if args[1] == "fail" {
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	name, err = GetMdsIdByRank(context, AdminClusterInfo("mycluster"), fs.MDSMap.FilesystemName, 0)
	assert.Equal(t, "", name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get mds info for rank 0")
}

func TestGetMDSDump(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "dump" {
				output := `{"epoch":12,"default_fscid":1,"compat":{"compat":{},"ro_compat":{},"incompat":
				{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs",
				"feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap",
				"feature_8":"no anchor table","feature_9":"file layout v2","feature_10":"snaprealm v2"}},"feature_flags":
				{"enable_multiple":false,"ever_enabled_multiple":false},"standbys":[{"gid":26829,"name":"rook-ceph-filesystem-b","rank":-1,"incarnation":0,"state":"up:standby",
				"state_seq":1,"addr":"10.110.29.245:6805/3170687682","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.245:6804","nonce":3170687682},{"type":"v1","addr":"10.110.29.245:6805","nonce":3170687682}]},"export_targets":[],"features":4611087854035861503,"flags":0,"epoch":12}],"filesystems":[{"mdsmap":{"epoch":11,"flags":18,"ever_allowed_features":32,"explicitly_allowed_features":32,"created":"2021-04-23 01:52:33.467863",
				"modified":"2021-04-23 08:31:03.019621","tableserver":0,"root":0,"session_timeout":60,"session_autoclose":300,"min_compat_client":"-1 (unspecified)","max_file_size":1099511627776,"last_failure":0,"last_failure_osd_epoch":0,"compat":{"compat":{},"ro_compat":{},"incompat":{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs","feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap","feature_8":"no anchor table","feature_9":"file layout v2",
				"feature_10":"snaprealm v2"}},"max_mds":1,"in":[0],"up":{"mds_0":14707},"failed":[],"damaged":[],"stopped":[],"info":{"gid_14707":{"gid":14707,"name":"rook-ceph-filesystem-a","rank":0,"incarnation":5,"state":"up:active","state_seq":2,"addr":"10.110.29.236:6807/1996297745","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.236:6806","nonce":1996297745},
				{"type":"v1","addr":"10.110.29.236:6807","nonce":1996297745}]},"export_targets":[],"features":4611087854035861503,"flags":0}},"data_pools":[3],"metadata_pool":2,"enabled":true,"fs_name":"rook-ceph-filesystem","balancer":"","standby_count_wanted":1},"id":1}]}`
				return output, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	mdsDump, err := GetMDSDump(context, AdminClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.ElementsMatch(t, mdsDump.Standbys, []MDSStandBy{{Name: "rook-ceph-filesystem-b", Rank: -1}})

	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "dump" {
				return "", errors.Errorf("dump fs failed")
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	_, err = GetMDSDump(context, AdminClusterInfo("mycluster"))
	assert.Error(t, err)
}

func TestWaitForNoStandbys(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "dump" {
				output := `{"epoch":12,"default_fscid":1,"compat":{"compat":{},"ro_compat":{},"incompat":
				{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs",
				"feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap",
				"feature_8":"no anchor table","feature_9":"file layout v2","feature_10":"snaprealm v2"}},"feature_flags":
				{"enable_multiple":false,"ever_enabled_multiple":false},"standbys":[{"gid":26829,"name":"rook-ceph-filesystem-b","rank":-1,"incarnation":0,"state":"up:standby",
				"state_seq":1,"addr":"10.110.29.245:6805/3170687682","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.245:6804","nonce":3170687682},{"type":"v1","addr":"10.110.29.245:6805","nonce":3170687682}]},"export_targets":[],"features":4611087854035861503,"flags":0,"epoch":12}],"filesystems":[{"mdsmap":{"epoch":11,"flags":18,"ever_allowed_features":32,"explicitly_allowed_features":32,"created":"2021-04-23 01:52:33.467863",
				"modified":"2021-04-23 08:31:03.019621","tableserver":0,"root":0,"session_timeout":60,"session_autoclose":300,"min_compat_client":"-1 (unspecified)","max_file_size":1099511627776,"last_failure":0,"last_failure_osd_epoch":0,"compat":{"compat":{},"ro_compat":{},"incompat":{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs","feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap","feature_8":"no anchor table","feature_9":"file layout v2",
				"feature_10":"snaprealm v2"}},"max_mds":1,"in":[0],"up":{"mds_0":14707},"failed":[],"damaged":[],"stopped":[],"info":{"gid_14707":{"gid":14707,"name":"rook-ceph-filesystem-a","rank":0,"incarnation":5,"state":"up:active","state_seq":2,"addr":"10.110.29.236:6807/1996297745","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.236:6806","nonce":1996297745},
				{"type":"v1","addr":"10.110.29.236:6807","nonce":1996297745}]},"export_targets":[],"features":4611087854035861503,"flags":0}},"data_pools":[3],"metadata_pool":2,"enabled":true,"fs_name":"rook-ceph-filesystem","balancer":"","standby_count_wanted":1},"id":1}]}`
				return output, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := WaitForNoStandbys(context, AdminClusterInfo("mycluster"), 6*time.Second)
	assert.Error(t, err)

	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "dump" {
				return "", errors.Errorf("failed to dump fs info")
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err = WaitForNoStandbys(context, AdminClusterInfo("mycluster"), 6*time.Second)
	assert.Error(t, err)

	firstCall := true
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "fs" {
			if args[1] == "dump" {
				if firstCall {
					firstCall = false
					output := `{"epoch":12,"default_fscid":1,"compat":{"compat":{},"ro_compat":{},"incompat":
				{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs",
				"feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap",
				"feature_8":"no anchor table","feature_9":"file layout v2","feature_10":"snaprealm v2"}},"feature_flags":
				{"enable_multiple":false,"ever_enabled_multiple":false},"standbys":[{"gid":26829,"name":"rook-ceph-filesystem-b","rank":-1,"incarnation":0,"state":"up:standby",
				"state_seq":1,"addr":"10.110.29.245:6805/3170687682","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.245:6804","nonce":3170687682},{"type":"v1","addr":"10.110.29.245:6805","nonce":3170687682}]},"export_targets":[],"features":4611087854035861503,"flags":0,"epoch":12}],"filesystems":[{"mdsmap":{"epoch":11,"flags":18,"ever_allowed_features":32,"explicitly_allowed_features":32,"created":"2021-04-23 01:52:33.467863",
				"modified":"2021-04-23 08:31:03.019621","tableserver":0,"root":0,"session_timeout":60,"session_autoclose":300,"min_compat_client":"-1 (unspecified)","max_file_size":1099511627776,"last_failure":0,"last_failure_osd_epoch":0,"compat":{"compat":{},"ro_compat":{},"incompat":{"feature_1":"base v0.20","feature_2":"client writeable ranges","feature_3":"default file layouts on dirs","feature_4":"dir inode in separate object","feature_5":"mds uses versioned encoding","feature_6":"dirfrag is stored in omap","feature_8":"no anchor table","feature_9":"file layout v2",
				"feature_10":"snaprealm v2"}},"max_mds":1,"in":[0],"up":{"mds_0":14707},"failed":[],"damaged":[],"stopped":[],"info":{"gid_14707":{"gid":14707,"name":"rook-ceph-filesystem-a","rank":0,"incarnation":5,"state":"up:active","state_seq":2,"addr":"10.110.29.236:6807/1996297745","addrs":{"addrvec":[{"type":"v2","addr":"10.110.29.236:6806","nonce":1996297745},
				{"type":"v1","addr":"10.110.29.236:6807","nonce":1996297745}]},"export_targets":[],"features":4611087854035861503,"flags":0}},"data_pools":[3],"metadata_pool":2,"enabled":true,"fs_name":"rook-ceph-filesystem","balancer":"","standby_count_wanted":1},"id":1}]}`
					return output, nil
				}

				return `{"standbys":[],"filesystemds":[]}`, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	err = WaitForNoStandbys(context, AdminClusterInfo("mycluster"), 6*time.Second)
	assert.NoError(t, err)

}

func TestGetPerfStatMarshal(t *testing.T) {
	var fsStats cephv1.FilesystemStats
	err := json.Unmarshal([]byte(cephFilesystemPerfStatsRaw), &fsStats)
	assert.NoError(t, err)
	assert.Equal(t, 1, fsStats.Version)
	assert.Equal(t, "10.1.1.100", fsStats.ClientMetadata["client.614146"].IP)
}
