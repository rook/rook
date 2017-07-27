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

	"github.com/prometheus/client_golang/prometheus"

	cephclient "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

// OSDCollector displays statistics about OSD in the ceph cluster.
// An important aspect of monitoring OSDs is to ensure that when the cluster is up and
// running that all OSDs that are in the cluster are up and running, too
type OSDCollector struct {
	// Context for executing commands against the Ceph cluster
	context *clusterd.Context

	// The name of the ceph cluster
	clusterName string

	// CrushWeight is a persistent setting, and it affects how CRUSH assigns data to OSDs.
	// It displays the CRUSH weight for the OSD
	CrushWeight *prometheus.GaugeVec

	// Depth displays the OSD's level of hierarchy in the CRUSH map
	Depth *prometheus.GaugeVec

	// Reweight sets an override weight on the OSD.
	// It displays value within 0 to 1.
	Reweight *prometheus.GaugeVec

	// Bytes displays the total bytes available in the OSD
	Bytes *prometheus.GaugeVec

	// UsedBytes displays the total used bytes in the OSD
	UsedBytes *prometheus.GaugeVec

	// AvailBytes displays the total available bytes in the OSD
	AvailBytes *prometheus.GaugeVec

	// Utilization displays current utilization of the OSD
	Utilization *prometheus.GaugeVec

	// Variance displays current variance of the OSD from the standard utilization
	Variance *prometheus.GaugeVec

	// Pgs displays total no. of placement groups in the OSD.
	// Available in Ceph Jewel version.
	Pgs *prometheus.GaugeVec

	// CommitLatency displays in seconds how long it takes for an operation to be applied to disk
	CommitLatency *prometheus.GaugeVec

	// ApplyLatency displays in seconds how long it takes to get applied to the backing filesystem
	ApplyLatency *prometheus.GaugeVec

	// OSDIn displays the In state of the OSD
	OSDIn *prometheus.GaugeVec

	// OSDUp displays the Up state of the OSD
	OSDUp *prometheus.GaugeVec

	// TotalBytes displays total bytes in all OSDs
	TotalBytes prometheus.Gauge

	// TotalUsedBytes displays total used bytes in all OSDs
	TotalUsedBytes prometheus.Gauge

	// TotalAvailBytes displays total available bytes in all OSDs
	TotalAvailBytes prometheus.Gauge

	// AverageUtil displays average utilization in all OSDs
	AverageUtil prometheus.Gauge
}

//NewOSDCollector creates an instance of the OSDCollector and instantiates
// the individual metrics that show information about the OSD.
func NewOSDCollector(context *clusterd.Context, clusterName string) *OSDCollector {
	return &OSDCollector{
		context:     context,
		clusterName: clusterName,

		CrushWeight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_crush_weight",
				Help:      "OSD Crush Weight",
			},
			[]string{"osd"},
		),

		Depth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_depth",
				Help:      "OSD Depth",
			},
			[]string{"osd"},
		),

		Reweight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_reweight",
				Help:      "OSD Reweight",
			},
			[]string{"osd"},
		),

		Bytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_bytes",
				Help:      "OSD Total Bytes",
			},
			[]string{"osd"},
		),

		UsedBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_used_bytes",
				Help:      "OSD Used Storage in Bytes",
			},
			[]string{"osd"},
		),

		AvailBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_avail_bytes",
				Help:      "OSD Available Storage in Bytes",
			},
			[]string{"osd"},
		),

		Utilization: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_utilization",
				Help:      "OSD Utilization",
			},
			[]string{"osd"},
		),

		Variance: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_variance",
				Help:      "OSD Variance",
			},
			[]string{"osd"},
		),

		Pgs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_pgs",
				Help:      "OSD Placement Group Count",
			},
			[]string{"osd"},
		),

		TotalBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_total_bytes",
				Help:      "OSD Total Storage Bytes",
			},
		),
		TotalUsedBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_total_used_bytes",
				Help:      "OSD Total Used Storage Bytes",
			},
		),

		TotalAvailBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_total_avail_bytes",
				Help:      "OSD Total Available Storage Bytes ",
			},
		),

		AverageUtil: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_average_utilization",
				Help:      "OSD Average Utilization",
			},
		),

		CommitLatency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_perf_commit_latency_seconds",
				Help:      "OSD Perf Commit Latency",
			},
			[]string{"osd"},
		),

		ApplyLatency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_perf_apply_latency_seconds",
				Help:      "OSD Perf Apply Latency",
			},
			[]string{"osd"},
		),

		OSDIn: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_in",
				Help:      "OSD In Status",
			},
			[]string{"osd"},
		),

		OSDUp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cephNamespace,
				Name:      "osd_up",
				Help:      "OSD Up Status",
			},
			[]string{"osd"},
		),
	}
}

func (o *OSDCollector) collectorList() []prometheus.Collector {
	return []prometheus.Collector{
		o.CrushWeight,
		o.Depth,
		o.Reweight,
		o.Bytes,
		o.UsedBytes,
		o.AvailBytes,
		o.Utilization,
		o.Variance,
		o.Pgs,
		o.TotalBytes,
		o.TotalUsedBytes,
		o.TotalAvailBytes,
		o.AverageUtil,
		o.CommitLatency,
		o.ApplyLatency,
		o.OSDIn,
		o.OSDUp,
	}
}

