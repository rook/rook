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
package api

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	ceph "github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/collectors"
)

// CephExporter wraps all the ceph collectors and provides a single global
// exporter to extracts metrics out of. It also ensures that the collection
// is done in a thread-safe manner, the necessary requirement stated by
// prometheus. It also implements a prometheus.Collector interface in order
// to register it correctly.
type CephExporter struct {
	mu         sync.Mutex
	collectors []prometheus.Collector
	handler    *Handler
	conn       ceph.Connection
}

// Verify that the exporter implements the interface correctly.
var _ prometheus.Collector = &CephExporter{}

// NewCephExporter creates an instance to CephExporter and returns a reference
// to it. We can choose to enable a collector to extract stats out of by adding
// it to the list of collectors.
func NewCephExporter(handler *Handler, conn ceph.Connection) *CephExporter {
	return &CephExporter{
		handler:    handler,
		conn:       conn,
		collectors: createCollectors(conn),
	}
}

// Describe sends all the descriptors of the collectors included to
// the provided channel.
func (c *CephExporter) Describe(ch chan<- *prometheus.Desc) {
	for _, cc := range c.collectors {
		cc.Describe(ch)
	}
}

// Collect sends the collected metrics from each of the collectors to
// prometheus. Collect could be called several times concurrently
// and thus its run is protected by a single mutex.
func (c *CephExporter) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// test the ceph connection to make sure it's still active/alive
	_, err := ceph.Status(c.conn)
	if err != nil {
		// the ceph connection doesn't appear to be alive, close the old one and create a new one then recreate the collectors
		logger.Noticef("metrics ceph connection appears down, will try to restart it: %+v", err)
		c.conn.Shutdown()

		newConn, err := c.handler.connectToCeph()
		if err != nil {
			logger.Errorf("metrics ceph connection cannot be restarted: %+v", err)
			return
		}

		logger.Noticef("metrics ceph connection was reconnected")
		c.conn = newConn
		c.collectors = createCollectors(c.conn)
	}

	// collect metrics from all of our collectors
	for _, cc := range c.collectors {
		cc.Collect(ch)
	}
}

func createCollectors(conn ceph.Connection) []prometheus.Collector {
	return []prometheus.Collector{
		collectors.NewClusterUsageCollector(conn),
		collectors.NewClusterHealthCollector(conn),
		collectors.NewMonitorCollector(conn),
		collectors.NewOSDCollector(conn),
		collectors.NewPoolUsageCollector(conn),
	}
}
