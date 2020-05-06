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
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	upStatus  = 1
	inStatus  = 1
	graceTime = 60 * time.Minute
)

var (
	healthCheckInterval = 300 * time.Second
)

// OSDHealthMonitor defines OSD process monitoring
type OSDHealthMonitor struct {
	context                        *clusterd.Context
	namespace                      string
	removeOSDsIfOUTAndSafeToRemove bool
	cephVersion                    cephver.CephVersion
}

// NewOSDHealthMonitor instantiates OSD monitoring
func NewOSDHealthMonitor(context *clusterd.Context, namespace string, removeOSDsIfOUTAndSafeToRemove bool, cephVersion cephver.CephVersion) *OSDHealthMonitor {
	return &OSDHealthMonitor{context, namespace, removeOSDsIfOUTAndSafeToRemove, cephVersion}
}

// Start runs monitoring logic for osds status at set intervals
func (m *OSDHealthMonitor) Start(stopCh chan struct{}) {

	for {
		select {
		case <-time.After(healthCheckInterval):
			logger.Debug("Checking osd processes status.")
			err := m.checkOSDHealth()
			if err != nil {
				logger.Warningf("failed OSD status check. %v", err)
			}

		case <-stopCh:
			logger.Infof("Stopping monitoring of OSDs in namespace %s", m.namespace)
			return
		}
	}
}

// Update updates the removeOSDsIfOUTAndSafeToRemove
func (m *OSDHealthMonitor) Update(removeOSDsIfOUTAndSafeToRemove bool) {
	m.removeOSDsIfOUTAndSafeToRemove = removeOSDsIfOUTAndSafeToRemove
}

// checkOSDHealth takes action when needed if the OSDs are not healthy
func (m *OSDHealthMonitor) checkOSDHealth() error {
	osdDump, err := client.GetOSDDump(m.context, m.namespace)
	if err != nil {
		return err
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

		// check if the down osd is stuck terminating
		if err := m.restartOSDIfStuck(id); err != nil {
			logger.Warningf("failed to restart OSD %d. %v", id, err)
		}

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
	dp, err := k8sutil.GetDeployments(m.context.Clientset, m.namespace, label)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get osd deployment of osd id %d", outOSDid)
	}
	if len(dp.Items) != 0 {
		safeToDestroyOSD, err := client.OsdSafeToDestroy(m.context, m.namespace, outOSDid, m.cephVersion)
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

// restartOSDIfStuck will check if a portable OSD is on a node that is not ready.
// If the pod is stuck in terminating state, go ahead and force delete the pod so K8s
// will free up the volume and allow the OSD to be restarted on another node.
func (m *OSDHealthMonitor) restartOSDIfStuck(osdID int) error {
	labels := fmt.Sprintf("ceph-osd-id=%d,portable=true", osdID)
	pods, err := m.context.Clientset.CoreV1().Pods(m.namespace).List(metav1.ListOptions{LabelSelector: labels})
	if err != nil {
		return errors.Wrapf(err, "failed to get OSD with ID %d", osdID)
	}
	return m.restartOSDPodsIfStuck(osdID, pods)
}

func (m *OSDHealthMonitor) restartOSDPodsIfStuck(osdID int, pods *v1.PodList) error {
	for _, pod := range pods.Items {
		logger.Debugf("checking if osd %d pod is stuck and should be force deleted", osdID)
		if pod.DeletionTimestamp.IsZero() {
			logger.Debugf("skipping restart of OSD %d since the pod is not deleted", osdID)
			continue
		}
		node, err := m.context.Clientset.CoreV1().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			logger.Warningf("skipping restart of OSD %d since the node status is not available. %v", osdID, err)
			continue
		}
		if k8sutil.NodeIsReady(*node) {
			logger.Debugf("skipping restart of OSD %d since the node status is ready", osdID)
			continue
		}

		logger.Infof("force deleting pod %q that appears to be stuck terminating", pod.Name)
		var gracePeriod int64
		deleteOpts := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod}
		if err := m.context.Clientset.CoreV1().Pods(m.namespace).Delete(pod.Name, deleteOpts); err != nil {
			logger.Warningf("pod %q deletion failed. %v", pod.Name, err)
			continue
		}
		logger.Infof("pod %q deletion succeeded", pod.Name)
	}

	return nil
}
