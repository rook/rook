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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	osdPodTerminationTimeout = 5 * time.Minute
	pvcDeletionTimeout       = 5 * time.Minute
)

type pvcDeviceCleanup struct {
	pvcName    string
	pvName     string
	nodeName   string
	devicePath string
	dmName     string
	encrypted  bool
}

// RemoveOSDs purges a list of OSDs from the cluster
func RemoveOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, osdsToRemove []string, preservePVC, forceOSDRemoval bool) error {
	// Generate the ceph config for running ceph commands similar to the operator
	if err := client.WriteCephConfig(context, clusterInfo); err != nil {
		return errors.Wrap(err, "failed to write the ceph config")
	}

	osdDump, err := client.GetOSDDump(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd dump")
	}

	var errs []string
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
			errs = append(errs, fmt.Sprintf("osd.%d: OSD is healthy. It cannot be removed unless it is 'down'", osdID))
			continue
		} else {
			logger.Infof("osd.%d is marked 'DOWN'", osdID)
		}

		removeOSD(context, clusterInfo, osdID, preservePVC, forceOSDRemoval)
	}

	if len(errs) > 0 {
		return errors.Errorf("OSD removal failed for %d OSD(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}
	return nil
}

func removeOSD(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int, preservePVC, forceOSDRemoval bool) {
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

	// Check if we can safely remove the OSD
	// Loop forever until the osd is safe-to-destroy
	for {
		isSafeToDestroy, err := client.OsdSafeToDestroy(clusterdContext, clusterInfo, osdID)
		if err != nil {
			// If we want to force remove the OSD and there was an error let's break outside of
			// the loop and proceed with the OSD removal
			if forceOSDRemoval {
				logger.Errorf("failed to check if osd %d is safe to destroy, but force removal is enabled so proceeding with removal. %v", osdID, err)
				break
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
			if forceOSDRemoval {
				logger.Infof("osd.%d is NOT ok to destroy but force removal is enabled so proceeding with removal", osdID)
				break
			}
			// Else we wait until the OSD can be removed
			logger.Warningf("osd.%d is NOT ok to destroy, retrying in 15s until success", osdID)
			time.Sleep(15 * time.Second)
		}
	}

	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", osdID)
	deployment, err := clusterdContext.Clientset.AppsV1().Deployments(clusterInfo.Namespace).Get(clusterInfo.Context, deploymentName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to fetch deployment %q for osd.%d. %v", deploymentName, osdID, err)
	}

	// Gather cleanup metadata for all PVCs (data, metadata, WAL) before any deletion.
	var pvcName string
	encrypted := false
	if deployment != nil {
		pvcName = deployment.GetLabels()[oposd.OSDOverPVCLabelKey]
	}

	var cleanupDevices []pvcDeviceCleanup
	if pvcName != "" {
		encrypted = deployment.GetLabels()["encrypted"] == "true"
		cleanupDevices = gatherCleanupMetadata(clusterdContext, clusterInfo, osdID, pvcName, encrypted)
		fenceDeployment(clusterdContext, clusterInfo, clusterInfo.Namespace, deploymentName, osdID)
		removeOSDPrepareJob(clusterdContext, clusterInfo, pvcName)
		removePVCs(clusterdContext, clusterInfo, cleanupDevices, preservePVC)
	} else if deployment != nil {
		logger.Infof("did not find a pvc name to remove for osd %q", deploymentName)
	}

	if len(cleanupDevices) > 0 && !preservePVC {
		// Wait for pod and PVCs to actually be deleted
		waitForOSDPodTermination(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, deploymentName)
		// Wait for all PVCs to be deleted
		for _, device := range cleanupDevices {
			waitForPVCDeletion(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, device.pvcName)
		}

		// Create purge job for each device
		for _, device := range cleanupDevices {
			if device.nodeName != "" {
				cfg := purgeNodeJobConfig{
					namespace:   clusterInfo.Namespace,
					osdID:       osdID,
					nodeName:    device.nodeName,
					devicePath:  device.devicePath,
					hostPathDir: filepath.Dir(device.devicePath),
					encrypted:   device.encrypted,
					dmName:      device.dmName,
					rookImage:   deployment.Spec.Template.Spec.Containers[0].Image,
					tolerations: deployment.Spec.Template.Spec.Tolerations,
				}
				if err := createAndRunPurgeNodeJob(clusterInfo.Context, clusterdContext.Clientset, cfg); err != nil {
					logger.Errorf("skipping purge node job for osd.%d on node %q: %v", osdID, device.nodeName, err)
					continue
				}
				waitForPurgeNodeJob(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, osdID)
			}
		}
	}

	if deployment != nil {
		logger.Infof("removing the OSD deployment %q", deploymentName)
		if err := k8sutil.DeleteDeployment(clusterInfo.Context, clusterdContext.Clientset, clusterInfo.Namespace, deploymentName); err != nil {
			logger.Errorf("failed to delete deployment %q for osd.%d; check operator logs for details. %v", deploymentName, osdID, err)
		}
	}

	// purge the osd
	logger.Infof("purging osd.%d", osdID)
	purgeOSDArgs := []string{"osd", "purge", fmt.Sprintf("osd.%d", osdID), "--force", "--yes-i-really-mean-it"}
	if _, err = client.NewCephCommand(clusterdContext, clusterInfo, purgeOSDArgs).Run(); err != nil {
		logger.Errorf("failed to purge osd.%d from ceph; check operator logs for details. %v", osdID, err)
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

// gatherCleanupMetadata collects device information from all PVCs (data, metadata, WAL) for an OSD.
func gatherCleanupMetadata(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int, pvcName string, encrypted bool) []pvcDeviceCleanup {
	var cleanupDevices []pvcDeviceCleanup

	dataPVC, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Get(clusterInfo.Context, pvcName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			logger.Errorf("failed to get PVC %q for osd.%d. %v", pvcName, osdID, err)
		} else {
			logger.Infof("PVC %q for osd.%d not found; continuing", pvcName, osdID)
		}
		return cleanupDevices
	}

	labels := dataPVC.GetLabels()
	deviceSet := labels[oposd.CephDeviceSetLabelKey]
	setIndex := labels[oposd.CephSetIndexLabelKey]

	// List all PVCs (data, metadata, WAL) using the same deviceSet/setIndex labels
	labelSelector := fmt.Sprintf("%s=%s,%s=%s", oposd.CephDeviceSetLabelKey, deviceSet, oposd.CephSetIndexLabelKey, setIndex)
	listOptions := metav1.ListOptions{LabelSelector: labelSelector}
	pvcs, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).List(clusterInfo.Context, listOptions)
	if err != nil {
		logger.Errorf("failed to list PVCs for OSD %q. %v", pvcName, err)
		return cleanupDevices
	}

	// Collect cleanup metadata for each PVC
	for _, pvc := range pvcs.Items {
		pv, err := clusterdContext.Clientset.CoreV1().PersistentVolumes().Get(clusterInfo.Context, pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			logger.Errorf("failed to get PV %q for osd.%d. %v", pvc.Spec.VolumeName, osdID, err)
			continue
		}

		device := pvcDeviceCleanup{
			pvcName:   pvc.Name,
			pvName:    pvc.Spec.VolumeName,
			encrypted: encrypted,
		}

		if pv.Spec.Local != nil {
			device.nodeName = nodeNameFromPV(pv)
			device.devicePath = pv.Spec.Local.Path
			if encrypted {
				dmcryptType := oposd.DmcryptBlockType
				if strings.Contains(pvc.Name, "metadata") {
					dmcryptType = oposd.DmcryptMetadataType
				} else if strings.Contains(pvc.Name, "wal") {
					dmcryptType = oposd.DmcryptWalType
				}
				device.dmName = oposd.EncryptionDMName(pvc.Name, dmcryptType)
			}
		}

		cleanupDevices = append(cleanupDevices, device)
	}

	return cleanupDevices
}

// fenceDeployment sets do-not-reconcile label and scales deployment to 0 so the OSD pod stops before PVC deletion.
func fenceDeployment(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, namespace, deploymentName string, osdID int) {
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, err := clusterdContext.Clientset.AppsV1().Deployments(namespace).Get(clusterInfo.Context, deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		latest.Labels[cephv1.SkipReconcileLabelKey] = "true"
		latest.Spec.Replicas = ptr.To(int32(0))
		_, err = clusterdContext.Clientset.AppsV1().Deployments(namespace).Update(clusterInfo.Context, latest, metav1.UpdateOptions{})
		return err
	}); err != nil {
		logger.Errorf("failed to fence deployment %q for osd.%d. %v", deploymentName, osdID, err)
	}
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
			// Continue with the cleanup even if the job fails to be deleted
			logger.Errorf("failed to delete prepare job for osd %q. %v", prepareJob.GetName(), err)
		}
	}
}

