/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"

	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	monitoringPath                   = "/etc/ceph-monitoring/"
	serviceMonitorFile               = "exporter-service-monitor.yaml"
	sockDir                          = "/run/ceph"
	defaultPrioLimit                 = "5"
	defaultStatsPeriod               = "5"
	DefaultMetricsPort        uint16 = 9926
	exporterServiceMetricName        = "ceph-exporter-http-metrics"
	exporterKeyringUsername          = "client.ceph-exporter"
	exporterKeyName                  = "rook-ceph-exporter-keyring"
)

var (
	MinVersionForCephExporter = cephver.CephVersion{Major: 18, Minor: 0, Extra: 0}
)

// createOrUpdateCephExporter is a wrapper around controllerutil.CreateOrUpdate
func (r *ReconcileNode) createOrUpdateCephExporter(node corev1.Node, tolerations []corev1.Toleration, cephCluster cephv1.CephCluster, cephVersion *cephver.CephVersion) (controllerutil.OperationResult, error) {
	// CephVersion change is done temporarily, as some regression was detected in Ceph version 17.2.6 which is summarised here https://github.com/ceph/ceph/pull/50718#issuecomment-1505608312.
	// Thus, disabling ceph-exporter for now until all the regression are fixed.
	if !cephVersion.IsAtLeast(MinVersionForCephExporter) {
		logger.Infof("Skipping exporter reconcile on ceph version %q", cephVersion.String())
		return controllerutil.OperationResultNone, nil
	}
	if cephCluster.Spec.Monitoring.MetricsDisabled {
		logger.Info("Skipping exporter reconcile since monitoring.metricsDisabled is true")
		return controllerutil.OperationResultNone, nil
	}

	nodeHostnameLabel, ok := node.Labels[k8sutil.LabelHostname()]
	if !ok {
		return controllerutil.OperationResultNone, errors.Errorf("label key %q does not exist on node %q", k8sutil.LabelHostname(), node.GetName())
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", cephExporterAppName), nodeHostnameLabel),
			Namespace: cephCluster.GetNamespace(),
		},
	}
	deploy.Spec.RevisionHistoryLimit = controller.RevisionHistoryLimit()
	err := controllerutil.SetControllerReference(&cephCluster, deploy, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Errorf("failed to set owner reference of ceph-exporter deployment %q", deploy.Name)
	}

	volumes := append(
		controller.DaemonVolumesBase(config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath), "", cephCluster.Spec.DataDirHostPath),
		keyring.Volume().Exporter())

	mutateFunc := func() error {

		// labels for the pod, the deployment, and the deploymentSelector
		deploymentLabels := map[string]string{
			k8sutil.LabelHostname(): nodeHostnameLabel,
			k8sutil.AppAttr:         cephExporterAppName,
			NodeNameLabel:           node.GetName(),
		}
		deploymentLabels[controller.DaemonIDLabel] = "exporter"
		deploymentLabels[k8sutil.ClusterAttr] = cephCluster.GetNamespace()

		selectorLabels := map[string]string{
			k8sutil.LabelHostname(): nodeHostnameLabel,
			k8sutil.AppAttr:         cephExporterAppName,
			NodeNameLabel:           node.GetName(),
		}

		nodeSelector := map[string]string{k8sutil.LabelHostname(): nodeHostnameLabel}

		// Deployment selector is immutable so we set this value only if
		// a new object is going to be created
		if deploy.ObjectMeta.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
		}

		deploy.ObjectMeta.Labels = deploymentLabels
		cephv1.GetCephExporterLabels(cephCluster.Spec.Labels).ApplyToObjectMeta(&deploy.ObjectMeta)
		k8sutil.AddRookVersionLabelToDeployment(deploy)
		if cephVersion != nil {
			controller.AddCephVersionLabelToDeployment(*cephVersion, deploy)
		}

		// wait for previous exporter pod to be deleted, before creating a new one
		// to avoid fighting for the same socket file
		deploy.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType

		var terminationGracePeriodSeconds int64 = 2
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
				Tolerations:                   tolerations,
				RestartPolicy:                 corev1.RestartPolicyAlways,
				HostNetwork:                   cephCluster.Spec.Network.IsHost(),
				Volumes:                       volumes,
				PriorityClassName:             cephv1.GetCephExporterPriorityClassName(cephCluster.Spec.PriorityClassNames),
				TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
				SecurityContext:               &corev1.PodSecurityContext{},
				ServiceAccountName:            k8sutil.DefaultServiceAccount,
			},
		}
		cephv1.GetCephExporterAnnotations(cephCluster.Spec.Annotations).ApplyToObjectMeta(&deploy.Spec.Template.ObjectMeta)
		applyPrometheusAnnotations(cephCluster, &deploy.Spec.Template.ObjectMeta)

		return nil
	}

	return controllerutil.CreateOrUpdate(r.opManagerContext, r.client, deploy, mutateFunc)
}

func getCephExporterChownInitContainer(cephCluster cephv1.CephCluster) corev1.Container {
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	mounts := controller.DaemonVolumeMounts(dataPathMap, "", cephCluster.Spec.DataDirHostPath)

	return controller.ChownCephDataDirsInitContainer(
		*dataPathMap,
		cephCluster.Spec.CephVersion.Image,
		controller.GetContainerImagePullPolicy(cephCluster.Spec.CephVersion.ImagePullPolicy),
		mounts,
		cephv1.GetCephExporterResources(cephCluster.Spec.Resources),
		controller.PodSecurityContext(),
		"",
	)
}

