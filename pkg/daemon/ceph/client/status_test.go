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
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	activeClean              = "active+clean"
	activeCleanScrubbing     = "active+clean+scrubbing"
	activeCleanScrubbingDeep = "active+clean+scrubbing+deep"
	activeCleanSnaptrim      = "active+clean+snaptrim"
	activeCleanSnaptrimWait  = "active+clean+snaptrim_wait"

	// this JSON was generated from `ceph status -f json`, using Ceph Luminous 12.1.3
	CephStatusResponseRaw = `{"fsid":"613975f3-3025-4802-9de1-a2280b950e75","health":{"checks":{"OSD_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 osds down"}},"OSD_HOST_DOWN":{"severity":"HEALTH_WARN","summary":{"message":"1 host (1 osds) down"}},"PG_AVAILABILITY":{"severity":"HEALTH_WARN","summary":{"message":"Reduced data availability: 101 pgs stale"}},"POOL_APP_NOT_ENABLED":{"severity":"HEALTH_WARN","summary":{"message":"application not enabled on 1 pool(s)"}}},"status":"HEALTH_WARN","overall_status":"HEALTH_WARN"},"election_epoch":12,"quorum":[0,1,2],"quorum_names":["rook-ceph-mon0","rook-ceph-mon2","rook-ceph-mon1"],"monmap":{"epoch":3,"fsid":"613975f3-3025-4802-9de1-a2280b950e75","modified":"2017-08-11 20:13:02.075679","created":"2017-08-11 20:12:35.314510","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"10.3.0.45:6789/0","public_addr":"10.3.0.45:6789/0"},{"rank":1,"name":"rook-ceph-mon2","addr":"10.3.0.249:6789/0","public_addr":"10.3.0.249:6789/0"},{"rank":2,"name":"rook-ceph-mon1","addr":"10.3.0.252:6789/0","public_addr":"10.3.0.252:6789/0"}]},"osdmap":{"epoch":17,"num_osds":2,"num_up_osds":1,"num_in_osds":2,"full":false,"nearfull":true,"num_remapped_pgs":0},"pgmap":{"pgs_by_state":[{"state_name":"stale+active+clean","count":101},{"state_name":"active+clean","count":99}],"num_pgs":200,"num_pools":2,"num_objects":243,"data_bytes":976793635,"bytes_used":13611479040,"bytes_avail":19825307648,"bytes_total":33436786688},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":14111,"active_name":"rook-ceph-mgr0","active_addr":"10.2.73.6:6800/9","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","prometheus","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
)

// this JSON was generated from `ceph status -f json`, using Ceph Nautilus 14.2.1
// It was chopped to only show what the tests are looking for
var statusFakeRaw = []byte(`{
		"fsmap": {
		  "epoch": 13,
		  "id": 1,
		  "up": 1,
		  "in": 1,
		  "max": 1,
		  "by_rank": [
			{
			  "filesystem_id": 1,
			  "rank": 0,
			  "name": "myfs-b",
			  "status": "up:active",
			  "gid": 13999249051405999408
			}
		  ],
		  "up:standby": 1
		}
	  }`)

func TestStatusMarshal(t *testing.T) {
	var status CephStatus
	err := json.Unmarshal([]byte(CephStatusResponseRaw), &status)
	assert.Nil(t, err)

	// verify some health fields
	assert.Equal(t, "HEALTH_WARN", status.Health.Status)
	assert.Equal(t, 4, len(status.Health.Checks))
	assert.Equal(t, "HEALTH_WARN", status.Health.Checks["OSD_DOWN"].Severity)
	assert.Equal(t, "1 osds down", status.Health.Checks["OSD_DOWN"].Summary.Message)
	assert.Equal(t, "HEALTH_WARN", status.Health.Checks["OSD_HOST_DOWN"].Severity)
	assert.Equal(t, "1 host (1 osds) down", status.Health.Checks["OSD_HOST_DOWN"].Summary.Message)

	// verify some Mon map fields
	assert.Equal(t, 3, status.MonMap.Epoch)
	assert.Equal(t, "rook-ceph-mon0", status.MonMap.Mons[0].Name)
	assert.Equal(t, "10.3.0.45:6789/0", status.MonMap.Mons[0].Address)

	// verify some OSD map fields
	assert.Equal(t, 2, status.OsdMap.NumOsd)
	assert.Equal(t, 1, status.OsdMap.NumUpOsd)
	assert.False(t, status.OsdMap.Full)
	assert.True(t, status.OsdMap.NearFull)

	// verify some PG map fields
	assert.Equal(t, 200, status.PgMap.NumPgs)
	assert.Equal(t, uint64(13611479040), status.PgMap.UsedBytes)
	assert.Equal(t, 101, status.PgMap.PgsByState[0].Count)
	assert.Equal(t, "stale+active+clean", status.PgMap.PgsByState[0].StateName)
}

func TestIsClusterClean(t *testing.T) {
	status := CephStatus{
		PgMap: PgMap{
			PgsByState: []PgStateEntry{
				{StateName: activeClean, Count: 3},
			},
			NumPgs: 14,
		},
	}

	// not a clean cluster with PGs not adding up
	_, clean := isClusterClean(status, defaultPgHealthyRegexCompiled)
	assert.False(t, clean)

	// clean cluster
	status.PgMap.PgsByState = append(status.PgMap.PgsByState,
		PgStateEntry{StateName: activeCleanScrubbing, Count: 5})
	status.PgMap.PgsByState = append(status.PgMap.PgsByState,
		PgStateEntry{StateName: activeCleanScrubbingDeep, Count: 2})
	status.PgMap.PgsByState = append(status.PgMap.PgsByState,
		PgStateEntry{StateName: activeCleanSnaptrim, Count: 2})
	status.PgMap.PgsByState = append(status.PgMap.PgsByState,
		PgStateEntry{StateName: activeCleanSnaptrimWait, Count: 2})
	_, clean = isClusterClean(status, defaultPgHealthyRegexCompiled)
	assert.True(t, clean)

	// not a clean cluster with PGs in a bad state
	status.PgMap.PgsByState[0].StateName = "notclean"
	_, clean = isClusterClean(status, defaultPgHealthyRegexCompiled)
	assert.False(t, clean)

	// clean cluster if the regex is satisfied
	re := regexp.MustCompile("notclean")
	_, clean = isClusterClean(status, re)
	assert.False(t, clean)
}

func TestGetMDSRank(t *testing.T) {
	var statusFake CephStatus
	err := json.Unmarshal(statusFakeRaw, &statusFake)
	assert.NoError(t, err)

	mdsRankFake, err := getMDSRank(statusFake, "myfs-b")
	assert.Nil(t, err)
	assert.Equal(t, 0, mdsRankFake)
}

func TestIsCephHealthy(t *testing.T) {
	var statusFake CephStatus
	err := json.Unmarshal(statusFakeRaw, &statusFake)
	assert.NoError(t, err)

	statusFake.Health.Status = "HEALTH_WARN"
	s := isCephHealthy(statusFake)
	assert.True(t, s)

	statusFake.Health.Status = "HEALTH_OK"
	s = isCephHealthy(statusFake)
	assert.True(t, s)

	statusFake.Health.Status = "HEALTH_ERR"
	s = isCephHealthy(statusFake)
	assert.False(t, s)
}
