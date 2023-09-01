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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/rook/rook/pkg/clusterd"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getMonitoringClient(context *clusterd.Context) (*monitoringclient.Clientset, error) {
	client, err := monitoringclient.NewForConfig(context.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	return client, nil
}

// GetServiceMonitor creates serviceMonitor object template
func GetServiceMonitor(name string, namespace string, portName string) *monitoringv1.ServiceMonitor {
	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"team": "rook",
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					namespace,
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":          name,
					"rook_cluster": namespace,
				},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     portName,
					Path:     "/metrics",
					Interval: "5s",
				},
			},
		},
	}
}

// CreateOrUpdateServiceMonitor creates serviceMonitor object or an error
func CreateOrUpdateServiceMonitor(context *clusterd.Context, ctx context.Context, serviceMonitorDefinition *monitoringv1.ServiceMonitor) (*monitoringv1.ServiceMonitor, error) {
	name := serviceMonitorDefinition.GetName()
	namespace := serviceMonitorDefinition.GetNamespace()
	logger.Debugf("creating servicemonitor %s", name)
	client, err := getMonitoringClient(context)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	oldSm, err := client.MonitoringV1().ServiceMonitors(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			sm, err := client.MonitoringV1().ServiceMonitors(namespace).Create(ctx, serviceMonitorDefinition, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to create servicemonitor. %v", err)
			}
			return sm, nil
		}
		return nil, fmt.Errorf("failed to retrieve servicemonitor. %v", err)
	}
	oldSm.Spec = serviceMonitorDefinition.Spec
	oldSm.ObjectMeta.Labels = serviceMonitorDefinition.ObjectMeta.Labels
	sm, err := client.MonitoringV1().ServiceMonitors(namespace).Update(ctx, oldSm, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update servicemonitor. %v", err)
	}
	return sm, nil
}

// DeleteServiceMonitor deletes a ServiceMonitor and returns the error if any
func DeleteServiceMonitor(context *clusterd.Context, ctx context.Context, ns string, name string) error {
	client, err := getMonitoringClient(context)
	if err != nil {
		return fmt.Errorf("failed to get monitoring client. %v", err)
	}
	_, err = client.MonitoringV1().ServiceMonitors(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// Either the service monitor does not exist or there are no privileges to detect it
		// so we ignore any errors
		return nil
	}
	err = client.MonitoringV1().ServiceMonitors(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if kerror.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to delete service monitor %q", name)
	}
	return nil
}
