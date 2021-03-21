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

package cluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterCleanUpPolicyRetryInterval = 5 //seconds
	// CleanupAppName is the cluster clean up job name
	CleanupAppName = "rook-ceph-cleanup"
)

var (
	volumeName                     = "cleanup-volume"
	dataDirHostPath                = "ROOK_DATA_DIR_HOST_PATH"
	namespaceDir                   = "ROOK_NAMESPACE_DIR"
	monitorSecret                  = "ROOK_MON_SECRET"
	clusterFSID                    = "ROOK_CLUSTER_FSID"
	sanitizeMethod                 = "ROOK_SANITIZE_METHOD"
	sanitizeDataSource             = "ROOK_SANITIZE_DATA_SOURCE"
	sanitizeIteration              = "ROOK_SANITIZE_ITERATION"
	sanitizeIterationDefault int32 = 1
)

func (c *ClusterController) startClusterCleanUp(stopCleanupCh chan struct{}, cluster *cephv1.CephCluster, cephHosts []string, monSecret, clusterFSID string) {
	logger.Infof("starting clean up for cluster %q", cluster.Name)
	err := c.waitForCephDaemonCleanUp(stopCleanupCh, cluster, time.Duration(clusterCleanUpPolicyRetryInterval)*time.Second)
	if err != nil {
		logger.Errorf("failed to wait till ceph daemons are destroyed. %v", err)
		return
	}

	c.startCleanUpJobs(cluster, cephHosts, monSecret, clusterFSID)
}

func (c *ClusterController) startCleanUpJobs(cluster *cephv1.CephCluster, cephHosts []string, monSecret, clusterFSID string) {
	for _, hostName := range cephHosts {
		logger.Infof("starting clean up job on node %q", hostName)
		jobName := k8sutil.TruncateNodeName("cluster-cleanup-job-%s", hostName)
		podSpec := c.cleanUpJobTemplateSpec(cluster, monSecret, clusterFSID)
		podSpec.Spec.NodeSelector = map[string]string{v1.LabelHostname: hostName}
		labels := controller.AppLabels(CleanupAppName, cluster.Namespace)
		labels[CleanupAppName] = "true"
		job := &batch.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: cluster.Namespace,
				Labels:    labels,
			},
			Spec: batch.JobSpec{
				Template: podSpec,
			},
		}

		// Apply annotations
		cephv1.GetCleanupAnnotations(cluster.Spec.Annotations).ApplyToObjectMeta(&job.ObjectMeta)
		cephv1.GetCleanupLabels(cluster.Spec.Labels).ApplyToObjectMeta(&job.ObjectMeta)

		if err := k8sutil.RunReplaceableJob(c.context.Clientset, job, true); err != nil {
			logger.Errorf("failed to run cluster clean up job on node %q. %v", hostName, err)
		}
	}
}

func (c *ClusterController) cleanUpJobContainer(cluster *cephv1.CephCluster, monSecret, cephFSID string) v1.Container {
	volumeMounts := []v1.VolumeMount{}
	envVars := []v1.EnvVar{}
	if cluster.Spec.DataDirHostPath != "" {
		if cluster.Spec.CleanupPolicy.SanitizeDisks.Iteration == 0 {
			cluster.Spec.CleanupPolicy.SanitizeDisks.Iteration = sanitizeIterationDefault
		}

		hostPathVolumeMount := v1.VolumeMount{Name: volumeName, MountPath: cluster.Spec.DataDirHostPath}
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, hostPathVolumeMount)
		volumeMounts = append(volumeMounts, devMount)
		envVars = append(envVars, []v1.EnvVar{
			{Name: dataDirHostPath, Value: cluster.Spec.DataDirHostPath},
			{Name: namespaceDir, Value: cluster.Namespace},
			{Name: monitorSecret, Value: monSecret},
			{Name: clusterFSID, Value: cephFSID},
			{Name: "ROOK_LOG_LEVEL", Value: "DEBUG"},
			mon.PodNamespaceEnvVar(cluster.Namespace),
			{Name: sanitizeMethod, Value: cluster.Spec.CleanupPolicy.SanitizeDisks.Method.String()},
			{Name: sanitizeDataSource, Value: cluster.Spec.CleanupPolicy.SanitizeDisks.DataSource.String()},
			{Name: sanitizeIteration, Value: strconv.Itoa(int(cluster.Spec.CleanupPolicy.SanitizeDisks.Iteration))},
		}...)
	}

	return v1.Container{
		Name:            "host-cleanup",
		Image:           c.rookImage,
		SecurityContext: osd.PrivilegedContext(),
		VolumeMounts:    volumeMounts,
		Env:             envVars,
		Args:            []string{"ceph", "clean"},
		Resources:       cephv1.GetCleanupResources(cluster.Spec.Resources),
	}
}

