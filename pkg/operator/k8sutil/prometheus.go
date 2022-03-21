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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sYAML "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
)

func getMonitoringClient() (*monitoringclient.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to build config. %v", err)
	}
	client, err := monitoringclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	return client, nil
}

// GetServiceMonitor returns servicemonitor or an error
func GetServiceMonitor(filePath string) (*monitoringv1.ServiceMonitor, error) {
	file, err := ioutil.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("servicemonitor file could not be fetched. %v", err)
	}
	var servicemonitor monitoringv1.ServiceMonitor
	err = k8sYAML.NewYAMLOrJSONDecoder(bytes.NewBufferString(string(file)), 1000).Decode(&servicemonitor)
	if err != nil {
		return nil, fmt.Errorf("servicemonitor could not be decoded. %v", err)
	}
	return &servicemonitor, nil
}

// CreateOrUpdateServiceMonitor creates serviceMonitor object or an error
func CreateOrUpdateServiceMonitor(ctx context.Context, serviceMonitorDefinition *monitoringv1.ServiceMonitor) (*monitoringv1.ServiceMonitor, error) {
	name := serviceMonitorDefinition.GetName()
	namespace := serviceMonitorDefinition.GetNamespace()
	logger.Debugf("creating servicemonitor %s", name)
	client, err := getMonitoringClient()
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
