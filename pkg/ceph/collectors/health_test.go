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

func TestClusterHealthCollector(t *testing.T) {
	for _, tt := range []struct {
		input   string
		regexes []*regexp.Regexp
	}{
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "5 pgs degraded"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`degraded_pgs 5`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "15 pgs stuck degraded"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_degraded_pgs 15`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "6 pgs unclean"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`unclean_pgs 6`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "16 pgs stuck unclean"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_unclean_pgs 16`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "7 pgs undersized"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`undersized_pgs 7`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "17 pgs stuck undersized"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_undersized_pgs 17`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "8 pgs stale"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stale_pgs 8`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "18 pgs stuck stale"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_stale_pgs 18`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "recovery 10/20 objects degraded"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`degraded_objects 10`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "recovery 20/40 objects misplaced"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`misplaced_objects 20`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 20,
			"num_up_osds": 10,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`osds_down 10`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 1200,
			"num_up_osds": 1200,
			"num_in_osds": 1190,
			"num_remapped_pgs": 10
		}
	},
	"health": {"summary": []}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`osds 1200`),
				regexp.MustCompile(`osds_up 1200`),
				regexp.MustCompile(`osds_in 1190`),
				regexp.MustCompile(`pgs_remapped 10`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 1200,
			"num_up_osds": 1200,
			"num_in_osds": 1190,
			"num_remapped_pgs": 10
		}
	},
	"health": { "overall_status": "HEALTH_OK" } }`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 0`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 1200,
			"num_up_osds": 1200,
			"num_in_osds": 1190,
			"num_remapped_pgs": 10
		}
	},
	"health": { "overall_status": "HEALTH_WARN" } }`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 1`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 1200,
			"num_up_osds": 1200,
			"num_in_osds": 1190,
			"num_remapped_pgs": 10
		}
	},
	"health": { "overall_status": "HEALTH_ERR" } }`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 2`),
			},
		},
		{
			input: `
$ sudo ceph -s
    cluster eff51be8-938a-4afa-b0d1-7a580b4ceb37
     health HEALTH_OK
     monmap e3: 3 mons at {mon01,mon02,mon03}
  recovery io 5779 MB/s, 4 keys/s, 1522 objects/s
  client io 4273 kB/s rd, 2740 MB/s wr, 2863 op/s
`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`recovery_io_bytes 5.779e`),
				regexp.MustCompile(`recovery_io_keys 4`),
				regexp.MustCompile(`recovery_io_objects 1522`),
				regexp.MustCompile(`client_io_ops 2863`),
				regexp.MustCompile(`client_io_read_bytes 4.273e`),
				regexp.MustCompile(`client_io_write_bytes 2.74e`),
			},
		},
		{
			input: `
$ sudo ceph -s
    cluster eff51be8-938a-4afa-b0d1-7a580b4ceb37
     health HEALTH_OK
     monmap e3: 3 mons at {mon01,mon02,mon03}
  recovery io 5779 MB/s, 4 keys/s, 1522 objects/s
  client io 2863 op/s rd, 5847 op/s wr
  cache io 251 MB/s flush, 6646 kB/s evict, 55 op/s promote
`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`recovery_io_bytes 5.779e`),
				regexp.MustCompile(`recovery_io_keys 4`),
				regexp.MustCompile(`recovery_io_objects 1522`),
				regexp.MustCompile(`client_io_ops 8710`),
				regexp.MustCompile(`client_io_read_ops 2863`),
				regexp.MustCompile(`client_io_write_ops 5847`),
				regexp.MustCompile(`cache_flush_io_bytes 2.51e`),
				regexp.MustCompile(`cache_evict_io_bytes 6.646e`),
				regexp.MustCompile(`cache_promote_io_ops 55`),
			},
		},
		{
			input: `
{
	"osdmap": {
		"osdmap": {
			"num_osds": 0,
			"num_up_osds": 0,
			"num_in_osds": 0,
			"num_remapped_pgs": 0
		}
	},
	"pgmap": { "num_pgs": 52000 },
	"health": {"summary": [{"severity": "HEALTH_WARN", "summary": "7 pgs undersized"}]}
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`total_pgs 52000`),
			},
		},
	} {
		func() {
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutputFile: func(actionName string, command string, outFileArg string, args ...string) (string, error) {
					return tt.input, nil
				},
			}
			configDir, _ := ioutil.TempDir("", "")
			context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
			defer os.RemoveAll(context.ConfigDir)
			collector := NewClusterHealthCollector(context, "mycluster")
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

			for _, re := range tt.regexes {
				assert.True(t, re.Match(buf), fmt.Sprintf("failed matching: %q", re))
			}
		}()
	}
}