func getCephExporterDaemonContainer(cephCluster cephv1.CephCluster, cephVersion cephver.CephVersion) corev1.Container {
	cephImage := cephCluster.Spec.CephVersion.Image
	dataPathMap := config.NewDatalessDaemonDataPathMap(cephCluster.GetNamespace(), cephCluster.Spec.DataDirHostPath)
	volumeMounts := controller.DaemonVolumeMounts(dataPathMap, "", cephCluster.Spec.DataDirHostPath)
	volumeMounts = append(volumeMounts, keyring.VolumeMount().Exporter())

	exporterEnvVar := generateExporterEnvVar()
	envVars := append(controller.DaemonEnvVars(&cephCluster.Spec), exporterEnvVar)

	prioLimit, statsPeriod := defaultPrioLimit, defaultStatsPeriod
	if cephCluster.Spec.Monitoring.Exporter != nil {
		prioLimit = strconv.Itoa(int(cephCluster.Spec.Monitoring.Exporter.PerfCountersPrioLimit))
		statsPeriod = strconv.Itoa(int(cephCluster.Spec.Monitoring.Exporter.StatsPeriodSeconds))
	}
	args := []string{
		"--sock-dir", sockDir,
		"--port", strconv.Itoa(int(DefaultMetricsPort)),
		"--prio-limit", prioLimit,
		"--stats-period", statsPeriod,
	}

	// If DualStack or IPv6 is enabled ensure ceph-exporter binds to both IPv6 and IPv4 interfaces.
	if cephCluster.Spec.Network.DualStack || cephCluster.Spec.Network.IPFamily == "IPv6" {
		args = append(args, "--addrs", "::")
	}

	container := corev1.Container{
		Name:            "ceph-exporter",
		Command:         []string{"ceph-exporter"},
		Args:            args,
		Image:           cephImage,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(cephCluster.Spec.CephVersion.ImagePullPolicy),
		Env:             envVars,
		VolumeMounts:    volumeMounts,
		Resources:       cephv1.GetCephExporterResources(cephCluster.Spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}

// MakeCephExporterMetricsService generates the Kubernetes service object for the exporter monitoring service
func MakeCephExporterMetricsService(cephCluster cephv1.CephCluster, servicePortMetricName string, scheme *runtime.Scheme) (*corev1.Service, error) {
	labels := controller.AppLabels(cephExporterAppName, cephCluster.Namespace)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cephExporterAppName,
			Namespace: cephCluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     servicePortMetricName,
					Port:     int32(DefaultMetricsPort),
					Protocol: corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	err := controllerutil.SetControllerReference(&cephCluster, svc, scheme)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to monitoring service %q", svc.Name)
	}
	return svc, nil
}

// EnableCephExporterServiceMonitor add a servicemonitor that allows prometheus to scrape from the monitoring endpoint of the exporter
func EnableCephExporterServiceMonitor(context *clusterd.Context, cephCluster cephv1.CephCluster, scheme *runtime.Scheme, opManagerContext context.Context, servicePortMetricName string) error {
	serviceMonitor := k8sutil.GetServiceMonitor(cephExporterAppName, cephCluster.Namespace, servicePortMetricName)

	if cephCluster.Spec.Monitoring.Interval != nil {
		duration := cephCluster.Spec.Monitoring.Interval.Duration.String()
		serviceMonitor.Spec.Endpoints[0].Interval = monitoringv1.Duration(duration)
	}

	cephv1.GetCephExporterLabels(cephCluster.Spec.Labels).OverwriteApplyToObjectMeta(&serviceMonitor.ObjectMeta)

	err := controllerutil.SetControllerReference(&cephCluster, serviceMonitor, scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to service monitor %q", serviceMonitor.Name)
	}
	serviceMonitor.Spec.Selector.MatchLabels = controller.AppLabels(cephExporterAppName, cephCluster.Namespace)
	applyCephExporterLabels(cephCluster, serviceMonitor)

	if _, err = k8sutil.CreateOrUpdateServiceMonitor(context, opManagerContext, serviceMonitor); err != nil {
		return errors.Wrap(err, "service monitor could not be enabled")
	}
	return nil
}

func applyCephExporterLabels(cephCluster cephv1.CephCluster, serviceMonitor *monitoringv1.ServiceMonitor) {
	if cephCluster.Spec.Labels != nil {
		cephv1.GetMonitoringLabels(cephCluster.Spec.Labels).OverwriteApplyToObjectMeta(&serviceMonitor.ObjectMeta)
		if cephExporterLabels, ok := cephCluster.Spec.Labels["exporter"]; ok {
			if managedBy, ok := cephExporterLabels["rook.io/managedBy"]; ok {
				relabelConfig := monitoringv1.RelabelConfig{
					TargetLabel: "managedBy",
					Replacement: &managedBy,
				}
				serviceMonitor.Spec.Endpoints[0].RelabelConfigs = append(
					serviceMonitor.Spec.Endpoints[0].RelabelConfigs, relabelConfig)
			} else {
				logger.Info("rook.io/managedBy not specified in ceph-exporter labels")
			}
		} else {
			logger.Debug("ceph-exporter labels not specified")
		}
	}
}

func applyPrometheusAnnotations(cephCluster cephv1.CephCluster, objectMeta *metav1.ObjectMeta) {
	if len(cephv1.GetCephExporterAnnotations(cephCluster.Spec.Annotations)) == 0 {
		t := cephv1.Annotations{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   strconv.Itoa(int(DefaultMetricsPort)),
		}

		t.ApplyToObjectMeta(objectMeta)
	}
}

func generateExporterEnvVar() corev1.EnvVar {
	val := fmt.Sprintf("-m $(ROOK_CEPH_MON_HOST) -n %s -k %s", exporterKeyringUsername, keyring.VolumeMount().ExporterKeyringFilePath())
	env := corev1.EnvVar{Name: "CEPH_ARGS", Value: val}

	return env
}
