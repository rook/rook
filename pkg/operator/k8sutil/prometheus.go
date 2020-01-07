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
	"fmt"
	"io/ioutil"
	"path/filepath"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/coreos/prometheus-operator/pkg/client/versioned"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sYAML "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
)

func getMonitoringClient() (*monitoringclient.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", "")
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
func CreateOrUpdateServiceMonitor(serviceMonitorDefinition *monitoringv1.ServiceMonitor) (*monitoringv1.ServiceMonitor, error) {
	name := serviceMonitorDefinition.GetName()
	namespace := serviceMonitorDefinition.GetNamespace()
	logger.Debugf("creating servicemonitor %s", name)
	client, err := getMonitoringClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	sm, err := client.MonitoringV1().ServiceMonitors(namespace).Create(serviceMonitorDefinition)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create servicemonitor. %v", err)
		}
		sm, err = client.MonitoringV1().ServiceMonitors(namespace).Update(sm)
		if err != nil {
			return nil, fmt.Errorf("failed to update servicemonitor. %v", err)

		}
	}
	return sm, nil
}

// GetPrometheusRule returns provided prometheus rules or an error
func GetPrometheusRule(ruleFilePath string) (*monitoringv1.PrometheusRule, error) {
	ruleFile, err := ioutil.ReadFile(filepath.Clean(ruleFilePath))
	if err != nil {
		return nil, fmt.Errorf("prometheusRules file could not be fetched. %v", err)
	}
	var rule monitoringv1.PrometheusRule
	err = k8sYAML.NewYAMLOrJSONDecoder(bytes.NewBufferString(string(ruleFile)), 1000).Decode(&rule)
	if err != nil {
		return nil, fmt.Errorf("prometheusRules could not be decoded. %v", err)
	}
	return &rule, nil
}

// CreateOrUpdatePrometheusRule creates a prometheusRule object or an error
func CreateOrUpdatePrometheusRule(prometheusRule *monitoringv1.PrometheusRule) (*monitoringv1.PrometheusRule, error) {
	name := prometheusRule.GetName()
	namespace := prometheusRule.GetNamespace()
	logger.Debugf("creating prometheusRule %s", name)
	client, err := getMonitoringClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	promRule, err := client.MonitoringV1().PrometheusRules(namespace).Create(prometheusRule)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create prometheusRules. %v", err)
		}
		// Get current PrometheusRule so the ResourceVersion can be set as needed
		// for the object update operation
		promRule, err := client.MonitoringV1().PrometheusRules(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get prometheusRule object. %v", err)
		}
		prometheusRule.ObjectMeta.ResourceVersion = promRule.ObjectMeta.ResourceVersion

		promRule, err = client.MonitoringV1().PrometheusRules(namespace).Update(prometheusRule)
		if err != nil {
			return nil, fmt.Errorf("failed to update prometheusRule. %v", err)
		}
	}
	return promRule, nil
}
