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
	"fmt"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMetricsCollectFailureRetry(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	// mock it so the first status mon command attempt will fail (even though the initial connection was established OK)
	attempt := 0
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		attempt++
		switch {
		case args[0] == "status":
			if attempt <= 1 {
				// first attempt for status mon command should fail
				return "", fmt.Errorf("mock mon command failure")
			}
			// subsequent attempts return a good response
			return CephStatusResponseRaw, nil
		}

		// mock fail all other mon commands (each collector will make several, but we don't care about their results)
		return "", fmt.Errorf("mock mon command failure for '%v'", args)
	}

	// create the handler
	h := newTestHandler(context)

	// create a ceph metrics exporter that will use the ceph connection
	cephExporter := NewCephExporter(h)
	assert.NotNil(t, cephExporter)

	// try to collect ceph metrics, this will fail at first due to the mock failed mon command above, but
	// then it should reestablish the connection successfully and complete the collection process
	ch := make(chan prometheus.Metric)
	go func() {
		// in a goroutine, read from the metrics channel (but discard the results) so that the collection process can complete
		for _ = range ch {
		}
	}()
	cephExporter.Collect(ch)

	// there should have been 2 connection attempts: initial and the retry after the failed mon command
	assert.Equal(t, 2, attempt)
}
