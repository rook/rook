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
	"fmt"
	"os"
	"strconv"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

// RemoveOSDs purges a list of OSDs from the cluster
func RemoveOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, osdsToRemove []string, preservePVC bool, forceRemovalCallback func(osdID int) (bool, bool)) error {
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
		} else {
			logger.Infof("osd.%d is marked 'DOWN'", osdID)
		}

		removeOSD(context, clusterInfo, osdID, preservePVC, forceRemovalCallback)
	}

	return nil
}

func removeOSD(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int, preservePVC bool, forceRemovalCallback func(osdID int) (bool, bool)) {
	// Get the host where the OSD is found
	hostName, err := client.GetCrushHostName(clusterdContext, clusterInfo, osdID)
	if err != nil {
		logger.Errorf("failed to get the host where osd.%d is running. %v", osdID, err)
	}

	// Mark the OSD as out.
	logger.Infof("marking osd.%d out", osdID)
	args := []string{"osd", "out", fmt.Sprintf("osd.%d", osdID)}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, args).Run()
	if err != nil {
		logger.Errorf("failed to exclude osd.%d out of the crush map. %v", osdID, err)
	}
	forceRemoval, exitIfNotSafe := forceRemovalCallback(osdID)
	// Check we can remove the OSD
	// Loop forever until the osd is safe-to-destroy
	for {
		isSafeToDestroy, err := client.OsdSafeToDestroy(clusterdContext, clusterInfo, osdID)
		if err != nil {
			// If we want to force remove the OSD and there was an error let's break outside of
			// the loop and proceed with the OSD removal

			if forceRemoval {
				logger.Errorf("failed to check if osd %d is safe to destroy, but force removal is enabled so proceeding with removal. %v", osdID, err)
				break
			} else if exitIfNotSafe {
				logger.Error("osd.%d is not safe to destroy")
				return
			} else {
				logger.Errorf("failed to check if osd %d is safe to destroy, retrying in 1m. %v", osdID, err)
				time.Sleep(1 * time.Minute)
				continue
			}
		}

		// If no error and the OSD is safe to destroy, we can proceed with the OSD removal
		if isSafeToDestroy {
			logger.Infof("osd.%d is safe to destroy, proceeding", osdID)
			break
		} else {
			// If we arrive here and forceOSDRemoval is true, we should proceed with the OSD removal
			if forceRemoval {
				logger.Infof("osd.%d is NOT ok to destroy but force removal is enabled so proceeding with removal", osdID)
				break
			} else if exitIfNotSafe {
				logger.Error("osd.%d is not safe to destroy")
				return
			}
			// Else we wait until the OSD can be removed
			logger.Warningf("osd.%d is NOT ok to destroy, retrying in 1m until success", osdID)
			time.Sleep(1 * time.Minute)
		}
	}

	// Remove the OSD deployment
	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", osdID)
	deployment, err := clusterdContext.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Get(clusterInfo.Context, deploymentName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to fetch the deployment %q. %v", deploymentName, err)
	} else {
		logger.Infof("removing the OSD deployment %q", deploymentName)
		if err := k8sutil.DeleteDeployment(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, deploymentName); err != nil {
			if err != nil {
				// Continue purging the OSD even if the deployment fails to be deleted
				logger.Errorf("failed to delete deployment for OSD %d. %v", osdID, err)
			}
		}
		if pvcName, ok := deployment.GetLabels()[oposd.OSDOverPVCLabelKey]; ok {
			removeOSDPrepareJob(clusterdContext, clusterInfo, pvcName)
			removePVCs(clusterdContext, clusterInfo, pvcName, preservePVC)
		} else {
			logger.Infof("did not find a pvc name to remove for osd %q", deploymentName)
		}
	}

	// purge the osd
	logger.Infof("purging osd.%d", osdID)
	purgeOSDArgs := []string{"osd", "purge", fmt.Sprintf("osd.%d", osdID), "--force", "--yes-i-really-mean-it"}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, purgeOSDArgs).Run()
	if err != nil {
		logger.Errorf("failed to purge osd.%d. %v", osdID, err)
	}

	// Attempting to remove the parent host. Errors can be ignored if there are other OSDs on the same host
	logger.Infof("attempting to remove host %q from crush map if not in use", hostName)
	hostArgs := []string{"osd", "crush", "rm", hostName}
	_, err = client.NewCephCommand(clusterdContext, clusterInfo, hostArgs).Run()
	if err != nil {
		logger.Infof("failed to remove CRUSH host %q. %v", hostName, err)
	} else {
		logger.Infof("removed CRUSH host %q", hostName)
	}

	// call archiveCrash to silence crash warning in ceph health if any
	archiveCrash(clusterdContext, clusterInfo, osdID)

	logger.Infof("completed removal of OSD %d", osdID)
}

