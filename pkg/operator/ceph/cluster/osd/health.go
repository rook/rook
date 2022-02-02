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
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

const (
	upStatus  = 1
	inStatus  = 1
	graceTime = 60 * time.Minute
)

var (
	defaultHealthCheckInterval = 60 * time.Second
)

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
		logger.Infof("ceph osd status in namespace %q check interval %q", h.clusterInfo.Namespace, checkInterval.Duration.String())
		h.interval = &checkInterval.Duration
	}

	return h
}

// Start runs monitoring logic for osds status at set intervals
func (m *OSDHealthMonitor) Start(context context.Context) {

	for {
		select {
		case <-time.After(*m.interval):
			logger.Debug("checking osd processes status.")
			m.checkOSDHealth()

		case <-context.Done():
			logger.Infof("stopping monitoring of OSDs in namespace %q", m.clusterInfo.Namespace)
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
		logger.Debugf("failed to check OSD Dump. %v", err)
	}
	err = m.checkDeviceClasses()
	if err != nil {
		logger.Debugf("failed to check device classes. %v", err)
	}
}

func (m *OSDHealthMonitor) checkDeviceClasses() error {
	devices, err := client.GetDeviceClasses(m.context, m.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd device classes")
	}

	if len(devices) > 0 {
		m.updateCephStatus(devices)
	}

	return nil
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

		logger.Debugf("validating status of osd.%d", id)

		status, in, err := osdDump.StatusByID(int64(id))
		if err != nil {
			return err
		}

		if status == upStatus {
			logger.Debugf("osd.%d is healthy.", id)
			continue
		}

		logger.Debugf("osd.%d is marked 'DOWN'", id)

		if in != inStatus {
			logger.Debugf("osd.%d is marked 'OUT'", id)
			if m.removeOSDsIfOUTAndSafeToRemove {
				if err := m.removeOSDDeploymentIfSafeToDestroy(id); err != nil {
					logger.Errorf("error handling marked out osd osd.%d. %v", id, err)
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
				logger.Infof("osd.%d is 'safe-to-destroy'. removing the osd deployment.", outOSDid)
				if err := k8sutil.DeleteDeployment(m.clusterInfo.Context, m.context.Clientset, dp.Items[0].Namespace, dp.Items[0].Name); err != nil {
					return errors.Wrapf(err, "failed to delete osd deployment %s", dp.Items[0].Name)
				}
			}
		}
	}
	return nil
}

// updateCephStorage updates the CR with deviceclass details
func (m *OSDHealthMonitor) updateCephStatus(devices []string) {
	cephCluster := cephv1.CephCluster{}
	cephClusterStorage := cephv1.CephStorage{}

	for _, device := range devices {
		cephClusterStorage.DeviceClasses = append(cephClusterStorage.DeviceClasses, cephv1.DeviceClasses{Name: device})
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := m.context.Client.Get(m.clusterInfo.Context, m.clusterInfo.NamespacedName(), &cephCluster)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
				return nil
			}
			logger.Errorf("failed to retrieve ceph cluster %q to update ceph Storage. %v", m.clusterInfo.NamespacedName().Name, err)
			return err
		}
		if !reflect.DeepEqual(cephCluster.Status.CephStorage, cephClusterStorage) {
			cephCluster.Status.CephStorage = &cephClusterStorage
			return m.context.Client.Status().Update(m.clusterInfo.Context, &cephCluster)
		}

		return nil
	})
	if err != nil {
		logger.Errorf("failed to update status for ceph cluster %q. %v", m.clusterInfo.NamespacedName().Name, err)
	}
}
