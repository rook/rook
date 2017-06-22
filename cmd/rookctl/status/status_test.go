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
package status

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
)

func TestGetStatus(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetStatusDetails: func() (model.StatusDetails, error) {
			return model.StatusDetails{
				OverallStatus: model.HealthWarning,
				SummaryMessages: []model.StatusSummary{
					{Status: model.HealthWarning, Message: "a cluster warning"},
					{Status: model.HealthOK, Message: "a cluster OK message"},
				},
				Monitors: []model.MonitorSummary{
					{Name: "mon00", Address: "192.155.0.0", InQuorum: true, Status: model.HealthOK},
					{Name: "mon01", Address: "192.155.0.1", InQuorum: false, Status: model.HealthError},
				},
				Mgrs: model.MgrSummary{
					ActiveName: "cephmgr1",
					Standbys:   []string{"cephmgr23", "cephmgr46"},
				},
				OSDs: model.OSDSummary{
					Total: 5, NumberIn: 4, NumberUp: 4, Full: false, NearFull: true,
				},
				PGs: model.PGSummary{
					Total:       100,
					StateCounts: map[string]int{"state1": 50},
				},
				Usage: model.UsageSummary{
					TotalBytes:     1000,
					DataBytes:      100,
					UsedBytes:      200,
					AvailableBytes: 700,
				},
			}, nil
		},
	}

	out, err := getStatus(c)
	assert.Nil(t, err)

	expectedOut := "OVERALL STATUS: WARNING\n\n" +
		"SUMMARY:\n" +
		"SEVERITY   MESSAGE\n" +
		"WARNING    a cluster warning\n" +
		"OK         a cluster OK message\n\n" +
		"USAGE:\n" +
		"TOTAL     USED      DATA      AVAILABLE\n" +
		"1000 B    200 B     100 B     700 B\n\n" +
		"MONITORS:\n" +
		"NAME      ADDRESS       IN QUORUM   STATUS\n" +
		"mon00     192.155.0.0   true        OK\n" +
		"mon01     192.155.0.1   false       ERROR\n\n" +
		"MGRs:\n" +
		"NAME        STATUS\n" +
		"cephmgr1    Active\n" +
		"cephmgr23   Standby\n" +
		"cephmgr46   Standby\n\n" +
		"OSDs:\n" +
		"TOTAL     UP        IN        FULL      NEAR FULL\n" +
		"5         4         4         false     true\n\n" +
		"PLACEMENT GROUPS (100 total):\n" +
		"STATE     COUNT\n" +
		"state1    50\n"
	assert.Equal(t, expectedOut, out)
}

func TestGetStatusEmptyResponse(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetStatusDetails: func() (model.StatusDetails, error) {
			return model.StatusDetails{
				OverallStatus:   model.HealthUnknown,
				SummaryMessages: []model.StatusSummary{},
				Monitors:        []model.MonitorSummary{},
				PGs:             model.PGSummary{StateCounts: map[string]int{}},
			}, nil
		},
	}

	out, err := getStatus(c)
	assert.Nil(t, err)

	expectedOut := "OVERALL STATUS: UNKNOWN\n" +
		"\nSUMMARY:\n" +
		"SEVERITY   MESSAGE\n\n" +
		"USAGE:\n" +
		"TOTAL     USED      DATA      AVAILABLE\n" +
		"0 B       0 B       0 B       0 B\n\n" +
		"MONITORS:\n" +
		"NAME      ADDRESS   IN QUORUM   STATUS\n\n" +
		"MGRs:\n" +
		"NAME      STATUS\n\n" +
		"OSDs:\n" +
		"TOTAL     UP        IN        FULL      NEAR FULL\n" +
		"0         0         0         false     false\n\n" +
		"PLACEMENT GROUPS (0 total):\n" +
		"STATE     COUNT\n"
	assert.Equal(t, expectedOut, out)
}
