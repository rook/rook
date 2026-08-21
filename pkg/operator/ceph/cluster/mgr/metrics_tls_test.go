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
	"context"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func testClusterWithMetricsTLS() *Cluster {
	enabled := true
	clusterInfo := cephclient.AdminTestClusterInfo("rook-ceph")
	clusterInfo.CephVersion = cephver.Tentacle
	return &Cluster{
		clusterInfo: clusterInfo,
		spec: cephv1.ClusterSpec{
			Monitoring: cephv1.MonitoringSpec{
				MetricsTLS: &cephv1.MetricsTLSSpec{
					Enabled: &enabled,
				},
			},
		},
	}
}

func TestShouldConfigureMetricsTLS(t *testing.T) {
	c := testClusterWithMetricsTLS()
	if !c.shouldConfigureMetricsTLS() {
		t.Fatal("expected metrics TLS on Tentacle")
	}

	c.clusterInfo.CephVersion = cephver.Squid
	if c.shouldConfigureMetricsTLS() {
		t.Fatal("expected metrics TLS to require Tentacle+")
	}
}

func TestApplyMetricsTLSToService(t *testing.T) {
	c := testClusterWithMetricsTLS()
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: AppName}}
	c.applyMetricsTLSToService(svc)

	if svc.Spec.Ports[0].Name != serviceMonitorPortHTTPS {
		t.Fatalf("unexpected port name %q", svc.Spec.Ports[0].Name)
	}
	if svc.Spec.Ports[0].TargetPort != intstr.FromInt(defaultMetricsSSLPort) {
		t.Fatalf("unexpected target port %v", svc.Spec.Ports[0].TargetPort)
	}
}

func TestApplyMetricsTLSToServiceMonitor(t *testing.T) {
	c := testClusterWithMetricsTLS()
	sm := &monitoringv1.ServiceMonitor{
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{{Port: serviceMonitorPort}},
		},
	}
	c.applyMetricsTLSToServiceMonitor(sm)
	endpoint := sm.Spec.Endpoints[0]
	if endpoint.Port != serviceMonitorPortHTTPS {
		t.Fatalf("unexpected port %q", endpoint.Port)
	}
	if endpoint.Scheme == nil || *endpoint.Scheme != monitoringv1.SchemeHTTPS {
		t.Fatal("expected HTTPS scheme")
	}
	if endpoint.TLSConfig == nil || endpoint.TLSConfig.ServerName == nil {
		t.Fatal("expected TLS server name")
	}
	if endpoint.TLSConfig.CA.Secret == nil || endpoint.TLSConfig.CA.Secret.Key != "ca.crt" {
		t.Fatal("expected ca.crt from the TLS Secret")
	}
}

func TestReconcileMetricsServicePreservesAnnotations(t *testing.T) {
	const namespace = "rook-ceph"
	clientset := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppName,
			Namespace: namespace,
			Annotations: map[string]string{
				"example.com/custom": "keep-me",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: serviceMonitorPort, Port: int32(DefaultMetricsPort)}},
		},
	})

	c := testClusterWithMetricsTLS()
	c.context = &clusterd.Context{Clientset: clientset}
	c.clusterInfo.Namespace = namespace
	c.clusterInfo.Context = context.Background()

	desired := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: AppName, Namespace: namespace}}
	c.applyMetricsTLSToService(desired)

	if err := c.reconcileMetricsService(desired); err != nil {
		t.Fatalf("reconcile metrics service: %v", err)
	}

	updated, err := clientset.CoreV1().Services(namespace).Get(context.Background(), AppName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated service: %v", err)
	}
	if updated.Annotations["example.com/custom"] != "keep-me" {
		t.Fatal("expected annotation to be preserved")
	}
	if updated.Spec.Ports[0].Name != serviceMonitorPortHTTPS {
		t.Fatalf("unexpected port name %q", updated.Spec.Ports[0].Name)
	}
}

func newMetricsTLSMonStore(t *testing.T) (*config.MonStore, func() map[string]string) {
	var configFile *ini.File

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			joinedArgs := strings.Join(args, " ")
			if !strings.HasPrefix(joinedArgs, "config assimilate-conf") {
				return "", fmt.Errorf("unexpected ceph command %q", args)
			}

			fs := flag.NewFlagSet("", flag.ContinueOnError)
			inputFile := fs.String("i", "", "")
			if err := fs.Parse(args[2:4]); err != nil {
				return "", fmt.Errorf("parse flags: %w", err)
			}

			f, err := ini.Load(*inputFile)
			if err != nil {
				return "", fmt.Errorf("load ini file: %w", err)
			}
			configFile = f
			return "", nil
		},
	}

	ctx := &clusterd.Context{
		Clientset: testop.New(t, 1),
		Executor:  executor,
	}
	monStore := config.GetMonStore(ctx, cephclient.AdminTestClusterInfo("rook-ceph"))

	return monStore, func() map[string]string {
		if configFile == nil {
			return nil
		}
		settings := map[string]string{}
		for _, key := range configFile.Section("mgr").Keys() {
			settings[key.Name()] = key.Value()
		}
		return settings
	}
}

