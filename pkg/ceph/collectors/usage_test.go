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

func TestClusterUsage(t *testing.T) {
	for _, tt := range []struct {
		input              string
		reMatch, reUnmatch []*regexp.Regexp
	}{
		{
			input: `
{
	"stats": {
		"total_bytes": 10,
		"total_used_bytes": 6,
		"total_avail_bytes": 4,
		"total_objects": 1
	}
}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes 10`),
				regexp.MustCompile(`ceph_cluster_used_bytes 6`),
				regexp.MustCompile(`ceph_cluster_available_bytes 4`),
				regexp.MustCompile(`ceph_cluster_objects 1`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{
	"stats": {
		"total_used_bytes": 6,
		"total_avail_bytes": 4,
		"total_objects": 1
	}
}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes 0`),
				regexp.MustCompile(`ceph_cluster_used_bytes 6`),
				regexp.MustCompile(`ceph_cluster_available_bytes 4`),
				regexp.MustCompile(`ceph_cluster_objects 1`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{
	"stats": {
		"total_bytes": 10,
		"total_avail_bytes": 4,
		"total_objects": 1
	}
}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes 10`),
				regexp.MustCompile(`ceph_cluster_used_bytes 0`),
				regexp.MustCompile(`ceph_cluster_available_bytes 4`),
				regexp.MustCompile(`ceph_cluster_objects 1`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{
	"stats": {
		"total_bytes": 10,
		"total_used_bytes": 6,
		"total_objects": 1
	}
}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes 10`),
				regexp.MustCompile(`ceph_cluster_used_bytes 6`),
				regexp.MustCompile(`ceph_cluster_available_bytes 0`),
				regexp.MustCompile(`ceph_cluster_objects 1`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{
	"stats": {
		"total_bytes": 10,
		"total_used_bytes": 6,
		"total_avail_bytes": 4
	}
}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes 10`),
				regexp.MustCompile(`ceph_cluster_used_bytes 6`),
				regexp.MustCompile(`ceph_cluster_available_bytes 4`),
				regexp.MustCompile(`ceph_cluster_objects 0`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{
	"stats": {{{
		"total_bytes": 10,
		"total_used_bytes": 6,
		"total_avail_bytes": 4,
		"total_objects": 1
	}
}`,
			reMatch: []*regexp.Regexp{},
			reUnmatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_cluster_capacity_bytes`),
				regexp.MustCompile(`ceph_cluster_used_bytes`),
				regexp.MustCompile(`ceph_cluster_available_bytes`),
				regexp.MustCompile(`ceph_cluster_objects`),
			},
		},
	} {
		func() {
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
					return tt.input, nil
				},
			}
			configDir, _ := ioutil.TempDir("", "")
			context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
			defer os.RemoveAll(context.ConfigDir)
			collector := NewClusterUsageCollector(context, "mycluster")
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

			for _, re := range tt.reMatch {
				assert.True(t, re.Match(buf), fmt.Sprintf("failed matching: %q", re))
			}
			for _, re := range tt.reUnmatch {
				assert.False(t, re.Match(buf), fmt.Sprintf("should not have matched: %q", re))
			}
		}()
	}
}
