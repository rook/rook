package k8sutil

import (
	"bytes"
	"fmt"
	"io/ioutil"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/coreos/prometheus-operator/pkg/client/versioned"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sYAML "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
)

func getMonitoringClient() (*monitoringclient.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	client, err := monitoringclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %+v", err)
	}
	return client, nil
}

// GetServiceMonitor returns servicemonitor or an error
func GetServiceMonitor(filePath string) (*monitoringv1.ServiceMonitor, error) {
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("servicemonitor file could not be fetched. %+v", err)
	}
	var servicemonitor monitoringv1.ServiceMonitor
	err = k8sYAML.NewYAMLOrJSONDecoder(bytes.NewBufferString(string(file)), 1000).Decode(&servicemonitor)
	if err != nil {
		return nil, fmt.Errorf("servicemonitor could not be decoded. %+v", err)
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
		return nil, fmt.Errorf("failed to get monitoring client. %+v", err)
	}
	sm, err := client.MonitoringV1().ServiceMonitors(namespace).Create(serviceMonitorDefinition)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create servicemonitor. %+v", err)
		}
		sm, err = client.MonitoringV1().ServiceMonitors(namespace).Update(sm)
		if err != nil {
			return nil, fmt.Errorf("failed to update servicemonitor. %+v", err)

		}
	}
	return sm, nil
}

// GetPrometheusRule returns provided prometheus rules or an error
func GetPrometheusRule(ruleFilePath string) (*monitoringv1.PrometheusRule, error) {
	ruleFile, err := ioutil.ReadFile(ruleFilePath)
	if err != nil {
		return nil, fmt.Errorf("prometheusRules file could not be fetched. %+v", err)
	}
	var rule monitoringv1.PrometheusRule
	err = k8sYAML.NewYAMLOrJSONDecoder(bytes.NewBufferString(string(ruleFile)), 1000).Decode(&rule)
	if err != nil {
		return nil, fmt.Errorf("prometheusRules could not be decoded. %+v", err)
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
		return nil, fmt.Errorf("failed to get monitoring client. %+v", err)
	}
	promRule, err := client.MonitoringV1().PrometheusRules(namespace).Create(prometheusRule)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create prometheusRules. %+v", err)
		}
		promRule, err = client.MonitoringV1().PrometheusRules(namespace).Update(prometheusRule)
		if err != nil {
			return nil, fmt.Errorf("failed to update prometheusRule. %+v", err)
		}
	}
	return promRule, nil
}