func TestConfigurePrometheusTLS(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	disableSettings := map[string]string{
		"mgr/prometheus/ssl":             "false",
		"mgr/prometheus/crt_file":        "",
		"mgr/prometheus/key_file":        "",
		"mgr/prometheus/ssl_server_port": "",
	}
	enableSettings := map[string]string{
		"mgr/prometheus/ssl":             "true",
		"mgr/prometheus/crt_file":        metricsTLSCertFile,
		"mgr/prometheus/key_file":        metricsTLSKeyFile,
		"mgr/prometheus/ssl_server_port": fmt.Sprintf("%d", defaultMetricsSSLPort),
	}

	tests := []struct {
		name             string
		metricsTLS       *cephv1.MetricsTLSSpec
		cephVersion      cephver.CephVersion
		expectAssimilate bool
		expectedSettings map[string]string
	}{
		{
			name:             "metrics TLS not configured",
			metricsTLS:       nil,
			cephVersion:      cephver.Tentacle,
			expectAssimilate: false,
		},
		{
			name:             "metrics TLS disabled explicitly",
			metricsTLS:       &cephv1.MetricsTLSSpec{Enabled: &enabledFalse},
			cephVersion:      cephver.Tentacle,
			expectAssimilate: true,
			expectedSettings: disableSettings,
		},
		{
			name:             "metrics TLS enabled without Tentacle",
			metricsTLS:       &cephv1.MetricsTLSSpec{Enabled: &enabledTrue},
			cephVersion:      cephver.Squid,
			expectAssimilate: false,
		},
		{
			name:             "metrics TLS enabled on Tentacle",
			metricsTLS:       &cephv1.MetricsTLSSpec{Enabled: &enabledTrue},
			cephVersion:      cephver.Tentacle,
			expectAssimilate: true,
			expectedSettings: enableSettings,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monStore, getSettings := newMetricsTLSMonStore(t)

			c := testClusterWithMetricsTLS()
			c.clusterInfo.CephVersion = tt.cephVersion
			c.spec.Monitoring.MetricsTLS = tt.metricsTLS

			err := c.configurePrometheusTLS(monStore, "mgr")
			require.NoError(t, err)

			settings := getSettings()
			if !tt.expectAssimilate {
				assert.Nil(t, settings)
				return
			}
			assert.Equal(t, tt.expectedSettings, settings)
		})
	}
}

func testMgrDeployment() *apps.Deployment {
	return &apps.Deployment{
		Spec: apps.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "mgr"},
						{Name: "watch-active"},
					},
				},
			},
		},
	}
}

func TestApplyMetricsTLSToDeployment(t *testing.T) {
	const namespace = "rook-ceph"
	enabledTrue := true
	secretResourceVersion := "12345"

	t.Run("metrics TLS not configured", func(t *testing.T) {
		c := testClusterWithMetricsTLS()
		c.clusterInfo.CephVersion = cephver.Squid
		d := testMgrDeployment()

		c.applyMetricsTLSToDeployment(d)

		assert.Empty(t, d.Spec.Template.Spec.Volumes)
		assert.Empty(t, d.Spec.Template.Spec.Containers[0].VolumeMounts)
		assert.Empty(t, d.Spec.Template.Spec.Containers[0].Ports)
		assert.Empty(t, d.Spec.Template.Annotations)
	})

	t.Run("metrics TLS enabled on Tentacle", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            cephv1.DefaultMetricsTLSSecretName,
				Namespace:       namespace,
				ResourceVersion: secretResourceVersion,
			},
		})

		c := testClusterWithMetricsTLS()
		c.context = &clusterd.Context{Clientset: clientset}
		c.clusterInfo.Namespace = namespace
		c.clusterInfo.Context = context.Background()

		d := testMgrDeployment()
		c.applyMetricsTLSToDeployment(d)

		require.Len(t, d.Spec.Template.Spec.Volumes, 1)
		volume := d.Spec.Template.Spec.Volumes[0]
		assert.Equal(t, metricsTLSSecretVolumeName, volume.Name)
		assert.Equal(t, cephv1.DefaultMetricsTLSSecretName, volume.Secret.SecretName)
		assert.NotNil(t, volume.Secret.Optional)
		assert.True(t, *volume.Secret.Optional)

		mgrContainer := d.Spec.Template.Spec.Containers[0]
		require.Len(t, mgrContainer.VolumeMounts, 1)
		assert.Equal(t, metricsTLSSecretVolumeName, mgrContainer.VolumeMounts[0].Name)
		assert.Equal(t, metricsTLSSecretMountPath, mgrContainer.VolumeMounts[0].MountPath)
		assert.True(t, mgrContainer.VolumeMounts[0].ReadOnly)

		assert.Empty(t, d.Spec.Template.Spec.Containers[1].VolumeMounts)

		require.Len(t, mgrContainer.Ports, 1)
		assert.Equal(t, serviceMonitorPortHTTPS, mgrContainer.Ports[0].Name)
		assert.Equal(t, int32(defaultMetricsSSLPort), mgrContainer.Ports[0].ContainerPort)
		assert.Equal(t, corev1.ProtocolTCP, mgrContainer.Ports[0].Protocol)

		assert.Equal(t, secretResourceVersion, d.Spec.Template.Annotations[metricsTLSSecretHashAnnotation])
	})

	t.Run("custom secret name", func(t *testing.T) {
		//#nosec G101 -- This is only a test
		customSecret := "my-metrics-tls-secret"
		clientset := fake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            customSecret,
				Namespace:       namespace,
				ResourceVersion: secretResourceVersion,
			},
		})

		c := testClusterWithMetricsTLS()
		c.context = &clusterd.Context{Clientset: clientset}
		c.clusterInfo.Namespace = namespace
		c.clusterInfo.Context = context.Background()
		c.spec.Monitoring.MetricsTLS = &cephv1.MetricsTLSSpec{
			Enabled:    &enabledTrue,
			SecretName: customSecret,
		}

		d := testMgrDeployment()
		c.applyMetricsTLSToDeployment(d)

		assert.Equal(t, customSecret, d.Spec.Template.Spec.Volumes[0].Secret.SecretName)
		assert.Equal(t, secretResourceVersion, d.Spec.Template.Annotations[metricsTLSSecretHashAnnotation])
	})
}
