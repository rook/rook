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
	CephStatusResponseRaw = `{"health":{"health":{"health_services":[{"mons":[{"name":"mon0","kb_total":64891708,"kb_used":34813204,"kb_avail":26759160,"avail_percent":41,"last_updated":"2016-10-26 17:03:36.573444","store_stats":{"bytes_total":14871920,"bytes_sst":0,"bytes_log":2833842,"bytes_misc":12038078,"last_updated":"0.000000"},"health":"HEALTH_OK"}]}]},"timechecks":{"epoch":3,"round":0,"round_status":"finished"},"summary":[{"severity":"HEALTH_WARN","summary":"too many PGs per OSD (2048 > max 300)"}],"overall_status":"HEALTH_WARN","detail":[]},"fsid":"515d542a-fa63-496c-991d-cc8c1e156a3a","election_epoch":3,"quorum":[0],"quorum_names":["mon0"],"monmap":{"epoch":1,"fsid":"515d542a-fa63-496c-991d-cc8c1e156a3a","modified":"2016-10-26 16:10:36.449756","created":"2016-10-26 16:10:36.449756","mons":[{"rank":0,"name":"mon0","addr":"127.0.0.1:6790\/0"}]},"osdmap":{"osdmap":{"epoch":6,"num_osds":10,"num_up_osds":9,"num_in_osds":9,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":2048}],"version":600,"num_pgs":2048,"data_bytes":0,"bytes_used":39048007680,"bytes_avail":27401101312,"bytes_total":66449108992},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"active_gid":0,"active_name":"","standbys":[]}}`
)

func TestStatusMarshal(t *testing.T) {
	var status CephStatus
	err := json.Unmarshal([]byte(CephStatusResponseRaw), &status)
	assert.Nil(t, err)

	// verify some health fields
	assert.Equal(t, "HEALTH_WARN", status.Health.OverallStatus)
	assert.Equal(t, 1, len(status.Health.Summary))
	assert.Equal(t, "HEALTH_WARN", status.Health.Summary[0].Severity)
	assert.Equal(t, "too many PGs per OSD (2048 > max 300)", status.Health.Summary[0].Summary)
	assert.Equal(t, "mon0", status.Health.Details.Services[0]["mons"][0].Name)
	assert.Equal(t, 41, status.Health.Details.Services[0]["mons"][0].AvailablePercent)
	assert.Equal(t, "HEALTH_OK", status.Health.Details.Services[0]["mons"][0].Health)
	assert.Equal(t, uint64(2833842), status.Health.Details.Services[0]["mons"][0].StoreStats.BytesLog)

	// verify some Mon map fields
	assert.Equal(t, 1, status.MonMap.Epoch)
	assert.Equal(t, "mon0", status.MonMap.Mons[0].Name)
	assert.Equal(t, "127.0.0.1:6790/0", status.MonMap.Mons[0].Address)

	// verify some OSD map fields
	assert.Equal(t, 10, status.OsdMap.OsdMap.NumOsd)
	assert.Equal(t, 9, status.OsdMap.OsdMap.NumUpOsd)
	assert.False(t, status.OsdMap.OsdMap.Full)
	assert.True(t, status.OsdMap.OsdMap.NearFull)

	// verify some PG map fields
	assert.Equal(t, 2048, status.PgMap.NumPgs)
	assert.Equal(t, uint64(39048007680), status.PgMap.UsedBytes)
	assert.Equal(t, 2048, status.PgMap.PgsByState[0].Count)
	assert.Equal(t, "active+clean", status.PgMap.PgsByState[0].StateName)
}
