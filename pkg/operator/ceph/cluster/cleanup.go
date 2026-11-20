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
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	clusterCleanUpPolicyRetryInterval = 5 // seconds
	// CleanupAppName is the cluster clean up job name
	CleanupAppName = "rook-ceph-cleanup"
	// cleanupSecretName holds the mon secret for the cleanup Jobs. They cannot read it from the
	// rook-ceph-mon Secret, which is garbage collected with the CephCluster before they start.
	cleanupSecretName = CleanupAppName
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

func (c *ClusterController) startClusterCleanUp(context context.Context, cluster *cephv1.CephCluster, cephHosts []string, monSecret, clusterFSID string) {
	log.NamespacedInfo(cluster.Namespace, logger, "starting clean up for cluster %q", cluster.Name)
	err := c.waitForCephDaemonCleanUp(context, cluster, time.Duration(clusterCleanUpPolicyRetryInterval)*time.Second)
	if err != nil {
		log.NamespacedError(cluster.Namespace, logger, "failed to wait till ceph daemons are destroyed. %v", err)
		return
	}

	c.startCleanUpJobs(cluster, cephHosts, monSecret, clusterFSID)
}

func (c *ClusterController) startCleanUpJobs(cluster *cephv1.CephCluster, cephHosts []string, monSecret, clusterFSID string) {
	if len(cephHosts) == 0 {
		return
	}
	// The Jobs' pods cannot start until the Secret they reference exists.
	secret, err := c.saveCleanupSecret(cluster, monSecret)
	if err != nil {
		log.NamespacedError(cluster.Namespace, logger, "failed to save the mon secret for the clean up jobs. skip cluster cleanup. %v", err)
		return
	}

	createdJobs := []string{}
	for _, hostName := range cephHosts {
		log.NamespacedInfo(cluster.Namespace, logger, "starting clean up job on node %q", hostName)
		jobName, err := c.runCleanUpJob(cluster, hostName, clusterFSID)
		if err != nil {
			log.NamespacedError(cluster.Namespace, logger, "failed to run cluster clean up job on node %q. %v", hostName, err)
			continue
		}
		createdJobs = append(createdJobs, jobName)
	}

	if err := c.setCleanupSecretOwners(secret, createdJobs); err != nil {
		log.NamespacedWarning(cluster.Namespace, logger, "failed to make the clean up jobs own secret %q, so it will not be deleted with them. %v", cleanupSecretName, err)
	}
}

// runCleanUpJob starts the cleanup Job for one host, replacing any left by an earlier run, and
// returns its name.
func (c *ClusterController) runCleanUpJob(cluster *cephv1.CephCluster, hostName, clusterFSID string) (string, error) {
	jobName := k8sutil.TruncateNodeNameForJob("cluster-cleanup-job-%s", hostName)
	podSpec := c.cleanUpJobTemplateSpec(cluster, clusterFSID)
	podSpec.Spec.NodeSelector = map[string]string{k8sutil.LabelHostname(): hostName}
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels:    cleanupLabels(cluster.Namespace),
		},
		Spec: batch.JobSpec{
			Template: podSpec,
		},
	}

	// Apply annotations
	cephv1.GetCleanupAnnotations(cluster.Spec.Annotations).ApplyToObjectMeta(&job.ObjectMeta)
	cephv1.GetCleanupLabels(cluster.Spec.Labels).ApplyToObjectMeta(&job.ObjectMeta)

	return jobName, k8sutil.RunReplaceableJob(c.OpManagerCtx, c.context.Clientset, job, true)
}

func cleanupLabels(namespace string) map[string]string {
	labels := opcontroller.AppLabels(CleanupAppName, namespace)
	labels[CleanupAppName] = "true"
	return labels
}

// saveCleanupSecret stores the mon secret for the cleanup Jobs to read. It is written without
// owners, dropping any from an earlier run: RunReplaceableJob deletes that run's Jobs before
// replacing them, and the garbage collector would delete the Secret along with them.
func (c *ClusterController) saveCleanupSecret(cluster *cephv1.CephCluster, monSecret string) (*v1.Secret, error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cleanupSecretName,
			Namespace: cluster.Namespace,
			// Not the CephCluster's labels or annotations, which could opt the key into Secret replication.
			Labels: cleanupLabels(cluster.Namespace),
		},
		Data: map[string][]byte{opcontroller.MonSecretNameKey: []byte(monSecret)},
		Type: k8sutil.RookType,
	}
	saved, err := k8sutil.CreateOrUpdateSecret(c.OpManagerCtx, c.context.Clientset, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to save secret %q", cleanupSecretName)
	}
	return saved, nil
}

