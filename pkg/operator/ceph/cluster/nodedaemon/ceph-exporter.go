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

package nodedaemon

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"

	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// cephExporterKeyringUsername = "client.ceph-exporter"
	// cephExporterKeyName   = "rook-ceph-exporter-keyring"
	cephConfFile          = "/var/lib/rook/rook-ceph/rook-ceph.config"
	sockDir               = "/run/ceph"
	defaultPort           = "9926"
	perfCountersPrioLimit = "5"
	statsPeriod           = "5"
)

// createOrUpdateCephExporter is a wrapper around controllerutil.CreateOrUpdate
func (r *ReconcileNode) createOrUpdateCephExporter(node corev1.Node, tolerations []corev1.Toleration, cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion) (controllerutil.OperationResult, error) {
	// Create or Update the deployment default/foo
	nodeHostnameLabel, ok := node.ObjectMeta.Labels[corev1.LabelHostname]
	if !ok {
		return controllerutil.OperationResultNone, errors.Errorf("label key %q does not exist on node %q", corev1.LabelHostname, node.GetName())
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", cephExporterAppName), nodeHostnameLabel),
			Namespace: cephCluster.GetNamespace(),
		},
	}
	err := controllerutil.SetControllerReference(&cephCluster, deploy, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Errorf("failed to set owner reference of ceph-exporter deployment %q", deploy.Name)
	}

	volumes := controller.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "", cephCluster.Spec.DataDirHostPath, true)

	mutateFunc := func() error {

		// labels for the pod, the deployment, and the deploymentSelector
		deploymentLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      cephExporterAppName,
			NodeNameLabel:        node.GetName(),
		}
		deploymentLabels[config.CephExporterType] = "ceph-exporter"
		deploymentLabels[controller.DaemonIDLabel] = "ceph-exporter"
		deploymentLabels[k8sutil.ClusterAttr] = cephCluster.GetNamespace()

		selectorLabels := map[string]string{
			corev1.LabelHostname: nodeHostnameLabel,
			k8sutil.AppAttr:      cephExporterAppName,
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
		cephv1.GetCephExporterCollectorLabels(cephCluster.Spec.Labels).ApplyToObjectMeta(&deploy.ObjectMeta)
		k8sutil.AddRookVersionLabelToDeployment(deploy)
		if cephVersion != nil {
			controller.AddCephVersionLabelToDeployment(*cephVersion, deploy)
		}
		deploy.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: deploymentLabels,
			},
			Spec: corev1.PodSpec{
				NodeSelector: nodeSelector,
				InitContainers: []corev1.Container{
					getCephExporterChownInitContainer(cephCluster),
				},
				Containers: []corev1.Container{
					getCephExporterDaemonContainer(cephCluster, *cephVersion),
				},
				Tolerations:   tolerations,
				RestartPolicy: corev1.RestartPolicyAlways,
				HostNetwork:   cephCluster.Spec.Network.IsHost(),
				Volumes:       volumes,
			},
		}

		return nil
	}

	return controllerutil.CreateOrUpdate(r.opManagerContext, r.client, deploy, mutateFunc)
}

func getCephExporterChownInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)

	return controller.ChownCephDataDirsInitContainer(
		*dataPathMap,
		cephCluster.Spec.CephVersion.Image,
		controller.GetContainerImagePullPolicy(cephCluster.Spec.CephVersion.ImagePullPolicy),
		controller.DaemonVolumeMounts(dataPathMap, "", true),
		cephv1.GetCephExporterResources(cephCluster.Spec.Resources),
		controller.PodSecurityContext(),
	)
}

func getCephExporterDaemonContainer(cephCluster cephv1.CephCluster, cephVersion cephver.CephVersion) corev1.Container {
	cephImage := cephCluster.Spec.CephVersion.Image
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	volumeMounts := controller.DaemonVolumeMounts(dataPathMap, "", true)

	container := corev1.Container{
		Name:            "ceph-exporter",
		Command:         []string{"/usr/bin/ceph-exporter"},
		Args:            []string{"--conf", cephConfFile, "--sock-dir", sockDir, "--port", defaultPort, "--prio-limit", perfCountersPrioLimit, "--stats-period", statsPeriod},
		Image:           cephImage,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(cephCluster.Spec.CephVersion.ImagePullPolicy),
		VolumeMounts:    volumeMounts,
		Resources:       cephv1.GetCephExporterResources(cephCluster.Spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}
