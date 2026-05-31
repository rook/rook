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
	"sync"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	upStatus  = 1
	inStatus  = 1
	graceTime = 60 * time.Minute
)

var defaultHealthCheckInterval = 60 * time.Second

// OSDHealthMonitor defines OSD process monitoring
type OSDHealthMonitor struct {
	context                        *clusterd.Context
	clusterInfo                    *client.ClusterInfo
	removeOSDsIfOUTAndSafeToRemove bool
	interval                       *time.Duration
}

// NewOSDHealthMonitor instantiates OSD monitoring
func NewOSDHealthMonitor(context *clusterd.Context, clusterInfo *client.ClusterInfo, removeOSDsIfOUTAndSafeToRemove bool, healthCheck cephv1.CephClusterHealthCheckSpec) *OSDHealthMonitor {
	h := &OSDHealthMonitor{
		context:                        context,
		clusterInfo:                    clusterInfo,
		removeOSDsIfOUTAndSafeToRemove: removeOSDsIfOUTAndSafeToRemove,
		interval:                       &defaultHealthCheckInterval,
	}

	// allow overriding the check interval
	checkInterval := healthCheck.DaemonHealth.ObjectStorageDaemon.Interval
	if checkInterval != nil {
		log.NamespacedInfo(h.clusterInfo.Namespace, logger, "ceph osd status in namespace %q check interval %q", h.clusterInfo.Namespace, checkInterval.Duration.String())
		h.interval = &checkInterval.Duration
	}

	return h
}

// Start runs monitoring logic for osds status at set intervals
func (m *OSDHealthMonitor) Start(monitoringRoutines *sync.Map, daemon string) {
	for {
		// We must perform this check otherwise the case will check an index that does not exist anymore and
		// we will get an invalid pointer error and the go routine will panic
		v, ok := monitoringRoutines.Load(daemon)
		if !ok {
			log.NamespacedInfo(m.clusterInfo.Namespace, logger, "ceph cluster %q has been deleted. stopping monitoring of OSDs", m.clusterInfo.Namespace)
			return
		}
		health := v.(*opcontroller.ClusterHealth)
		select {
		case <-time.After(*m.interval):
			log.NamespacedDebug(m.clusterInfo.Namespace, logger, "checking osd processes status.")
			m.checkOSDHealth()

		case <-health.InternalCtx.Done():
			log.NamespacedInfo(m.clusterInfo.Namespace, logger, "stopping monitoring of OSDs in namespace %q", m.clusterInfo.Namespace)
			monitoringRoutines.Delete(daemon)
			return
		}
	}
}

// Update updates the removeOSDsIfOUTAndSafeToRemove
func (m *OSDHealthMonitor) Update(removeOSDsIfOUTAndSafeToRemove bool) {
	m.removeOSDsIfOUTAndSafeToRemove = removeOSDsIfOUTAndSafeToRemove
}

// checkOSDHealth takes action when needed if the OSDs are not healthy
func (m *OSDHealthMonitor) checkOSDHealth() {
	err := m.checkOSDDump()
	if err != nil {
		log.NamespacedDebug(m.clusterInfo.Namespace, logger, "failed to check OSD Dump. %v", err)
	}
}

func (m *OSDHealthMonitor) checkOSDDump() error {
	osdDump, err := client.GetOSDDump(m.context, m.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd dump")
	}

	for _, osdStatus := range osdDump.OSDs {
		id64, err := osdStatus.OSD.Int64()
		if err != nil {
			continue
		}
		id := int(id64)

		log.NamespacedDebug(m.clusterInfo.Namespace, logger, "validating status of osd.%d", id)

		status, in, err := osdDump.StatusByID(int64(id))
		if err != nil {
			return err
		}

		if status == upStatus {
			log.NamespacedDebug(m.clusterInfo.Namespace, logger, "osd.%d is healthy.", id)
			continue
		}

		log.NamespacedDebug(m.clusterInfo.Namespace, logger, "osd.%d is marked 'DOWN'", id)

		if in != inStatus {
			log.NamespacedDebug(m.clusterInfo.Namespace, logger, "osd.%d is marked 'OUT'", id)
			if m.removeOSDsIfOUTAndSafeToRemove {
				if err := m.removeOSDDeploymentIfSafeToDestroy(id); err != nil {
					log.NamespacedError(m.clusterInfo.Namespace, logger, "error handling marked out osd osd.%d. %v", id, err)
				}
			}
		}
	}

	return nil
}

func (m *OSDHealthMonitor) removeOSDDeploymentIfSafeToDestroy(outOSDid int) error {
	label := fmt.Sprintf("ceph-osd-id=%d", outOSDid)
	dp, err := k8sutil.GetDeployments(m.clusterInfo.Context, m.context.Clientset, m.clusterInfo.Namespace, label)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get osd deployment of osd id %d", outOSDid)
	}
	if len(dp.Items) != 0 {
		safeToDestroyOSD, err := client.OsdSafeToDestroy(m.context, m.clusterInfo, outOSDid)
		if err != nil {
			return errors.Wrapf(err, "failed to get osd deployment of osd id %d", outOSDid)
		}

		if safeToDestroyOSD {
			podCreationTimestamp := dp.Items[0].GetCreationTimestamp()
			podDeletionTimeStamp := podCreationTimestamp.Add(graceTime)
			currentTime := time.Now().UTC()
			if podDeletionTimeStamp.Before(currentTime) {
				log.NamespacedInfo(m.clusterInfo.Namespace, logger, "osd.%d is 'safe-to-destroy'. removing the osd deployment.", outOSDid)
				if err := k8sutil.DeleteDeployment(m.clusterInfo.Context, m.context.Clientset, dp.Items[0].Namespace, dp.Items[0].Name); err != nil {
					return errors.Wrapf(err, "failed to delete osd deployment %s", dp.Items[0].Name)
				}
			}
		}
	}
	return nil
}