// setCleanupSecretOwners makes the given cleanup Jobs own the Secret, so that the garbage collector
// deletes it with the last of them. If any of them cannot be read back, the Secret is left unowned
// rather than owned by the others, whose deletion would take it from that Job.
//
// The update carries the resourceVersion this run saved. If a later run has rewritten the Secret
// since, it is about to replace this run's Jobs, so the Secret is left for that run to own.
func (c *ClusterController) setCleanupSecretOwners(saved *v1.Secret, jobNames []string) error {
	if len(jobNames) == 0 {
		return nil
	}
	namespace := saved.Namespace
	owners := make([]metav1.OwnerReference, 0, len(jobNames))
	for _, name := range jobNames {
		job, err := c.context.Clientset.BatchV1().Jobs(namespace).Get(c.OpManagerCtx, name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to get clean up job %q", name)
		}
		// Neither controller nor blocking the Job's deletion, which would need permission on
		// jobs/finalizers.
		owners = append(owners, metav1.OwnerReference{
			APIVersion: batch.SchemeGroupVersion.String(),
			Kind:       "Job",
			Name:       job.Name,
			UID:        job.UID,
		})
	}

	secret := saved.DeepCopy()
	secret.OwnerReferences = owners
	if _, err := c.context.Clientset.CoreV1().Secrets(namespace).Update(c.OpManagerCtx, secret, metav1.UpdateOptions{}); err != nil {
		if kerrors.IsConflict(err) || kerrors.IsNotFound(err) {
			log.NamespacedInfo(namespace, logger, "secret %q was rewritten or removed after this clean up run saved it, leaving its owners to a later run", cleanupSecretName)
			return nil
		}
		return errors.Wrapf(err, "failed to update the owners of secret %q", cleanupSecretName)
	}
	return nil
}

func (c *ClusterController) cleanUpJobContainer(cluster *cephv1.CephCluster, cephFSID string) v1.Container {
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
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: "run-udev", MountPath: "/run/udev"})
		envVars = append(envVars, []v1.EnvVar{
			{Name: dataDirHostPath, Value: cluster.Spec.DataDirHostPath},
			{Name: namespaceDir, Value: cluster.Namespace},
			{Name: monitorSecret, ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: cleanupSecretName},
				Key:                  opcontroller.MonSecretNameKey,
			}}},
			{Name: clusterFSID, Value: cephFSID},
			{Name: "ROOK_LOG_LEVEL", Value: "DEBUG"},
			mon.PodNamespaceEnvVar(cluster.Namespace),
			{Name: sanitizeMethod, Value: cluster.Spec.CleanupPolicy.SanitizeDisks.Method.String()},
			{Name: sanitizeDataSource, Value: cluster.Spec.CleanupPolicy.SanitizeDisks.DataSource.String()},
			{Name: sanitizeIteration, Value: strconv.Itoa(int(cluster.Spec.CleanupPolicy.SanitizeDisks.Iteration))},
			{Name: "DM_DISABLE_UDEV", Value: "1"},
		}...)
		if opcontroller.LoopDevicesAllowed() {
			envVars = append(envVars, v1.EnvVar{Name: "CEPH_VOLUME_ALLOW_LOOP_DEVICES", Value: "true"})
		}
	}

	// Run as UID 0 since ceph-volume does not support running non-root
	// See https://tracker.ceph.com/issues/53511
	// Also, it's hard to catch the ceph version since the cluster is being deleted so not
	// implementing a version check and simply always run this as root
	securityContext := opcontroller.PrivilegedContext(true)

	return v1.Container{
		Name:            "host-cleanup",
		Image:           c.rookImage,
		SecurityContext: securityContext,
		VolumeMounts:    volumeMounts,
		Env:             envVars,
		Args:            []string{"ceph", "clean", "host"},
		Resources:       cephv1.GetCleanupResources(cluster.Spec.Resources),
	}
}

