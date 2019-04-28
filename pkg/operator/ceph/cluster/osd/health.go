/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package osd

import (
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

const upStatus = 1

var (
	healthCheckInterval = 60 * time.Second
	osdGracePeriod      = 600 * time.Second
)

// Monitor defines OSD process monitoring
type Monitor struct {
	context     *clusterd.Context
	clusterName string

	// lastStatus keeps track of OSDs status
	// key - OSD id; value: time of the status change.
	lastStatus map[int]time.Time
}

// newMonitor instantiates OSD monitoring
func NewMonitor(context *clusterd.Context, clusterName string) *Monitor {
	return &Monitor{context, clusterName, make(map[int]time.Time)}
}

// Run runs monitoring logic for osds status at set intervals
func (m *Monitor) Start(stopCh chan struct{}) {

	for {
		select {
		case <-time.After(healthCheckInterval):
			logger.Debug("Checking osd processes status.")
			err := m.osdStatus()
			if err != nil {
				logger.Warningf("Failed OSD status check: %+v", err)
			}

		case <-stopCh:
			logger.Infof("Stopping monitoring of OSDs in namespace %s", m.clusterName)
			return
		}
	}
}

// OSDStatus validates osd dump output
func (m *Monitor) osdStatus() error {
	logger.Debugf("OSDs with previously detected Down status: %+v", m.lastStatus)
	osdDump, err := client.GetOSDDump(m.context, m.clusterName)
	if err != nil {
		return err
	}
	logger.Debugf("osd dump %v", osdDump)

	evalDownStatus := func(id int) {
		if now := time.Now(); now.Sub(m.lastStatus[id]) > osdGracePeriod {
			logger.Warningf("osd.%d has been down for longer than the grace period (down since %+v)", id, m.lastStatus[id])
			m.lastStatus[id] = time.Now()
		} else {
			logger.Warningf("waiting for the osd.%d to exceed the grace period", id)
		}
	}

	for _, osdStatus := range osdDump.OSDs {
		id64, err := osdStatus.OSD.Int64()
		if err != nil {
			continue
		}
		id := int(id64)

		logger.Debugf("validating status of osd.%d", id)
		_, tracked := m.lastStatus[id]

		// No action on in/out cluster state is taken at this time.
		status, _, err := osdDump.StatusByID(int64(id))
		if err != nil {
			return err
		}

		if status != upStatus {
			logger.Infof("osd.%d is marked 'DOWN'", id)
			if tracked {
				evalDownStatus(id)
			} else {
				m.lastStatus[id] = time.Now()
			}
		} else {
			logger.Debugf("osd.%d is healthy.", id)
			if tracked {
				logger.Debugf("osd.%d recovered, stopping tracking.", id)
				delete(m.lastStatus, id)
			}
		}
	}

	return nil
}
