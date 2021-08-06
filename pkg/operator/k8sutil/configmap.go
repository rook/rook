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

package k8sutil

import (
	"context"
	"fmt"
	"os"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DeleteConfigMap deletes a ConfigMap.
func DeleteConfigMap(clientset kubernetes.Interface, cmName, namespace string, opts *DeleteOptions) error {
	ctx := context.TODO()
	k8sOpts := BaseKubernetesDeleteOptions()
	delete := func() error { return clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, cmName, *k8sOpts) }
	verify := func() error {
		_, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, cmName, metav1.GetOptions{})
		return err
	}
	resource := fmt.Sprintf("ConfigMap %s", cmName)
	defaultWaitOptions := &WaitOptions{RetryCount: 20, RetryInterval: 2 * time.Second}
	return DeleteResource(delete, verify, resource, opts, defaultWaitOptions)
}

// GetOperatorSetting gets the operator setting from ConfigMap or Env Var
// returns defaultValue if setting is not found
func GetOperatorSetting(context context.Context, clientset kubernetes.Interface, configMapName, settingName, defaultValue string) (string, error) {
	// config must be in operator pod namespace
	namespace := os.Getenv(PodNamespaceEnvVar)
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context, configMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if settingValue, ok := os.LookupEnv(settingName); ok {
				logger.Infof("%s=%q (env var)", settingName, settingValue)
				return settingValue, nil
			}
			logger.Infof("%s=%q (default)", settingName, defaultValue)
			return defaultValue, nil
		}
		return defaultValue, fmt.Errorf("error reading ConfigMap %q. %v", configMapName, err)
	}

	return GetValue(cm.Data, settingName, defaultValue), nil
}

func GetValue(data map[string]string, settingName, defaultValue string) string {
	if settingValue, ok := data[settingName]; ok {
		logger.Infof("%s=%q (configmap)", settingName, settingValue)
		return settingValue
	} else if settingValue, ok := os.LookupEnv(settingName); ok {
		logger.Infof("%s=%q (env var)", settingName, settingValue)
		return settingValue
	}
	logger.Infof("%s=%q (default)", settingName, defaultValue)
	return defaultValue
}
