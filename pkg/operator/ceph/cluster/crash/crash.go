/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package crash

import (
	"context"
	"fmt"
	"path"
	"reflect"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/ceph/version"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	crashCollectorKeyringUsername = "client.crash"
	crashCollectorSecretName      = "rook-ceph-crash-collector-keyring"
)

// ClusterResource operator-kit Custom Resource Definition
var clusterResource = k8sutil.CustomResource{
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephCluster{}).Name(),
}

// createOrUpdateCephCrash is a wrapper around controllerutil.CreateOrUpdate
func (r *ReconcileNode) createOrUpdateCephCrash(node corev1.Node, tolerations []corev1.Toleration, cephCluster cephv1.CephCluster, cephVersion *version.CephVersion) (controllerutil.OperationResult, error) {
	// Create or Update the deployment default/foo
	nodeHostnameLabel, ok := node.ObjectMeta.Labels[corev1.LabelHostname]
	if !ok {
		return controllerutil.OperationResultNone, errors.Errorf("label key %q does not exist on node %q", corev1.LabelHostname, node.GetName())
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", AppName), nodeHostnameLabel),
			Namespace:       cephCluster.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{clusterOwnerRef(cephCluster.GetName(), string(cephCluster.GetUID()))},
		},
	}

	volumes := opspec.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "")
	volumes = append(volumes, getVolumes(*cephVersion))

	mutateFunc := func() error {

		// labels for the pod, the deployment, and the deploymentSelector
		deploymentLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      AppName,
			NodeNameLabel:        node.GetName(),
		}
		deploymentLabels[string(config.CrashType)] = "crash"
		deploymentLabels["ceph_daemon_id"] = "crash"

		selectorLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      AppName,
			NodeNameLabel:        node.GetName(),
		}

		nodeSelector := map[string]string{corev1.LabelHostname: nodeHostnameLabel}

		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
		}

		deploy.ObjectMeta.Labels = deploymentLabels
		k8sutil.AddRookVersionLabelToDeployment(deploy)
		if cephVersion != nil {
			opspec.AddCephVersionLabelToDeployment(*cephVersion, deploy)
		}
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: deploymentLabels,
			},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				InitContainers: []corev1.Container{
					getCrashDirInitContainer(cephCluster),
					getCrashChownInitContainer(cephCluster),
				},
				Containers: []corev1.Container{
					getCrashDaemonContainer(cephCluster, *cephVersion),
				},
				Tolerations:   tolerations,
				RestartPolicy: corev1.RestartPolicyAlways,
				HostNetwork:   cephCluster.Spec.Network.IsHost(),
				Volumes:       volumes,
			},
		}

		return nil
	}

	return controllerutil.CreateOrUpdate(context.TODO(), r.client, deploy, mutateFunc)
}

func getCrashDirInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	crashPostedDir := path.Join(dataPathMap.ContainerCrashDir(), "posted")

	container := corev1.Container{
		Name: "make-container-crash-dir",
		Command: []string{
			"mkdir",
			"-p",
		},
		Args: []string{
			crashPostedDir,
		},
		Image:           cephCluster.Spec.CephVersion.Image,
		SecurityContext: mon.PodSecurityContext(),
		Resources:       cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		VolumeMounts:    opspec.DaemonVolumeMounts(dataPathMap, ""),
	}
	return container
}

func getCrashChownInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)

	return opspec.ChownCephDataDirsInitContainer(
		*dataPathMap,
		cephCluster.Spec.CephVersion.Image,
		opspec.DaemonVolumeMounts(dataPathMap, ""),
		cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
		mon.PodSecurityContext(),
	)
}

func getCrashDaemonContainer(cephCluster cephv1.CephCluster, cephVersion version.CephVersion) corev1.Container {
	cephImage := cephCluster.Spec.CephVersion.Image
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	crashEnvVar := generateCrashEnvVar(cephVersion)
	envVars := append(opspec.DaemonEnvVars(cephImage), crashEnvVar)
	volumeMounts := opspec.DaemonVolumeMounts(dataPathMap, "")
	volumeMounts = append(volumeMounts, getVolumeMounts(cephVersion))

	container := corev1.Container{
		Name: "ceph-crash",
		Command: []string{
			"ceph-crash",
		},
		Image:        cephImage,
		Env:          envVars,
		VolumeMounts: volumeMounts,
		Resources:    cephv1.GetCrashCollectorResources(cephCluster.Spec.Resources),
	}

	return container
}

func clusterOwnerRef(clusterName, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", clusterResource.Group, clusterResource.Version),
		Kind:               clusterResource.Kind,
		Name:               clusterName,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func generateCrashEnvVar(v cephver.CephVersion) corev1.EnvVar {
	val := fmt.Sprintf("-m $(ROOK_CEPH_MON_HOST) -k %s", keyring.VolumeMount().AdminKeyringFilePath())
	if v.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}) {
		val = fmt.Sprintf("-m $(ROOK_CEPH_MON_HOST) -k %s", keyring.VolumeMount().CrashCollectorKeyringFilePath())
	}

	env := corev1.EnvVar{Name: "CEPH_ARGS", Value: val}
	return env
}

func getVolumeMounts(v cephver.CephVersion) corev1.VolumeMount {
	volumeMounts := keyring.VolumeMount().Admin()

	// As of Ceph Nautilus 14.2.5, the crash collector has its own key
	// If not running on on at least this version let's use the Ceph admin key
	// Thanks to https://github.com/ceph/ceph/pull/30844
	if v.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}) {
		volumeMounts = keyring.VolumeMount().CrashCollector()
	}

	return volumeMounts
}

func getVolumes(v cephver.CephVersion) corev1.Volume {
	volumes := keyring.Volume().Admin()

	// As of Ceph Nautilus 14.2.5, the crash collector has its own key
	// If not running on on at least this version let's use the Ceph admin key
	// Thanks to https://github.com/ceph/ceph/pull/30844
	if v.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 5}) {
		volumes = keyring.Volume().CrashCollector()
	}

	return volumes
}
