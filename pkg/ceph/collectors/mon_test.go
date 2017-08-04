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

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package collectors

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const monStatusInput = `
{
  "fsid": "1ea05ac4-8c5d-42e1-b5b5-1cf44d4f569e",
  "health": {
    "checks": {},
    "status": "HEALTH_OK"
  },
  "election_epoch": 8,
  "quorum": [
    0,
    1,
    2
  ],
  "quorum_names": [
    "rook-ceph-mon0",
    "rook-ceph-mon1",
    "rook-ceph-mon2"
  ],
  "monmap": {
    "epoch": 3,
    "fsid": "1ea05ac4-8c5d-42e1-b5b5-1cf44d4f569e",
    "modified": "2017-08-01 14:03:07.872957",
    "created": "2017-08-01 14:02:55.733786",
    "features": {
      "persistent": [
        "kraken",
        "luminous"
      ],
      "optional": []
    },
    "mons": [
      {
        "rank": 0,
        "name": "rook-ceph-mon0",
        "addr": "172.17.0.5:6790/0",
        "public_addr": "172.17.0.5:6790/0"
      },
      {
        "rank": 1,
        "name": "rook-ceph-mon1",
        "addr": "172.17.0.6:6790/0",
        "public_addr": "172.17.0.6:6790/0"
      },
      {
        "rank": 2,
        "name": "rook-ceph-mon2",
        "addr": "172.17.0.7:6790/0",
        "public_addr": "172.17.0.7:6790/0"
      }
    ]
  },
  "osdmap": {
    "osdmap": {
      "epoch": 8,
      "num_osds": 1,
      "num_up_osds": 1,
      "num_in_osds": 1,
      "full": false,
      "nearfull": false,
      "num_remapped_pgs": 0
    }
  },
  "pgmap": {
    "pgs_by_state": [],
    "num_pgs": 0,
    "num_pools": 0,
    "num_objects": 0,
    "data_bytes": 0,
    "bytes_used": 3408617472,
    "bytes_avail": 13884915712,
    "bytes_total": 17293533184
  },
  "fsmap": {
    "epoch": 1,
    "by_rank": []
  },
  "mgrmap": {
    "epoch": 3,
    "active_gid": 4114,
    "active_name": "rook-ceph-mgr0",
    "active_addr": "172.17.0.8:6800/11",
    "available": true,
    "standbys": [],
    "modules": [
      "restful",
      "status"
    ],
    "available_modules": [
      "dashboard",
      "restful",
      "status",
      "zabbix"
    ]
  },
  "servicemap": {
    "epoch": 1,
    "modified": "0.000000",
    "services": {}
  }
}
`
const timeStatusInput = `
{
  "time_skew_status": {
     "test-mon01": {
        "skew": 0.000000,
        "latency": 0.000000,
        "health": "HEALTH_OK"
    },
    "test-mon02": {
        "skew": -0.000002,
        "latency": 0.000815,
        "health": "HEALTH_OK"
     },
    "test-mon03": {
        "skew": -0.000002,
        "latency": 0.000829,
        "health": "HEALTH_OK"
    },
    "test-mon04": {
        "skew": -0.000019,
        "latency": 0.000609,
        "health": "HEALTH_OK"
    },
    "test-mon05": {
        "skew": -0.000628,
        "latency": 0.000659,
        "health": "HEALTH_OK"
    }
   },
   "timechecks": {
        "epoch": 8,
        "round": 2,
        "round_status": "finished"
    }
}
`

func TestMonitorCollector(t *testing.T) {
	expressions := []*regexp.Regexp{
		regexp.MustCompile(`ceph_monitor_clock_skew_seconds{monitor="test-mon01"} 0`),
		regexp.MustCompile(`ceph_monitor_clock_skew_seconds{monitor="test-mon02"} -2e-06`),
		regexp.MustCompile(`ceph_monitor_clock_skew_seconds{monitor="test-mon03"} -2e-06`),
		regexp.MustCompile(`ceph_monitor_clock_skew_seconds{monitor="test-mon04"} -1.9e-05`),
		regexp.MustCompile(`ceph_monitor_clock_skew_seconds{monitor="test-mon05"} -0.000628`),
		regexp.MustCompile(`ceph_monitor_latency_seconds{monitor="test-mon01"} 0`),
		regexp.MustCompile(`ceph_monitor_latency_seconds{monitor="test-mon02"} 0.000815`),
		regexp.MustCompile(`ceph_monitor_latency_seconds{monitor="test-mon03"} 0.000829`),
		regexp.MustCompile(`ceph_monitor_latency_seconds{monitor="test-mon04"} 0.000609`),
		regexp.MustCompile(`ceph_monitor_latency_seconds{monitor="test-mon05"} 0.000659`),
		regexp.MustCompile(`ceph_monitor_quorum_count 3`),
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(actionName string, command string, outFileArg string, args ...string) (string, error) {
			if args[0] == "time-sync-status" {
				return timeStatusInput, nil
			} else {
				return monStatusInput, nil
			}
		},
	}
	configDir, _ := ioutil.TempDir("", "")
	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)
	collector := NewMonitorCollector(context, "mycluster")
	if err := prometheus.Register(collector); err != nil {
		t.Fatalf("collector failed to register: %s", err)
	}
	defer prometheus.Unregister(collector)

	server := httptest.NewServer(prometheus.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected failed response from prometheus: %s", err)
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading server response: %s", err)
	}

	for _, re := range expressions {
		assert.True(t, re.Match(buf), fmt.Sprintf("expected: %q", re))
	}
}
