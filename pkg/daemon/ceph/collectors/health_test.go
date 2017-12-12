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
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 5 pgs degraded"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`degraded_pgs 5`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 15 pgs stuck degraded"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_degraded_pgs 15`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 6 pgs unclean, 48 pgs stale"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`unclean_pgs 6`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 16 pgs stuck unclean"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_unclean_pgs 16`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 7 pgs undersized"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`undersized_pgs 7`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 17 pgs stuck undersized"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_undersized_pgs 17`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 8 pgs stale"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stale_pgs 8`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: 18 pgs stuck stale"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`stuck_stale_pgs 18`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: recovery 10/20 objects degraded"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`degraded_objects 10`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {
					"PG_DEGRADED": {
						"severity": "HEALTH_WARN",
						"summary": {
						"message": "Degraded data redundancy: recovery 20/40 objects misplaced"
						}
					}
					},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
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
				"health": {
					"checks": {},
					"status": "HEALTH_OK",
					"overall_status": "HEALTH_OK"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 0`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {},
					"status": "HEALTH_WARN",
					"overall_status": "HEALTH_WARN"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 1`),
			},
		},
		{
			input: `
			{
				"health": {
					"checks": {},
					"status": "HEALTH_ERR",
					"overall_status": "HEALTH_ERR"
				}
			}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`health_status 2`),
			},
		},
		{
			input: `
{
	"pgmap": {
		"read_bytes_sec": 1234,
		"write_bytes_sec": 5678,
		"read_op_per_sec": 10,
		"write_op_per_sec": 20,
		"recovering_bytes_per_sec": 123,
		"recovering_objects_per_sec": 1522,
		"recovering_keys_per_sec": 4,
		"flush_bytes_sec": 2510,
		"evict_bytes_sec": 6646,
		"promote_op_per_sec": 55
	}
}
`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`recovery_io_bytes 123`),
				regexp.MustCompile(`recovery_io_keys 4`),
				regexp.MustCompile(`recovery_io_objects 1522`),
				regexp.MustCompile(`client_io_ops 30`),
				regexp.MustCompile(`client_io_read_bytes 1234`),
				regexp.MustCompile(`client_io_write_bytes 5678`),
				regexp.MustCompile(`cache_flush_io_bytes 2510`),
				regexp.MustCompile(`cache_evict_io_bytes 6646`),
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
		{
			input: `
{
  "health": {
    "checks": {
      "REQUEST_SLOW": {
        "severity": "HEALTH_WARN",
        "summary": {
          "message": "6 slow requests are blocked > 32 sec"
        }
      }
    }
  }
}`,
			regexes: []*regexp.Regexp{
				regexp.MustCompile(`slow_requests 6`),
			},
		},
	} {
		func() {
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
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
				if !re.Match(buf) {
					assert.True(t, re.Match(buf), fmt.Sprintf("failed matching: %q", re))
				}
			}
		}()
	}
}
