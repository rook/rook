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
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	upStatus  = 1
	inStatus  = 1
	graceTime = 60 * time.Minute
)

var (
	healthCheckInterval = 300 * time.Second
)

// Monitor defines OSD process monitoring
type Monitor struct {
	context                        *clusterd.Context
	clusterName                    string
	removeOSDsIfOUTAndSafeToRemove bool
	cephVersion                    cephver.CephVersion
}

// NewMonitor instantiates OSD monitoring
func NewMonitor(context *clusterd.Context, clusterName string, removeOSDsIfOUTAndSafeToRemove bool, cephVersion cephver.CephVersion) *Monitor {
	return &Monitor{context, clusterName, removeOSDsIfOUTAndSafeToRemove, cephVersion}
}

// Start runs monitoring logic for osds status at set intervals
func (m *Monitor) Start(stopCh chan struct{}) {

	for {
		select {
		case <-time.After(healthCheckInterval):
			logger.Debug("Checking osd processes status.")
			err := m.osdStatus()
			if err != nil {
				logger.Warningf("failed OSD status check. %v", err)
			}

		case <-stopCh:
			logger.Infof("Stopping monitoring of OSDs in namespace %s", m.clusterName)
			return
		}
	}
}

// Update updates the removeOSDsIfOUTAndSafeToRemove
func (m *Monitor) Update(removeOSDsIfOUTAndSafeToRemove bool) {
	m.removeOSDsIfOUTAndSafeToRemove = removeOSDsIfOUTAndSafeToRemove
}

// OSDStatus validates osd dump output
func (m *Monitor) osdStatus() error {
	osdDump, err := client.GetOSDDump(m.context, m.clusterName)
	if err != nil {
		return err
	}
	logger.Debugf("osd dump %v", osdDump)

	for _, osdStatus := range osdDump.OSDs {
		id64, err := osdStatus.OSD.Int64()
		if err != nil {
			continue
		}
		id := int(id64)

		logger.Debugf("validating status of osd.%d", id)

		status, in, err := osdDump.StatusByID(int64(id))
		if err != nil {
			return err
		}

		if status != upStatus {
			logger.Debugf("osd.%d is marked 'DOWN'", id)
		} else {
			logger.Debugf("osd.%d is healthy.", id)

		}

		if in != inStatus {
			logger.Debugf("osd.%d is marked 'OUT'", id)
			if m.removeOSDsIfOUTAndSafeToRemove {
				if err := m.handleOSDMarkedOut(id); err != nil {
					logger.Errorf("error handling marked out osd osd.%d. %v", id, err)
				}
			}
		}
	}

	return nil
}

func (m *Monitor) handleOSDMarkedOut(outOSDid int) error {
	label := fmt.Sprintf("ceph-osd-id=%d", outOSDid)
	dp, err := k8sutil.GetDeployments(m.context.Clientset, m.clusterName, label)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get osd deployment of osd id %d", outOSDid)
	}
	if len(dp.Items) != 0 {
		safeToDestroyOSD, err := client.OsdSafeToDestroy(m.context, m.clusterName, outOSDid, m.cephVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to get osd deployment of osd id %d", outOSDid)
		}

		if safeToDestroyOSD {
			podCreationTimestamp := dp.Items[0].GetCreationTimestamp()
			podDeletionTimeStamp := podCreationTimestamp.Add(graceTime)
			currentTime := time.Now().UTC()
			if podDeletionTimeStamp.Before(currentTime) {
				logger.Infof("osd.%d is 'safe-to-destroy'. removing the osd deployment.", outOSDid)
				if err := k8sutil.DeleteDeployment(m.context.Clientset, dp.Items[0].Namespace, dp.Items[0].Name); err != nil {
					return errors.Wrapf(err, "failed to delete osd deployment %s", dp.Items[0].Name)
				}
			}
		}
	}
	return nil
}
