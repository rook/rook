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
	"net/http"

	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/model"
)

// Gets the status details of this cluster.
// GET
// /status
func (h *Handler) GetStatusDetails(w http.ResponseWriter, r *http.Request) {
	cephStatus, err := ceph.Status(h.context, h.config.ClusterInfo.Name)
	if err != nil {
		logger.Errorf("failed to get status: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	summaries := make([]model.StatusSummary, len(cephStatus.Health.Summary))
	for i, s := range cephStatus.Health.Summary {
		summaries[i] = model.StatusSummary{
			Status:  ceph.HealthToModelHealthStatus(s.Severity),
			Message: s.Summary,
		}
	}

	// generate the monitor health summaries
	monitors := make([]model.MonitorSummary, len(cephStatus.MonMap.Mons))
	for i, m := range cephStatus.MonMap.Mons {
		monitors[i] = model.MonitorSummary{
			Name:    m.Name,
			Address: m.Address,
		}

		// determine if the mon is in quorum
		inQuorum := false
		for _, qr := range cephStatus.Quorum {
			if m.Rank == qr {
				inQuorum = true
				break
			}
		}
		monitors[i].InQuorum = inQuorum

		// determine the mon's health status
		monHealth := model.HealthUnknown
		for _, mh := range ceph.GetMonitorHealthSummaries(cephStatus) {
			if m.Name == mh.Name {
				monHealth = ceph.HealthToModelHealthStatus(mh.Health)
				break
			}
		}
		monitors[i].Status = monHealth
	}

	// generate the OSD health Summary
	osdMap := cephStatus.OsdMap.OsdMap
	osds := model.OSDSummary{
		Total:    osdMap.NumOsd,
		NumberIn: osdMap.NumInOsd,
		NumberUp: osdMap.NumUpOsd,
		Full:     osdMap.Full,
		NearFull: osdMap.NearFull,
	}

	// generate the Mgr summarhy
	mgrs := model.MgrSummary{
		ActiveName: cephStatus.MgrMap.ActiveName,
		ActiveAddr: cephStatus.MgrMap.ActiveAddr,
		Available:  cephStatus.MgrMap.Available,
	}
	for _, standby := range cephStatus.MgrMap.Standbys {
		mgrs.Standbys = append(mgrs.Standbys, standby.Name)
	}

	// generate the usage Summary
	usageSummary := model.UsageSummary{
		DataBytes:      cephStatus.PgMap.DataBytes,
		UsedBytes:      cephStatus.PgMap.UsedBytes,
		AvailableBytes: cephStatus.PgMap.AvailableBytes,
		TotalBytes:     cephStatus.PgMap.TotalBytes,
	}

	// generate the placement group health Summary
	pgStates := make(map[string]int, len(cephStatus.PgMap.PgsByState))
	for _, pg := range cephStatus.PgMap.PgsByState {
		pgStates[pg.StateName] = pg.Count
	}
	pgSummary := model.PGSummary{Total: cephStatus.PgMap.NumPgs, StateCounts: pgStates}

	statusDetails := model.StatusDetails{
		OverallStatus:   ceph.HealthToModelHealthStatus(cephStatus.Health.OverallStatus),
		SummaryMessages: summaries,
		Monitors:        monitors,
		Mgrs:            mgrs,
		OSDs:            osds,
		PGs:             pgSummary,
		Usage:           usageSummary,
	}

	FormatJsonResponse(w, statusDetails)
}