func (c *ClusterController) cleanUpJobTemplateSpec(cluster *cephv1.CephCluster, monSecret, clusterFSID string) v1.PodTemplateSpec {
	volumes := []v1.Volume{}
	hostPathVolume := v1.Volume{Name: volumeName, VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: cluster.Spec.DataDirHostPath}}}
	devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
	volumes = append(volumes, hostPathVolume)
	volumes = append(volumes, devVolume)

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: CleanupAppName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				c.cleanUpJobContainer(cluster, monSecret, clusterFSID),
			},
			Volumes:           volumes,
			RestartPolicy:     v1.RestartPolicyOnFailure,
			PriorityClassName: cephv1.GetCleanupPriorityClassName(cluster.Spec.PriorityClassNames),
		},
	}

	cephv1.GetCleanupAnnotations(cluster.Spec.Annotations).ApplyToObjectMeta(&podSpec.ObjectMeta)
	cephv1.GetCleanupLabels(cluster.Spec.Labels).ApplyToObjectMeta(&podSpec.ObjectMeta)

	// Apply placement
	getCleanupPlacement(cluster.Spec).ApplyToPodSpec(&podSpec.Spec)

	return podSpec
}

// getCleanupPlacement returns the placement for the cleanup job
func getCleanupPlacement(c cephv1.ClusterSpec) rookv1.Placement {
	// The cleanup jobs are assigned by the operator to a specific node, so the
	// node affinity and other affinity are not needed for scheduling.
	// The only placement required for the cleanup daemons is the tolerations.
	tolerations := c.Placement[rookv1.KeyAll].Tolerations
	tolerations = append(tolerations, c.Placement[cephv1.KeyCleanup].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMonArbiter].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMon].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMgr].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyOSD].Tolerations...)

	// Add the tolerations for all the device sets
	for _, deviceSet := range c.Storage.StorageClassDeviceSets {
		tolerations = append(tolerations, deviceSet.Placement.Tolerations...)
	}
	return rookv1.Placement{Tolerations: tolerations}
}

func (c *ClusterController) waitForCephDaemonCleanUp(stopCleanupCh chan struct{}, cluster *cephv1.CephCluster, retryInterval time.Duration) error {
	logger.Infof("waiting for all the ceph daemons to be cleaned up in the cluster %q", cluster.Namespace)
	for {
		select {
		case <-time.After(retryInterval):
			cephHosts, err := c.getCephHosts(cluster.Namespace)
			if err != nil {
				return errors.Wrap(err, "failed to list ceph daemon nodes")
			}

			if len(cephHosts) == 0 {
				logger.Info("all ceph daemons are cleaned up")
				return nil
			}

			logger.Debugf("waiting for ceph daemons in cluster %q to be cleaned up. Retrying in %q",
				cluster.Namespace, retryInterval.String())
			break
		case <-stopCleanupCh:
			return errors.New("cancelling the host cleanup job")
		}
	}
}

// getCephHosts returns a list of host names where ceph daemon pods are running
func (c *ClusterController) getCephHosts(namespace string) ([]string, error) {
	ctx := context.TODO()
	cephPodCount := map[string]int{}
	cephAppNames := []string{mon.AppName, mgr.AppName, osd.AppName, object.AppName, mds.AppName, rbd.AppName}
	nodeNameList := util.NewSet()
	hostNameList := []string{}

	// get all the node names where ceph daemons are running
	for _, app := range cephAppNames {
		appLabelSelector := fmt.Sprintf("app=%s", app)
		podList, err := c.context.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: appLabelSelector})
		if err != nil {
			return hostNameList, errors.Wrapf(err, "could not list the %q pods", app)
		}
		for _, cephPod := range podList.Items {
			podNodeName := cephPod.Spec.NodeName
			if podNodeName != "" && !nodeNameList.Contains(podNodeName) {
				nodeNameList.Add(podNodeName)
			}
		}
		cephPodCount[app] = len(podList.Items)
	}
	logger.Infof("existing ceph daemons in the namespace %q: rook-ceph-mon: %d, rook-ceph-osd: %d, rook-ceph-mds: %d, rook-ceph-rgw: %d, rook-ceph-mgr: %d, rook-ceph-rbd-mirror: %d",
		namespace, cephPodCount["rook-ceph-mon"], cephPodCount["rook-ceph-osd"], cephPodCount["rook-ceph-mds"], cephPodCount["rook-ceph-rgw"], cephPodCount["rook-ceph-mgr"], cephPodCount["rook-ceph-rbd-mirror"])

	for nodeName := range nodeNameList.Iter() {
		podHostName, err := k8sutil.GetNodeHostName(c.context.Clientset, nodeName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get hostname from node %q", nodeName)
		}
		hostNameList = append(hostNameList, podHostName)
	}

	return hostNameList, nil
}

func (c *ClusterController) getCleanUpDetails(namespace string) (string, string, error) {
	clusterInfo, _, _, err := mon.LoadClusterInfo(c.context, namespace)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get cluster info")
	}

	return clusterInfo.MonitorSecret, clusterInfo.FSID, nil
}
