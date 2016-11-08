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
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ceph "github.com/rook/rook/pkg/cephmgr/client"
	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "status",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "status"})
	CephStatusResponseRaw = `{"health":{"health":{"health_services":[{"mons":[{"name":"mon0","kb_total":64891708,"kb_used":34813204,"kb_avail":26759160,"avail_percent":41,"last_updated":"2016-10-26 17:03:36.573444","store_stats":{"bytes_total":14871920,"bytes_sst":0,"bytes_log":2833842,"bytes_misc":12038078,"last_updated":"0.000000"},"health":"HEALTH_OK"}]}]},"timechecks":{"epoch":3,"round":0,"round_status":"finished"},"summary":[{"severity":"HEALTH_WARN","summary":"too many PGs per OSD (2048 > max 300)"}],"overall_status":"HEALTH_WARN","detail":[]},"fsid":"515d542a-fa63-496c-991d-cc8c1e156a3a","election_epoch":3,"quorum":[0],"quorum_names":["mon0"],"monmap":{"epoch":1,"fsid":"515d542a-fa63-496c-991d-cc8c1e156a3a","modified":"2016-10-26 16:10:36.449756","created":"2016-10-26 16:10:36.449756","mons":[{"rank":0,"name":"mon0","addr":"127.0.0.1:6790\/0"}]},"osdmap":{"osdmap":{"epoch":6,"num_osds":10,"num_up_osds":9,"num_in_osds":9,"full":false,"nearfull":true,"num_remapped_pgs":0}},"pgmap":{"pgs_by_state":[{"state_name":"active+clean","count":2048},{"state_name":"created+peering","count":100}],"version":600,"num_pgs":2148,"data_bytes":0,"bytes_used":39048007680,"bytes_avail":27401101312,"bytes_total":66449108992},"fsmap":{"epoch":1,"by_rank":[]},"mgrmap":{"active_gid":0,"active_name":"","standbys":[]}}`
)

func TestGetStatusDetailsHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/status", nil)
	if err != nil {
		log.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "status") != -1:
				return []byte(CephStatusResponseRaw), "info", nil
			}
			return nil, "", fmt.Errorf("unexpected mon_command '%s'", string(args))
		},
	}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// make a request to GetStatusDetails and verify the results
	w := httptest.NewRecorder()
	h := NewHandler(context, connFactory, cephFactory)
	h.GetStatusDetails(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.StatusDetails{
		OverallStatus: model.HealthWarning,
		SummaryMessages: []model.StatusSummary{
			{Status: model.HealthWarning, Message: "too many PGs per OSD (2048 > max 300)"},
		},
		Monitors: []model.MonitorSummary{
			{Name: "mon0", Address: "127.0.0.1:6790/0", InQuorum: true, Status: model.HealthOK},
		},
		OSDs: model.OSDSummary{
			Total: 10, NumberIn: 9, NumberUp: 9, Full: false, NearFull: true,
		},
		PGs: model.PGSummary{
			Total:       2148,
			StateCounts: map[string]int{"active+clean": 2048, "created+peering": 100},
		},
		Usage: model.UsageSummary{
			TotalBytes:     66449108992,
			DataBytes:      0,
			UsedBytes:      39048007680,
			AvailableBytes: 27401101312,
		},
	}

	VerifyStatusResponse(t, expectedRespObj, w)
}

func TestGetStatusDetailsEmptyResponseFromCeph(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/status", nil)
	if err != nil {
		log.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "status") != -1:
				return []byte("{}"), "info", nil
			}
			return nil, "", fmt.Errorf("unexpected mon_command '%s'", string(args))
		},
	}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// make a request to GetStatusDetails and verify the results
	w := httptest.NewRecorder()
	h := NewHandler(context, connFactory, cephFactory)
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
