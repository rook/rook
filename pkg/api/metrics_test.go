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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	ceph "github.com/rook/rook/pkg/cephmgr/client"
	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
)

func TestMetricsCollectFailureRetry(t *testing.T) {
	context := &clusterd.Context{}
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}

	connectCount := 0
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		connectCount++
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// mock it so the first status mon command attempt will fail (even though the initial connection was established OK)
	attempt := 0
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			attempt++
			switch {
			case strings.Index(string(args), "status") != -1:
				if attempt <= 1 {
					// first attempt for status mon command should fail
					return nil, "", fmt.Errorf("mock mon command failure")
				}
				// subsequent attempts return a good response
				return []byte(CephStatusResponseRaw), "info", nil
			}

			// mock fail all other mon commands (each collector will make several, but we don't care about their results)
			return nil, "", fmt.Errorf("mock mon command failure for '%s'", string(args))
		},
	}

	// create the handler and connect to ceph
	h := newTestHandler(context, connFactory, cephFactory)
	conn, err := h.connectToCeph()
	assert.Nil(t, err)
	assert.NotNil(t, conn)

	// create a ceph metrics exporter that will use the ceph connection
	cephExporter := NewCephExporter(h, conn)
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
	assert.Equal(t, 2, connectCount)
}
