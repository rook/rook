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
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	cephclient "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

// PoolUsageCollector displays statistics about each pool we have created
// in the ceph cluster.
type PoolUsageCollector struct {
	// Context for executing commands against the Ceph cluster
	context *clusterd.Context

	// The name of the ceph cluster
	clusterName string

	// UsedBytes tracks the amount of bytes currently allocated for the pool. This
	// does not factor in the overcommitment made for individual images.
	UsedBytes *prometheus.GaugeVec

	// RawUsedBytes tracks the amount of raw bytes currently used for the pool. This
	// factors in the replication factor (size) of the pool.
	RawUsedBytes *prometheus.GaugeVec

	// MaxAvail tracks the amount of bytes currently free for the pool,
	// which depends on the replication settings for the pool in question.
	MaxAvail *prometheus.GaugeVec

	// Objects shows the no. of RADOS objects created within the pool.
	Objects *prometheus.GaugeVec

	// DirtyObjects shows the no. of RADOS dirty objects in a cache-tier pool,
	// this doesn't make sense in a regular pool, see:
	// http://lists.ceph.com/pipermail/ceph-users-ceph.com/2015-April/000557.html
	DirtyObjects *prometheus.GaugeVec

	// ReadIO tracks the read IO calls made for the images within each pool.
	ReadIO *prometheus.GaugeVec

	// Readbytes tracks the read throughput made for the images within each pool.
	ReadBytes *prometheus.GaugeVec

	// WriteIO tracks the write IO calls made for the images within each pool.
	WriteIO *prometheus.GaugeVec

	// WriteBytes tracks the write throughput made for the images within each pool.
	WriteBytes *prometheus.GaugeVec
}

// NewPoolUsageCollector creates a new instance of PoolUsageCollector and returns
// its reference.
func NewPoolUsageCollector(context *clusterd.Context, clusterName string) *PoolUsageCollector {
	var (
		subSystem = "pool"
		poolLabel = []string{"pool"}
	)
	return &PoolUsageCollector{
		context:     context,
		clusterName: clusterName,

		UsedBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "used_bytes",
				Help:      "Capacity of the pool that is currently under use",
			},
			poolLabel,
		),
		RawUsedBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "raw_used_bytes",
				Help:      "Raw capacity of the pool that is currently under use, this factors in the size",
			},
			poolLabel,
		),
		MaxAvail: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "available_bytes",
				Help:      "Free space for this ceph pool",
			},
			poolLabel,
		),
		Objects: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "objects_total",
				Help:      "Total no. of objects allocated within the pool",
			},
			poolLabel,
		),
		DirtyObjects: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "dirty_objects_total",
				Help:      "Total no. of dirty objects in a cache-tier pool",
			},
			poolLabel,
		),
		ReadIO: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "read_total",
				Help:      "Total read i/o calls for the pool",
			},
			poolLabel,
		),
		ReadBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "read_bytes_total",
				Help:      "Total read throughput for the pool",
			},
			poolLabel,
		),
		WriteIO: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "write_total",
				Help:      "Total write i/o calls for the pool",
			},
			poolLabel,
		),
		WriteBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Subsystem: subSystem,
				Name:      "write_bytes_total",
				Help:      "Total write throughput for the pool",
			},
			poolLabel,
		),
	}
}

func (p *PoolUsageCollector) collectorList() []prometheus.Collector {
	return []prometheus.Collector{
		p.UsedBytes,
		p.RawUsedBytes,
		p.MaxAvail,
		p.Objects,
		p.DirtyObjects,
		p.ReadIO,
		p.ReadBytes,
		p.WriteIO,
		p.WriteBytes,
	}
}

func (p *PoolUsageCollector) collect() error {
	stats, err := cephclient.GetPoolStats(p.context, p.clusterName)
	if err != nil {
		return err
	}

	if len(stats.Pools) < 1 {
		return errors.New("no pools found in the cluster to report stats on")
	}

	for _, pool := range stats.Pools {
		p.UsedBytes.WithLabelValues(pool.Name).Set(pool.Stats.BytesUsed)
		p.RawUsedBytes.WithLabelValues(pool.Name).Set(pool.Stats.RawBytesUsed)
		p.MaxAvail.WithLabelValues(pool.Name).Set(pool.Stats.MaxAvail)
		p.Objects.WithLabelValues(pool.Name).Set(pool.Stats.Objects)
		p.DirtyObjects.WithLabelValues(pool.Name).Set(pool.Stats.DirtyObjects)
		p.ReadIO.WithLabelValues(pool.Name).Set(pool.Stats.ReadIO)
		p.ReadBytes.WithLabelValues(pool.Name).Set(pool.Stats.ReadBytes)
		p.WriteIO.WithLabelValues(pool.Name).Set(pool.Stats.WriteIO)
		p.WriteBytes.WithLabelValues(pool.Name).Set(pool.Stats.WriteBytes)
	}

	return nil
}

// Describe fulfills the prometheus.Collector's interface and sends the descriptors
// of pool's metrics to the given channel.
func (p *PoolUsageCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range p.collectorList() {
		metric.Describe(ch)
	}
}

// Collect extracts the current values of all the metrics and sends them to the
// prometheus channel.
func (p *PoolUsageCollector) Collect(ch chan<- prometheus.Metric) {
	if err := p.collect(); err != nil {
		logger.Errorf("failed collecting pool usage metrics: %+v", err)
		return
	}

	for _, metric := range p.collectorList() {
		metric.Collect(ch)
	}
}