// removePVCs deletes or detaches the PVCs for an OSD.
func removePVCs(clusterdContext *clusterd.Context, clusterInfo *client.ClusterInfo, devices []pvcDeviceCleanup, preservePVC bool) {
	for _, device := range devices {
		if preservePVC {
			// Detach the OSD PVC from Rook by removing the device set ID label
			logger.Infof("detaching OSD PVC %q from Rook", device.pvcName)
			pvc, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Get(clusterInfo.Context, device.pvcName, metav1.GetOptions{})
			if err != nil {
				logger.Errorf("failed to get PVC %q for detachment: %v", device.pvcName, err)
				continue
			}
			labels := pvc.GetLabels()
			delete(labels, oposd.CephDeviceSetPVCIDLabelKey)
			pvc.SetLabels(labels)
			if _, err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Update(clusterInfo.Context, pvc, metav1.UpdateOptions{}); err != nil {
				logger.Errorf("failed to remove label %q from PVC %q: %v", oposd.CephDeviceSetPVCIDLabelKey, device.pvcName, err)
			}
		} else {
			// Delete the OSD PVC
			logger.Infof("removing OSD PVC %q", device.pvcName)
			if err := clusterdContext.Clientset.CoreV1().PersistentVolumeClaims(clusterInfo.Namespace).Delete(clusterInfo.Context, device.pvcName, metav1.DeleteOptions{}); err != nil {
				logger.Errorf("failed to delete PVC %q. %v", device.pvcName, err)
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
	if len(crash) == 0 {
		logger.Info("no ceph crash to silence")
		return
	}

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	matched := false
	for _, c := range crash {
		if c.Entity != osdEntity {
			continue
		}
		matched = true
		if err := client.ArchiveCrash(clusterdContext, clusterInfo, c.ID); err != nil {
			logger.Errorf("failed to archive the crash %q. %v", c.ID, err)
		}
	}
	if !matched {
		logger.Infof("no ceph crash for osd.%d to silence", osdID)
	}
}

func waitForOSDPodTermination(ctx context.Context, clientset kubernetes.Interface, namespace, deploymentName string) {
	deadline := time.Now().Add(osdPodTerminationTimeout)
	for time.Now().Before(deadline) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			logger.Errorf("failed to get deployment %q. %v", deploymentName, err)
			return
		}
		if deployment.Status.Replicas == 0 {
			logger.Infof("OSD pod for deployment %q has terminated", deploymentName)
			return
		}
		logger.Infof("waiting for OSD pod in deployment %q to terminate (current replicas: %d)", deploymentName, deployment.Status.Replicas)
		time.Sleep(5 * time.Second)
	}
	logger.Errorf("timed out waiting for OSD pod in deployment %q to terminate", deploymentName)
}

func waitForPVCDeletion(ctx context.Context, clientset kubernetes.Interface, namespace, pvcName string) {
	deadline := time.Now().Add(pvcDeletionTimeout)
	for time.Now().Before(deadline) {
		_, err := clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Infof("PVC %q has been deleted", pvcName)
				return
			}
			logger.Errorf("failed to check PVC %q deletion status. %v", pvcName, err)
			return
		}
		logger.Infof("waiting for PVC %q to be deleted", pvcName)
		time.Sleep(5 * time.Second)
	}
	logger.Errorf("timed out waiting for PVC %q to be deleted", pvcName)
}

