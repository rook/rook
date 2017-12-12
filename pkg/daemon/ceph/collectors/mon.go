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

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

// MonitorCollector is used to extract stats related to monitors
// running within Ceph cluster. As we extract information pertaining
// to each monitor instance, there are various vector metrics we
// need to use.
type MonitorCollector struct {
	// Context for executing commands against the Ceph cluster
	context *clusterd.Context

	// The name of the ceph cluster
	clusterName string

	// ClockSkew shows how far the monitor clocks have skewed from each other. This
	// is an important metric because the functioning of Ceph's paxos depends on
	// the clocks being aligned as close to each other as possible.
	ClockSkew *prometheus.GaugeVec

	// Latency displays the time the monitors take to communicate between themselves.
	Latency *prometheus.GaugeVec

	// NodesinQuorum show the size of the working monitor quorum. Any change in this
	// metric can imply a significant issue in the cluster if it is not manually changed.
	NodesinQuorum prometheus.Gauge
}

// NewMonitorCollector creates an instance of the MonitorCollector and instantiates
// the individual metrics that show information about the monitor processes.
func NewMonitorCollector(context *clusterd.Context, clusterName string) *MonitorCollector {
	return &MonitorCollector{
		context:     context,
		clusterName: clusterName,

		ClockSkew: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_clock_skew_seconds",
				Help:      "Clock skew the monitor node is incurring",
			},
			[]string{"monitor"},
		),
		Latency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_latency_seconds",
				Help:      "Latency the monitor node is incurring",
			},
			[]string{"monitor"},
		),
		NodesinQuorum: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_quorum_count",
				Help:      "The total size of the monitor quorum",
			},
		),
	}
}

func (m *MonitorCollector) collectorList() []prometheus.Collector {
	return []prometheus.Collector{
		m.ClockSkew,
		m.Latency,
	}
}

func (m *MonitorCollector) metricsList() []prometheus.Metric {
	return []prometheus.Metric{
		m.NodesinQuorum,
	}
}

func (m *MonitorCollector) collect() error {
	status, err := cephclient.GetMonTimeStatus(m.context, m.clusterName)
	if err != nil {
		return err
	}

	for name, s := range status.Skew {
		skew, err := s.Skew.Float64()
		if err != nil {
			return err
		}
		m.ClockSkew.WithLabelValues(name).Set(skew)

		latency, err := s.Latency.Float64()
		if err != nil {
			return err
		}
		m.Latency.WithLabelValues(name).Set(latency)
	}

	stats, err := cephclient.GetMonStats(m.context, m.clusterName)
	if err != nil {
		return err
	}

	m.NodesinQuorum.Set(float64(len(stats.Quorum)))

	return nil
}

// Describe sends the descriptors of each Monitor related metric we have defined
// to the channel provided.
func (m *MonitorCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range m.collectorList() {
		metric.Describe(ch)
	}

	for _, metric := range m.metricsList() {
		ch <- metric.Desc()
	}
}

// Collect extracts the given metrics from the Monitors and sends it to the prometheus
// channel.
func (m *MonitorCollector) Collect(ch chan<- prometheus.Metric) {
	if err := m.collect(); err != nil {
		logger.Errorf("failed collecting monitor metrics: %+v", err)
		return
	}

	for _, metric := range m.collectorList() {
		metric.Collect(ch)
	}

	for _, metric := range m.metricsList() {
		ch <- metric
	}
}
