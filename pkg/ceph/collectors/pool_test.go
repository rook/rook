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
	"log"
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

func TestPoolUsageCollector(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	for _, tt := range []struct {
		input              string
		reMatch, reUnmatch []*regexp.Regexp
	}{
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "objects": 5, "rd": 4, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 20`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 5`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 4`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 6`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"objects": 5, "rd": 4, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 0`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 5`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 4`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 6`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "rd": 4, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 20`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 0`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 4`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 6`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "objects": 5, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 20`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 5`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 0`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 6`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "objects": 5, "rd": 4}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 20`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 5`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 4`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 0`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
    {{{{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "objects": 5, "rd": 4, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{},
			reUnmatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes`),
				regexp.MustCompile(`pool_objects_total`),
				regexp.MustCompile(`pool_read_total`),
				regexp.MustCompile(`pool_write_total`),
			},
		},
		{
			input: `
{"pools": [
	{"name": "rbd", "id": 11, "stats": {"bytes_used": 20, "objects": 5, "rd": 4, "wr": 6}},
	{"name": "rbd-new", "id": 12, "stats": {"bytes_used": 50, "objects": 20, "rd": 10, "wr": 30}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_used_bytes{pool="rbd"} 20`),
				regexp.MustCompile(`pool_objects_total{pool="rbd"} 5`),
				regexp.MustCompile(`pool_read_total{pool="rbd"} 4`),
				regexp.MustCompile(`pool_write_total{pool="rbd"} 6`),
				regexp.MustCompile(`pool_used_bytes{pool="rbd-new"} 50`),
				regexp.MustCompile(`pool_objects_total{pool="rbd-new"} 20`),
				regexp.MustCompile(`pool_read_total{pool="rbd-new"} 10`),
				regexp.MustCompile(`pool_write_total{pool="rbd-new"} 30`),
			},
			reUnmatch: []*regexp.Regexp{},
		},
		{
			input: `
{"pools": [
	{"name": "ssd", "id": 11, "stats": {"max_avail": 4618201748262, "objects": 5, "rd": 4, "wr": 6}}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`pool_available_bytes{pool="ssd"} 4.618201748262e\+12`),
			},
		},
		{
			input: `
{"pools": [
	{"id": 32, "name": "cinder_sas", "stats": { "bytes_used": 71525351713, "dirty": 17124, "kb_used": 69848977, "max_avail": 6038098673664, "objects": 17124, "quota_bytes": 0, "quota_objects": 0, "raw_bytes_used": 214576054272, "rd": 348986643, "rd_bytes": 3288983853056, "wr": 45792703, "wr_bytes": 272268791808 }},
	{"id": 33, "name": "cinder_ssd", "stats": { "bytes_used": 68865564849, "dirty": 16461, "kb_used": 67251529, "max_avail": 186205372416, "objects": 16461, "quota_bytes": 0, "quota_objects": 0, "raw_bytes_used": 206596702208, "rd": 347, "rd_bytes": 12899328, "wr": 26721, "wr_bytes": 68882356224 }}
]}`,
			reMatch: []*regexp.Regexp{
				regexp.MustCompile(`ceph_pool_available_bytes{pool="cinder_sas"} 6.038098673664e\+12`),
				regexp.MustCompile(`ceph_pool_dirty_objects_total{pool="cinder_sas"} 17124`),
				regexp.MustCompile(`ceph_pool_objects_total{pool="cinder_sas"} 17124`),
				regexp.MustCompile(`ceph_pool_raw_used_bytes{pool="cinder_sas"} 2.14576054272e\+11`),
				regexp.MustCompile(`ceph_pool_read_bytes_total{pool="cinder_sas"} 3.288983853056e\+12`),
				regexp.MustCompile(`ceph_pool_read_total{pool="cinder_sas"} 3.48986643e\+08`),
				regexp.MustCompile(`ceph_pool_used_bytes{pool="cinder_sas"} 7.1525351713e\+10`),
				regexp.MustCompile(`ceph_pool_write_bytes_total{pool="cinder_sas"} 2.72268791808e\+11`),
				regexp.MustCompile(`ceph_pool_write_total{pool="cinder_sas"} 4.5792703e\+07`),
				regexp.MustCompile(`ceph_pool_available_bytes{pool="cinder_ssd"} 1.86205372416e\+11`),
				regexp.MustCompile(`ceph_pool_dirty_objects_total{pool="cinder_ssd"} 16461`),
				regexp.MustCompile(`ceph_pool_objects_total{pool="cinder_ssd"} 16461`),
				regexp.MustCompile(`ceph_pool_raw_used_bytes{pool="cinder_ssd"} 2.06596702208e\+11`),
				regexp.MustCompile(`ceph_pool_read_bytes_total{pool="cinder_ssd"} 1.2899328e\+07`),
				regexp.MustCompile(`ceph_pool_read_total{pool="cinder_ssd"} 347`),
				regexp.MustCompile(`ceph_pool_used_bytes{pool="cinder_ssd"} 6.8865564849e\+10`),
				regexp.MustCompile(`ceph_pool_write_bytes_total{pool="cinder_ssd"} 6.8882356224e\+10`),
				regexp.MustCompile(`ceph_pool_write_total{pool="cinder_ssd"} 26721`),
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
			collector := NewPoolUsageCollector(context, "mycluster")
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
