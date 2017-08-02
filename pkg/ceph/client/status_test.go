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
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "status",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "status"})
	CephStatusResponseRaw = `{"fsid":"3ca36e66-bd6a-49d4-a976-cdac216b78c3","health":{"checks":{"OSD_DOWN":{"severity":"HEALTH_WARN","message":"1 osds down"},"OSD_HOST_DOWN":{"severity":"HEALTH_WARN","message":"1 host (1 osds) down"},"OSD_ROOT_DOWN":{"severity":"HEALTH_WARN","message":"1 root (1 osds) down"},"PG_AVAILABILITY":{"severity":"HEALTH_WARN","message":"Reduced data availability: 100 pgs stale"}},"status":"HEALTH_WARN"},"election_epoch":22,"quorum":[0,1,2],"quorum_names":["rook-ceph-mon0","rook-ceph-mon1","rook-ceph-mon2"],"monmap":{"epoch":1,"fsid":"3ca36e66-bd6a-49d4-a976-cdac216b78c3","modified":"2017-07-28 02:39:57.490676","created":"2017-07-28 02:39:57.490676","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"10.0.0.13:6790/0","public_addr":"10.0.0.13:6790/0"},{"rank":1,"name":"rook-ceph-mon1","addr":"10.0.0.102:6790/0","public_addr":"10.0.0.102:6790/0"},{"rank":2,"name":"rook-ceph-mon2","addr":"10.0.0.227:6790/0","public_addr":"10.0.0.227:6790/0"}]},"osdmap":{"osdmap":{"epoch":13,"num_osds":1,"num_up_osds":0,"num_in_osds":1,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"stale+active+clean","count":100}],"num_pgs":100,"num_pools":1,"num_objects":0,"data_bytes":0,"bytes_used":3464249344,"bytes_avail":13829283840,"bytes_total":17293533184},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":4112,"active_name":"rook-ceph-mgr0","active_addr":"172.17.0.10:6800/13","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
)

func TestStatusMarshal(t *testing.T) {
	var status CephStatus
	err := json.Unmarshal([]byte(CephStatusResponseRaw), &status)
	assert.Nil(t, err)

	// verify some health fields
	assert.Equal(t, "HEALTH_WARN", status.Health.Status)
	assert.Equal(t, 4, len(status.Health.Checks))
	assert.Equal(t, "HEALTH_WARN", status.Health.Checks["OSD_DOWN"].Severity)
	assert.Equal(t, "1 osds down", status.Health.Checks["OSD_DOWN"].Message)
	assert.Equal(t, "HEALTH_WARN", status.Health.Checks["OSD_ROOT_DOWN"].Severity)
	assert.Equal(t, "1 root (1 osds) down", status.Health.Checks["OSD_ROOT_DOWN"].Message)

	// verify some Mon map fields
	assert.Equal(t, 1, status.MonMap.Epoch)
	assert.Equal(t, "rook-ceph-mon0", status.MonMap.Mons[0].Name)
	assert.Equal(t, "10.0.0.13:6790/0", status.MonMap.Mons[0].Address)

	// verify some OSD map fields
	assert.Equal(t, 1, status.OsdMap.OsdMap.NumOsd)
	assert.Equal(t, 0, status.OsdMap.OsdMap.NumUpOsd)
	assert.False(t, status.OsdMap.OsdMap.Full)
	assert.True(t, status.OsdMap.OsdMap.NearFull)

	// verify some PG map fields
	assert.Equal(t, 100, status.PgMap.NumPgs)
	assert.Equal(t, uint64(3464249344), status.PgMap.UsedBytes)
	assert.Equal(t, 100, status.PgMap.PgsByState[0].Count)
	assert.Equal(t, "stale+active+clean", status.PgMap.PgsByState[0].StateName)
}