func (o *OSDCollector) collect() error {
	osdDF, err := cephclient.GetOSDUsage(o.context, o.clusterName)
	if err != nil {
		return err
	}

	for _, node := range osdDF.OSDNodes {

		crushWeight, err := node.CrushWeight.Float64()
		if err != nil {
			return err
		}

		o.CrushWeight.WithLabelValues(node.Name).Set(crushWeight)

		depth, err := node.Depth.Float64()
		if err != nil {

			return err
		}

		o.Depth.WithLabelValues(node.Name).Set(depth)

		reweight, err := node.Reweight.Float64()
		if err != nil {
			return err
		}

		o.Reweight.WithLabelValues(node.Name).Set(reweight)

		osdKB, err := node.KB.Float64()
		if err != nil {
			return nil
		}

		o.Bytes.WithLabelValues(node.Name).Set(osdKB * 1e3)

		usedKB, err := node.UsedKB.Float64()
		if err != nil {
			return err
		}

		o.UsedBytes.WithLabelValues(node.Name).Set(usedKB * 1e3)

		availKB, err := node.AvailKB.Float64()
		if err != nil {
			return err
		}

		o.AvailBytes.WithLabelValues(node.Name).Set(availKB * 1e3)

		util, err := node.Utilization.Float64()
		if err != nil {
			return err
		}

		o.Utilization.WithLabelValues(node.Name).Set(util)

		variance, err := node.Variance.Float64()
		if err != nil {
			return err
		}

		o.Variance.WithLabelValues(node.Name).Set(variance)

		pgs, err := node.Pgs.Float64()
		if err != nil {
			continue
		}

		o.Pgs.WithLabelValues(node.Name).Set(pgs)

	}

	totalKB, err := osdDF.Summary.TotalKB.Float64()
	if err != nil {
		return err
	}

	o.TotalBytes.Set(totalKB * 1e3)

	totalUsedKB, err := osdDF.Summary.TotalUsedKB.Float64()
	if err != nil {
		return err
	}

	o.TotalUsedBytes.Set(totalUsedKB * 1e3)

	totalAvailKB, err := osdDF.Summary.TotalAvailKB.Float64()
	if err != nil {
		return err
	}

	o.TotalAvailBytes.Set(totalAvailKB * 1e3)

	averageUtil, err := osdDF.Summary.AverageUtil.Float64()
	if err != nil {
		return err
	}

	o.AverageUtil.Set(averageUtil)

	return nil

}

func (o *OSDCollector) collectOSDPerf() error {
	osdPerf, err := cephclient.GetOSDPerfStats(o.context, o.clusterName)
	if err != nil {
		return err
	}

	for _, perfStat := range osdPerf.PerfInfo {
		osdID, err := perfStat.ID.Int64()
		if err != nil {
			return err
		}
		osdName := fmt.Sprintf("osd.%v", osdID)

		commitLatency, err := perfStat.Stats.CommitLatency.Float64()
		if err != nil {
			return err
		}
		o.CommitLatency.WithLabelValues(osdName).Set(commitLatency / 1e3)

		applyLatency, err := perfStat.Stats.ApplyLatency.Float64()
		if err != nil {
			return err
		}
		o.ApplyLatency.WithLabelValues(osdName).Set(applyLatency / 1e3)
	}

	return nil
}

func (o *OSDCollector) collectOSDDump() error {
	osdDump, err := cephclient.GetOSDDump(o.context, o.clusterName)
	if err != nil {
		return err
	}

	for _, dumpInfo := range osdDump.OSDs {
		osdID, err := dumpInfo.OSD.Int64()
		if err != nil {
			return err
		}
		osdName := fmt.Sprintf("osd.%v", osdID)

		in, err := dumpInfo.In.Float64()
		if err != nil {
			return err
		}

		o.OSDIn.WithLabelValues(osdName).Set(in)

		up, err := dumpInfo.Up.Float64()
		if err != nil {
			return err
		}

		o.OSDUp.WithLabelValues(osdName).Set(up)
	}

	return nil

}

// Describe sends the descriptors of each OSDCollector related metrics we have defined
// to the provided prometheus channel.
func (o *OSDCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range o.collectorList() {
		metric.Describe(ch)
	}

}

// Collect sends all the collected metrics to the provided prometheus channel.
// It requires the caller to handle synchronization.
func (o *OSDCollector) Collect(ch chan<- prometheus.Metric) {

	if err := o.collectOSDPerf(); err != nil {
		logger.Errorf("failed collecting osd perf stats: %+v", err)
	}

	if err := o.collectOSDDump(); err != nil {
		logger.Errorf("failed collecting osd dump: %+v", err)
	}

	if err := o.collect(); err != nil {
		logger.Errorf("failed collecting osd metrics: %+v", err)
	}

	for _, metric := range o.collectorList() {
		metric.Collect(ch)
	}

}
