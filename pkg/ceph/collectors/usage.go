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
	"github.com/prometheus/client_golang/prometheus"

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	cephNamespace = "ceph"
)

// A ClusterUsageCollector is used to gather all the global stats about a given
// ceph cluster. It is sometimes essential to know how fast the cluster is growing
// or shrinking as a whole in order to zero in on the cause. The pool specific
// stats are provided separately.
type ClusterUsageCollector struct {
	// Context for executing commands against the Ceph cluster
	context *clusterd.Context

	// The name of the ceph cluster
	clusterName string

	// GlobalCapacity displays the total storage capacity of the cluster. This
	// information is based on the actual no. of objects that are allocated. It
	// does not take overcommitment into consideration.
	GlobalCapacity prometheus.Gauge

	// UsedCapacity shows the storage under use.
	UsedCapacity prometheus.Gauge

	// AvailableCapacity shows the remaining capacity of the cluster that is left unallocated.
	AvailableCapacity prometheus.Gauge

	// Objects show the total no. of RADOS objects that are currently allocated.
	Objects prometheus.Gauge
}

// NewClusterUsageCollector creates and returns the reference to ClusterUsageCollector
// and internally defines each metric that display cluster stats.
func NewClusterUsageCollector(context *clusterd.Context, clusterName string) *ClusterUsageCollector {
	return &ClusterUsageCollector{
		context:     context,
		clusterName: clusterName,

		GlobalCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: cephNamespace,
			Name:      "cluster_capacity_bytes",
			Help:      "Total capacity of the cluster",
		}),
		UsedCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: cephNamespace,
			Name:      "cluster_used_bytes",
			Help:      "Capacity of the cluster currently in use",
		}),
		AvailableCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: cephNamespace,
			Name:      "cluster_available_bytes",
			Help:      "Available space within the cluster",
		}),
		Objects: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: cephNamespace,
			Name:      "cluster_objects",
			Help:      "No. of rados objects within the cluster",
		}),
	}
}

func (c *ClusterUsageCollector) metricsList() []prometheus.Metric {
	return []prometheus.Metric{
		c.GlobalCapacity,
		c.UsedCapacity,
		c.AvailableCapacity,
		c.Objects,
	}
}

func (c *ClusterUsageCollector) collect() error {
	stats, err := ceph.Usage(c.context, c.clusterName)
	if err != nil {
		return err
	}

	var totBytes, usedBytes, availBytes, totObjects float64

	totBytes, err = stats.Stats.TotalBytes.Float64()
	if err != nil {
		logger.Errorf("cannot extract total bytes: %+v", err)
	}

	usedBytes, err = stats.Stats.TotalUsedBytes.Float64()
	if err != nil {
		logger.Errorf("cannot extract used bytes: %+v", err)
	}

	availBytes, err = stats.Stats.TotalAvailBytes.Float64()
	if err != nil {
		logger.Errorf("cannot extract available bytes: %+v", err)
	}

	totObjects, err = stats.Stats.TotalObjects.Float64()
	if err != nil {
		logger.Errorf("cannot extract total objects: %+v", err)
	}

	c.GlobalCapacity.Set(totBytes)
	c.UsedCapacity.Set(usedBytes)
	c.AvailableCapacity.Set(availBytes)
	c.Objects.Set(totObjects)

	return nil
}

// Describe sends the descriptors of each metric over to the provided channel.
// The corresponding metric values are sent separately.
func (c *ClusterUsageCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metricsList() {
		ch <- metric.Desc()
	}
}

// Collect sends the metric values for each metric pertaining to the global
// cluster usage over to the provided prometheus Metric channel.
func (c *ClusterUsageCollector) Collect(ch chan<- prometheus.Metric) {
	if err := c.collect(); err != nil {
		logger.Errorf("failed collecting cluster usage metrics: %+v", err)
		return
	}

	for _, metric := range c.metricsList() {
		ch <- metric
	}
}
