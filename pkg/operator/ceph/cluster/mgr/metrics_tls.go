/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package mgr

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	serviceMonitorPortHTTPS = "https-metrics"
	//#nosec G101 -- This is only a volume name
	metricsTLSSecretVolumeName = "metrics-tls"
	//#nosec G101 -- This is only a path name
	metricsTLSSecretMountPath = "/etc/ceph/metrics-tls"
	metricsTLSCertFile        = "/etc/ceph/metrics-tls/tls.crt"
	metricsTLSKeyFile         = "/etc/ceph/metrics-tls/tls.key"
	//#nosec G101 -- This is only an annotation key
	metricsTLSSecretHashAnnotation = "rook.io/metrics-tls-secret-hash"
	defaultMetricsSSLPort          = 9284
)

func (c *Cluster) isMetricsTLSEnabled() bool {
	return c.spec.Monitoring.MetricsTLS != nil && c.spec.Monitoring.MetricsTLS.Enabled != nil && *c.spec.Monitoring.MetricsTLS.Enabled
}

func (c *Cluster) shouldConfigureMetricsTLS() bool {
	return c.isMetricsTLSEnabled() && c.clusterInfo.CephVersion.IsAtLeastTentacle()
}

func (c *Cluster) metricsTLSSecretName() string {
	if c.spec.Monitoring.MetricsTLS != nil && c.spec.Monitoring.MetricsTLS.SecretName != "" {
		return c.spec.Monitoring.MetricsTLS.SecretName
	}
	return cephv1.DefaultMetricsTLSSecretName
}

func (c *Cluster) metricsServicePortName() string {
	if c.shouldConfigureMetricsTLS() {
		return serviceMonitorPortHTTPS
	}
	return serviceMonitorPort
}

func (c *Cluster) configurePrometheusTLS(monStore *config.MonStore, daemonID string) error {
	if c.spec.Monitoring.MetricsTLS == nil {
		return nil
	}
	if !c.isMetricsTLSEnabled() {
		return monStore.SetAll(daemonID, map[string]string{
			"mgr/prometheus/ssl":             "false",
			"mgr/prometheus/crt_file":        "",
			"mgr/prometheus/key_file":        "",
			"mgr/prometheus/ssl_server_port": "",
		})
	}
	if !c.shouldConfigureMetricsTLS() {
		return nil
	}

	return monStore.SetAll(daemonID, map[string]string{
		"mgr/prometheus/ssl":             "true",
		"mgr/prometheus/crt_file":        metricsTLSCertFile,
		"mgr/prometheus/key_file":        metricsTLSKeyFile,
		"mgr/prometheus/ssl_server_port": fmt.Sprintf("%d", defaultMetricsSSLPort),
	})
}

func (c *Cluster) applyMetricsTLSToService(svc *corev1.Service) {
	if !c.shouldConfigureMetricsTLS() {
		return
	}
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       serviceMonitorPortHTTPS,
			Port:       int32(DefaultMetricsPort),
			TargetPort: intstr.FromInt(defaultMetricsSSLPort),
			Protocol:   corev1.ProtocolTCP,
		},
	}
}

func (c *Cluster) reconcileMetricsService(desired *corev1.Service) error {
	existing, err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Get(
		c.clusterInfo.Context, desired.Name, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		_, err = k8sutil.CreateOrUpdateService(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, desired)
		return err
	}

	existing.Spec.Ports = desired.Spec.Ports

	_, err = c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Update(
		c.clusterInfo.Context, existing, metav1.UpdateOptions{})
	return err
}

func (c *Cluster) applyMetricsTLSToDeployment(d *apps.Deployment) {
	if !c.shouldConfigureMetricsTLS() {
		return
	}

	optional := true
	d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: metricsTLSSecretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: c.metricsTLSSecretName(),
				Optional:   &optional,
			},
		},
	})

	for i := range d.Spec.Template.Spec.Containers {
		if d.Spec.Template.Spec.Containers[i].Name != "mgr" {
			continue
		}

		d.Spec.Template.Spec.Containers[i].VolumeMounts = append(
			d.Spec.Template.Spec.Containers[i].VolumeMounts,
			corev1.VolumeMount{
				Name:      metricsTLSSecretVolumeName,
				MountPath: metricsTLSSecretMountPath,
				ReadOnly:  true,
			},
		)

		d.Spec.Template.Spec.Containers[i].Ports = append(
			d.Spec.Template.Spec.Containers[i].Ports,
			corev1.ContainerPort{
				Name:          serviceMonitorPortHTTPS,
				ContainerPort: defaultMetricsSSLPort,
				Protocol:      corev1.ProtocolTCP,
			},
		)
	}

	if hash := c.getMetricsTLSSecretResourceVersion(); hash != "" {
		if d.Spec.Template.Annotations == nil {
			d.Spec.Template.Annotations = map[string]string{}
		}
		d.Spec.Template.Annotations[metricsTLSSecretHashAnnotation] = hash
	}
}

func (c *Cluster) getMetricsTLSSecretResourceVersion() string {
	secret, err := c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Get(
		c.clusterInfo.Context, c.metricsTLSSecretName(), metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return secret.ResourceVersion
}

func (c *Cluster) applyMetricsTLSToServiceMonitor(serviceMonitor *monitoringv1.ServiceMonitor) {
	if !c.shouldConfigureMetricsTLS() {
		return
	}
	endpoint := &serviceMonitor.Spec.Endpoints[0]
	endpoint.Port = serviceMonitorPortHTTPS
	endpoint.Scheme = ptr.To(monitoringv1.SchemeHTTPS)
	tlsConfig := &monitoringv1.TLSConfig{
		SafeTLSConfig: monitoringv1.SafeTLSConfig{
			ServerName: ptr.To(fmt.Sprintf("%s.%s.svc", AppName, c.clusterInfo.Namespace)),
		},
	}

	tlsConfig.CA = monitoringv1.SecretOrConfigMap{
		Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: c.metricsTLSSecretName()},
			Key:                  "ca.crt",
		},
	}
	endpoint.TLSConfig = tlsConfig
}
