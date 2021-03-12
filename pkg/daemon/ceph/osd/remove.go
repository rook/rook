/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"strconv"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

// RemoveOSDs purges a list of OSDs from the cluster
func RemoveOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, osdsToRemove []string) error {

	// Generate the ceph config for running ceph commands similar to the operator
	if err := client.WriteCephConfig(context, clusterInfo); err != nil {
		return errors.Wrap(err, "failed to write the ceph config")
	}

	osdDump, err := client.GetOSDDump(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd dump")
	}

	for _, osdIDStr := range osdsToRemove {
		osdID, err := strconv.Atoi(osdIDStr)
		if err != nil {
			logger.Errorf("invalid OSD ID: %s. %v", osdIDStr, err)
			continue
		}
		logger.Infof("validating status of osd.%d", osdID)
		status, _, err := osdDump.StatusByID(int64(osdID))
		if err != nil {
			return errors.Wrapf(err, "failed to get osd status for osd %d", osdID)
		}
		const upStatus int64 = 1
		if status == upStatus {
			logger.Infof("osd.%d is healthy. It cannot be removed unless it is 'down'", osdID)
			continue
		}
		logger.Infof("osd.%d is marked 'DOWN'. Removing it", osdID)
		removeOSD(context, clusterInfo, osdID)
	}

	return nil
}

func removeOSD(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int) {
	ctx := context.TODO()
	// Get the host where the OSD is found
	hostName, err := client.GetCrushHostName(clusterdContext, clusterInfo, osdID)
	if err != nil {
		logger.Errorf("failed to get the host where osd.%d is running. %v", osdID, err)
	}

	// Mark the OSD as out.
	args := []string{"osd", "out", fmt.Sprintf("osd.%d", osdID)}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, args).Run()
	if err != nil {
		logger.Errorf("failed to exclude osd.%d out of the crush map. %v", osdID, err)
	}

	// Remove the OSD deployment
	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", osdID)
	deployment, err := clusterdContext.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to fetch the deployment %q. %v", deploymentName, err)
	} else {
		logger.Infof("removing the OSD deployment %q", deploymentName)
		if err := k8sutil.DeleteDeployment(clusterdContext.Clientset, clusterInfo.Namespace, deploymentName); err != nil {
			if err != nil {
				// Continue purging the OSD even if the deployment fails to be deleted
				logger.Errorf("failed to delete deployment for OSD %d. %v", osdID, err)
			}
		}
		if pvcName, ok := deployment.GetLabels()[osd.OSDOverPVCLabelKey]; ok {
			labelSelector := fmt.Sprintf("%s=%s", osd.OSDOverPVCLabelKey, pvcName)
			prepareJobList, err := clusterdContext.Clientset.BatchV1().Jobs(clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil && !kerrors.IsNotFound(err) {
				logger.Errorf("failed to list osd prepare jobs with pvc %q. %v ", pvcName, err)
			}
			// Remove osd prepare job
			for _, prepareJob := range prepareJobList.Items {
				logger.Infof("removing the osd prepare job %q", prepareJob.GetName())
				if err := k8sutil.DeleteBatchJob(clusterdContext.Clientset, clusterInfo.Namespace, prepareJob.GetName(), false); err != nil {
					if err != nil {
						// Continue deleting the OSD prepare job even if the deployment fails to be deleted
						logger.Errorf("failed to delete prepare job for osd %q. %v", prepareJob.GetName(), err)
					}
				}
			}
			// Remove the OSD PVC
			logger.Infof("removing the OSD PVC %q", pvcName)
			if err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Delete(ctx, pvcName, metav1.DeleteOptions{}); err != nil {
				if err != nil {
					// Continue deleting the OSD PVC even if PVC deletion fails
					logger.Errorf("failed to delete pvc for OSD %q. %v", pvcName, err)
				}
			}
		} else {
			logger.Infof("did not find a pvc name to remove for osd %q", deploymentName)
		}
	}

	// purge the osd
	purgeosdargs := []string{"osd", "purge", fmt.Sprintf("osd.%d", osdID), "--force", "--yes-i-really-mean-it"}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, purgeosdargs).Run()
	if err != nil {
		logger.Errorf("failed to purge osd.%d. %v", osdID, err)
	}

	// Attempting to remove the parent host. Errors can be ignored if there are other OSDs on the same host
	hostargs := []string{"osd", "crush", "rm", hostName}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, hostargs).Run()
	if err != nil {
		logger.Errorf("failed to remove CRUSH host %q. %v", hostName, err)
	}
	// call archiveCrash to silence crash warning in ceph health if any
	archiveCrash(clusterdContext, clusterInfo, osdID)

	logger.Infof("completed removal of OSD %d", osdID)
}

func archiveCrash(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int) {
	// The ceph health warning should be silenced by archiving the crash
	crash, err := client.GetCrash(clusterdContext, clusterInfo)
	if err != nil {
		logger.Errorf("failed to list ceph crash. %v", err)
		return
	}
	if crash != nil {
		logger.Info("no ceph crash to silence")
		return
	}
	var crashID string
	for _, c := range crash {
		if c.Entity == fmt.Sprintf("osd.%d", osdID) {
			crashID = c.ID
			break
		}
	}
	err = client.ArchiveCrash(clusterdContext, clusterInfo, crashID)
	if err != nil {
		logger.Errorf("failed to archive the crash %q. %v", crashID, err)
	}
}