func (c *ClusterController) cleanUpJobTemplateSpec(cluster *cephv1.CephCluster, clusterFSID string) v1.PodTemplateSpec {
	volumes := []v1.Volume{}
	hostPathVolume := v1.Volume{Name: volumeName, VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: cluster.Spec.DataDirHostPath}}}
	devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
	volumes = append(volumes, hostPathVolume)
	volumes = append(volumes, devVolume)
	volumes = append(volumes, v1.Volume{Name: "run-udev", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}})

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: CleanupAppName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				c.cleanUpJobContainer(cluster, clusterFSID),
			},
			Volumes:            volumes,
			RestartPolicy:      v1.RestartPolicyOnFailure,
			PriorityClassName:  cephv1.GetCleanupPriorityClassName(cluster.Spec.PriorityClassNames),
			SecurityContext:    &v1.PodSecurityContext{},
			ServiceAccountName: k8sutil.DefaultServiceAccount,
			HostNetwork:        cluster.Spec.Network.IsHost(),
		},
	}
	if cluster.Spec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	cephv1.GetCleanupAnnotations(cluster.Spec.Annotations).ApplyToObjectMeta(&podSpec.ObjectMeta)
	cephv1.GetCleanupLabels(cluster.Spec.Labels).ApplyToObjectMeta(&podSpec.ObjectMeta)

	// Apply placement
	getCleanupPlacement(cluster.Spec).ApplyToPodSpec(&podSpec.Spec)

	return podSpec
}

// getCleanupPlacement returns the placement for the cleanup job
func getCleanupPlacement(c cephv1.ClusterSpec) cephv1.Placement {
	// The cleanup jobs are assigned by the operator to a specific node, so the
	// node affinity and other affinity are not needed for scheduling.
	// The only placement required for the cleanup daemons is the tolerations.
	tolerations := c.Placement[cephv1.KeyAll].Tolerations
	tolerations = append(tolerations, c.Placement[cephv1.KeyCleanup].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMonArbiter].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMon].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyMgr].Tolerations...)
	tolerations = append(tolerations, c.Placement[cephv1.KeyOSD].Tolerations...)

	// Add the tolerations for all the device sets
	for _, deviceSet := range c.Storage.StorageClassDeviceSets {
		tolerations = append(tolerations, deviceSet.Placement.Tolerations...)
	}
	return cephv1.Placement{Tolerations: tolerations}
}

func (c *ClusterController) waitForCephDaemonCleanUp(context context.Context, cluster *cephv1.CephCluster, retryInterval time.Duration) error {
	log.NamespacedInfo(cluster.Namespace, logger, "waiting for all the ceph daemons to be cleaned up in the cluster %q", cluster.Namespace)
	for {
		select {
		case <-time.After(retryInterval):
			cephHosts, err := c.getCephHosts(cluster.Namespace)
			if err != nil {
				return errors.Wrap(err, "failed to list ceph daemon nodes")
			}

			if len(cephHosts) == 0 {
				log.NamespacedInfo(cluster.Namespace, logger, "all ceph daemons are cleaned up")
				return nil
			}

			log.NamespacedDebug(cluster.Namespace, logger, "waiting for ceph daemons in cluster %q to be cleaned up. Retrying in %q",
				cluster.Namespace, retryInterval.String())
		case <-context.Done():
			return errors.Errorf("cancelling the host cleanup job. %s", context.Err())
		}
	}
}

// getCephHosts returns a list of host names where ceph daemon pods are running
func (c *ClusterController) getCephHosts(namespace string) ([]string, error) {
	cephAppNames := []string{mon.AppName, mgr.AppName, osd.AppName, object.AppName, mds.AppName, rbd.AppName, mirror.AppName}
	nodeNameList := sets.New[string]()
	hostNameList := []string{}
	var b strings.Builder

	// get all the node names where ceph daemons are running
	for _, app := range cephAppNames {
		appLabelSelector := fmt.Sprintf("app=%s", app)
		podList, err := c.context.Clientset.CoreV1().Pods(namespace).List(c.OpManagerCtx, metav1.ListOptions{LabelSelector: appLabelSelector})
		if err != nil {
			return hostNameList, errors.Wrapf(err, "could not list the %q pods", app)
		}
		for _, cephPod := range podList.Items {
			podNodeName := cephPod.Spec.NodeName
			if podNodeName != "" && !nodeNameList.Has(podNodeName) {
				nodeNameList.Insert(podNodeName)
			}
		}
		fmt.Fprintf(&b, "%s: %d. ", app, len(podList.Items))
	}

	log.NamespacedInfo(namespace, logger, "existing ceph daemons in the namespace %q. %s", namespace, b.String())

	for nodeName := range nodeNameList {
		podHostName, err := k8sutil.GetNodeHostName(c.OpManagerCtx, c.context.Clientset, nodeName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get hostname from node %q", nodeName)
		}
		hostNameList = append(hostNameList, podHostName)
	}

	return hostNameList, nil
}

func (c *ClusterController) getCleanUpDetails(cephClusterSpec *cephv1.ClusterSpec, namespace string) (string, string, error) {
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(c.context, c.OpManagerCtx, namespace, cephClusterSpec)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get cluster info")
	}

	return clusterInfo.MonitorSecret, clusterInfo.FSID, nil
}