// nodeNameFromPV extracts the kubernetes node name from a local PV's node affinity.
func nodeNameFromPV(pv *v1.PersistentVolume) string {
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return ""
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == v1.LabelHostname && len(expr.Values) > 0 {
				return expr.Values[0]
			}
		}
	}
	return ""
}

// DestroyOSD fetches the OSD to be migrated based on the ID and then destroys that OSD and zaps the backing device
func DestroyOSD(context *clusterd.Context, clusterInfo *client.ClusterInfo, id int, isPVC bool) (*oposd.OSDInfo, error) {
	osdInfo, err := GetOSDInfoById(context, clusterInfo, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get OSD info for OSD.%d", id)
	}

	logger.Infof("destroying osd.%d", osdInfo.ID)
	destroyOSDArgs := []string{"osd", "destroy", fmt.Sprintf("osd.%d", osdInfo.ID), "--yes-i-really-mean-it"}
	_, err = client.NewCephCommand(context, clusterInfo, destroyOSDArgs).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to destroy osd.%d.", osdInfo.ID)
	}
	logger.Infof("successfully destroyed osd.%d", osdInfo.ID)

	// in case of OSD on PVs, fetch the actual device name for the mounted /mnt/<pvc-name>
	if isPVC {
		pvcName := os.Getenv(oposd.PVCNameEnvVarName)

		// remove the dm device
		if osdInfo.Encrypted {
			target := oposd.EncryptionDMName(pvcName, oposd.DmcryptBlockType)
			err = RemoveEncryptedDevice(context, target)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to remove dm device %q", target)
			}
		}
		// fetch the actual device for cleanup
		blockPath := fmt.Sprintf("/mnt/%s", pvcName)
		diskInfo, err := clusterd.PopulateDeviceInfo(blockPath, context.Executor)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get device info for %q", blockPath)
		}
		osdInfo.BlockPath = diskInfo.RealPath
	}

	logger.Infof("zap OSD.%d path %q", osdInfo.ID, osdInfo.BlockPath)
	output, err := context.Executor.ExecuteCommandWithCombinedOutput("stdbuf", "-oL", "ceph-volume", "lvm", "zap", osdInfo.BlockPath, "--destroy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to zap osd.%d path %q. %s.", osdInfo.ID, osdInfo.BlockPath, redactCephVolumeOutput(output))
	}

	logger.Infof("%s\n", redactCephVolumeOutput(output))
	logger.Infof("successfully zapped osd.%d path %q", osdInfo.ID, osdInfo.BlockPath)

	return osdInfo, nil
}