func removeOSDPrepareJob(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, pvcName string) {
	labelSelector := fmt.Sprintf("%s=%s", oposd.OSDOverPVCLabelKey, pvcName)
	prepareJobList, err := clusterdContext.Clientset.BatchV1().Jobs(clusterInfo.Namespace).List(clusterInfo.Context, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Errorf("failed to list osd prepare jobs with pvc %q. %v ", pvcName, err)
	}
	// Remove osd prepare job
	for _, prepareJob := range prepareJobList.Items {
		logger.Infof("removing the osd prepare job %q", prepareJob.GetName())
		if err := k8sutil.DeleteBatchJob(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, prepareJob.GetName(), false); err != nil {
			if err != nil {
				// Continue with the cleanup even if the job fails to be deleted
				logger.Errorf("failed to delete prepare job for osd %q. %v", prepareJob.GetName(), err)
			}
		}
	}
}

func removePVCs(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, dataPVCName string, preservePVC bool) {
	dataPVC, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Get(clusterInfo.Context, dataPVCName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get pvc for OSD %q. %v", dataPVCName, err)
		return
	}
	labels := dataPVC.GetLabels()
	deviceSet := labels[oposd.CephDeviceSetLabelKey]
	setIndex := labels[oposd.CephSetIndexLabelKey]

	labelSelector := fmt.Sprintf("%s=%s,%s=%s", oposd.CephDeviceSetLabelKey, deviceSet, oposd.CephSetIndexLabelKey, setIndex)
	listOptions := metav1.ListOptions{LabelSelector: labelSelector}
	pvcs, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(clusterInfo.Context, listOptions)
	if err != nil {
		logger.Errorf("failed to get pvcs for OSD %q. %v", dataPVCName, err)
		return
	}

	// Delete each of the data, wal, and db PVCs that belonged to the OSD
	for i, pvc := range pvcs.Items {
		if preservePVC {
			// Detach the OSD PVC from Rook. We will continue OSD deletion even if failed to remove PVC label
			logger.Infof("detach the OSD PVC %q from Rook", pvc.Name)
			delete(labels, oposd.CephDeviceSetPVCIDLabelKey)
			pvc.SetLabels(labels)
			if _, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Update(clusterInfo.Context, &pvcs.Items[i], metav1.UpdateOptions{}); err != nil {
				logger.Errorf("failed to remove label %q from pvc for OSD %q. %v", oposd.CephDeviceSetPVCIDLabelKey, pvc.Name, err)
			}
		} else {
			// Remove the OSD PVC
			logger.Infof("removing the OSD PVC %q", pvc.Name)
			if err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Delete(clusterInfo.Context, pvc.Name, metav1.DeleteOptions{}); err != nil {
				if err != nil {
					// Continue deleting the OSD PVC even if PVC deletion fails
					logger.Errorf("failed to delete pvc %q for OSD. %v", pvc.Name, err)
				}
			}
		}
	}
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

// DestroyOSD fetches the OSD to be replaced based on the ID and then destroys that OSD and zaps the backing device
func DestroyOSD(context *clusterd.Context, clusterInfo *client.ClusterInfo, id int, isPVC, isEncrypted bool) (*oposd.OSDReplaceInfo, error) {
	var block string
	osdInfo, err := GetOSDInfoById(context, clusterInfo, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get OSD info for OSD.%d", id)
	}

	block = osdInfo.BlockPath

	logger.Infof("destroying osd.%d", osdInfo.ID)
	destroyOSDArgs := []string{"osd", "destroy", fmt.Sprintf("osd.%d", osdInfo.ID), "--yes-i-really-mean-it"}
	_, err = client.NewCephCommand(context, clusterInfo, destroyOSDArgs).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to destroy osd.%d.", osdInfo.ID)
	}
	logger.Infof("successfully destroyed osd.%d", osdInfo.ID)

	if isPVC && isEncrypted {
		// remove the dm device
		pvcName := os.Getenv(oposd.PVCNameEnvVarName)
		target := oposd.EncryptionDMName(pvcName, oposd.DmcryptBlockType)
		err = removeEncryptedDevice(context, target)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to remove dm device %q", target)
		}
		// ceph-volume uses `/dev/mapper/*` for encrypted disks. This is not a block device. So we need to fetch the corresponding
		// block device for cleanup using `ceph-volume lvm zap`
		blockPath := fmt.Sprintf("/mnt/%s", pvcName)
		diskInfo, err := clusterd.PopulateDeviceInfo(blockPath, context.Executor)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get device info for %q", blockPath)
		}
		block = diskInfo.RealPath
	}

	logger.Infof("zap OSD.%d path %q", osdInfo.ID, block)
	output, err := context.Executor.ExecuteCommandWithCombinedOutput("stdbuf", "-oL", "ceph-volume", "lvm", "zap", block, "--destroy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to zap osd.%d path %q. %s.", osdInfo.ID, block, output)
	}

	logger.Infof("%s\n", output)
	logger.Infof("successfully zapped osd.%d path %q", osdInfo.ID, block)

	return &oposd.OSDReplaceInfo{ID: osdInfo.ID, Path: block}, nil
}
