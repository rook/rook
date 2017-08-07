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
package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the command: ceph status
	cephStatusResponseRaw = `{"fsid":"d64ecb40-3e93-44e8-9957-2faba55c56de","health":{"checks":{"TEST_MSG":{"severity":"HEALTH_WARN","message":"too many PGs per OSD (2048 > max 300)"}},"status":"HEALTH_WARN"},"election_epoch":6,"quorum":[0],"quorum_names":["rook-ceph-mon0"],"monmap":{"epoch":3,"fsid":"d64ecb40-3e93-44e8-9957-2faba55c56de","modified":"2017-08-01 16:50:42.901253","created":"2017-08-01 16:50:30.751733","features":{"persistent":["kraken","luminous"],"optional":[]},"mons":[{"rank":0,"name":"rook-ceph-mon0","addr":"127.0.0.1:6790/0","public_addr":"127.0.0.1:6790/0"}]},"osdmap":{"osdmap":{"epoch":8,"num_osds":10,"num_up_osds":9,"num_in_osds":9,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"created+peering","count": 100},{"state_name": "active+clean","count": 2048}],"num_pgs":2148,"num_pools":0,"num_objects":0,"data_bytes":0,"bytes_used":123,"bytes_avail":234,"bytes_total":345},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"epoch":3,"active_gid":4109,"active_name":"rook-ceph-mgr0","active_addr":"172.17.0.8:6800/13","available":true,"standbys":[],"modules":["restful","status"],"available_modules":["dashboard","restful","status","zabbix"]},"servicemap":{"epoch":1,"modified":"0.000000","services":{}}}`
	// this JSON was generated from the command: ceph time-sync-status
	cephTimeStatusReponseRaw = `{"time_skew_status":{"rook-ceph-mon0":{"skew":0.000000,"latency":0.000000,"health":"HEALTH_OK"}},"timechecks":{"epoch":6,"round":6,"round_status":"finished"}}`
)

func TestGetStatusDetailsHandler(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/status", nil)
	if err != nil {
		logger.Fatal(err)
	}

	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case args[0] == "status":
			return cephStatusResponseRaw, nil
		case args[0] == "time-sync-status":
			return cephTimeStatusReponseRaw, nil
		}
		return "", fmt.Errorf("unexpected mon_command '%v'", args)
	}

	// make a request to GetStatusDetails and verify the results
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetStatusDetails(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.StatusDetails{
		OverallStatus: model.HealthWarning,
		SummaryMessages: []model.StatusSummary{
			{Status: model.HealthWarning, Name: "TEST_MSG", Message: "too many PGs per OSD (2048 > max 300)"},
		},
		Monitors: []model.MonitorSummary{
			{Name: "rook-ceph-mon0", Address: "127.0.0.1:6790/0", InQuorum: true, Status: model.HealthOK},
		},
		OSDs: model.OSDSummary{
			Total: 10, NumberIn: 9, NumberUp: 9, Full: false, NearFull: true,
		},
		Mgrs: model.MgrSummary{
			ActiveName: "rook-ceph-mgr0",
			ActiveAddr: "172.17.0.8:6800/13",
			Available:  true,
		},
		PGs: model.PGSummary{
			Total:       2148,
			StateCounts: map[string]int{"active+clean": 2048, "created+peering": 100},
		},
		Usage: model.UsageSummary{
			TotalBytes:     345,
			DataBytes:      0,
			UsedBytes:      123,
			AvailableBytes: 234,
		},
	}

	VerifyStatusResponse(t, expectedRespObj, w)
}

func TestGetStatusDetailsEmptyResponseFromCeph(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/status", nil)
	if err != nil {
		logger.Fatal(err)
	}

	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case args[0] == "status":
			return "{}", nil

		case args[0] == "time-sync-status":
			return "{}", nil
		}
		return "", fmt.Errorf("unexpected mon_command '%v'", args)
	}

	// make a request to GetStatusDetails and verify the results
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetStatusDetails(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.StatusDetails{
		OverallStatus:   model.HealthUnknown,
		SummaryMessages: []model.StatusSummary{},
		Monitors:        []model.MonitorSummary{},
		PGs:             model.PGSummary{StateCounts: map[string]int{}},
	}
	VerifyStatusResponse(t, expectedRespObj, w)
}

func VerifyStatusResponse(t *testing.T, expectedRespObj model.StatusDetails, w *httptest.ResponseRecorder) {
	// unmarshal the http response to get the actual object and compare it to the expected object
	var actualResultObj model.StatusDetails
	bodyBytes, err := ioutil.ReadAll(w.Body)
	assert.Nil(t, err)
	err = json.Unmarshal(bodyBytes, &actualResultObj)
	assert.Nil(t, err)
	assert.Equal(t, expectedRespObj, actualResultObj)
}
