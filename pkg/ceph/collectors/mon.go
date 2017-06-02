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

	cephclient "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
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

	// TotalKBs display the total storage a given monitor node has.
	TotalKBs *prometheus.GaugeVec

	// UsedKBs depict how much of the total storage our monitor process
	// has utilized.
	UsedKBs *prometheus.GaugeVec

	// AvailKBs shows the space left unused.
	AvailKBs *prometheus.GaugeVec

	// PercentAvail shows the amount of unused space as a percentage of total
	// space.
	PercentAvail *prometheus.GaugeVec

	// Store exposes information about internal backing store.
	Store Store

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

// Store displays information about Monitor's FileStore. It is responsible for
// storing all the meta information about the cluster, including monmaps, osdmaps,
// pgmaps, etc. along with logs and other data.
type Store struct {
	// TotalBytes displays the current size of the FileStore.
	TotalBytes *prometheus.GaugeVec

	// SSTBytes shows the amount used by LevelDB's sorted-string tables.
	SSTBytes *prometheus.GaugeVec

	// LogBytes shows the amount used by logs.
	LogBytes *prometheus.GaugeVec

	// MiscBytes shows the amount used by miscellaneous information.
	MiscBytes *prometheus.GaugeVec
}

// NewMonitorCollector creates an instance of the MonitorCollector and instantiates
// the individual metrics that show information about the monitor processes.
func NewMonitorCollector(context *clusterd.Context, clusterName string) *MonitorCollector {
	return &MonitorCollector{
		context:     context,
		clusterName: clusterName,

		TotalKBs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_capacity_bytes",
				Help:      "Total storage capacity of the monitor node",
			},
			[]string{"monitor"},
		),
		UsedKBs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_used_bytes",
				Help:      "Storage of the monitor node that is currently allocated for use",
			},
			[]string{"monitor"},
		),
		AvailKBs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_avail_bytes",
				Help:      "Total unused storage capacity that the monitor node has left",
			},
			[]string{"monitor"},
		),
		PercentAvail: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "monitor_avail_percent",
				Help:      "Percentage of total unused storage capacity that the monitor node has left",
			},
			[]string{"monitor"},
		),
		Store: Store{
			TotalBytes: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: cephNamespace,
					Name:      "monitor_store_capacity_bytes",
					Help:      "Total capacity of the FileStore backing the monitor daemon",
				},
				[]string{"monitor"},
			),
			SSTBytes: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: cephNamespace,
					Name:      "monitor_store_sst_bytes",
					Help:      "Capacity of the FileStore used only for raw SSTs",
				},
				[]string{"monitor"},
			),
			LogBytes: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: cephNamespace,
					Name:      "monitor_store_log_bytes",
					Help:      "Capacity of the FileStore used only for logging",
				},
				[]string{"monitor"},
			),
			MiscBytes: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: cephNamespace,
					Name:      "monitor_store_misc_bytes",
					Help:      "Capacity of the FileStore used only for storing miscellaneous information",
				},
				[]string{"monitor"},
			),
		},
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
		m.TotalKBs,
		m.UsedKBs,
		m.AvailKBs,
		m.PercentAvail,

		m.Store.TotalBytes,
		m.Store.SSTBytes,
		m.Store.LogBytes,
		m.Store.MiscBytes,

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
	stats, err := cephclient.GetMonStats(m.context, m.clusterName)
	if err != nil {
		return err
	}

	for _, healthService := range stats.Health.Health.HealthServices {
		for _, monstat := range healthService.Mons {
			kbTotal, err := monstat.KBTotal.Float64()
			if err != nil {
				return err
			}
			m.TotalKBs.WithLabelValues(monstat.Name).Set(kbTotal * 1e3)

			kbUsed, err := monstat.KBUsed.Float64()
			if err != nil {
				return err
			}
			m.UsedKBs.WithLabelValues(monstat.Name).Set(kbUsed * 1e3)

			kbAvail, err := monstat.KBAvail.Float64()
			if err != nil {
				return err
			}
			m.AvailKBs.WithLabelValues(monstat.Name).Set(kbAvail * 1e3)

			percentAvail, err := monstat.AvailPercent.Float64()
			if err != nil {
				return err
			}
			m.PercentAvail.WithLabelValues(monstat.Name).Set(percentAvail)

			storeBytes, err := monstat.StoreStats.BytesTotal.Float64()
			if err != nil {
				return err
			}
			m.Store.TotalBytes.WithLabelValues(monstat.Name).Set(storeBytes)

			sstBytes, err := monstat.StoreStats.BytesSST.Float64()
			if err != nil {
				return err
			}
			m.Store.SSTBytes.WithLabelValues(monstat.Name).Set(sstBytes)

			logBytes, err := monstat.StoreStats.BytesLog.Float64()
			if err != nil {
				return err
			}
			m.Store.LogBytes.WithLabelValues(monstat.Name).Set(logBytes)

			miscBytes, err := monstat.StoreStats.BytesMisc.Float64()
			if err != nil {
				return err
			}
			m.Store.MiscBytes.WithLabelValues(monstat.Name).Set(miscBytes)
		}
	}

	for _, monstat := range stats.Health.TimeChecks.Mons {
		skew, err := monstat.Skew.Float64()
		if err != nil {
			return err
		}
		m.ClockSkew.WithLabelValues(monstat.Name).Set(skew)

		latency, err := monstat.Latency.Float64()
		if err != nil {
			return err
		}
		m.Latency.WithLabelValues(monstat.Name).Set(latency)
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
