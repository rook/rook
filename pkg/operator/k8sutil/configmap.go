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
	"sync"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var loggedOperatorSettings = sync.Map{}

// DeleteConfigMap deletes a ConfigMap.
func DeleteConfigMap(ctx context.Context, clientset kubernetes.Interface, cmName, namespace string, opts *DeleteOptions) error {
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

func CreateOrUpdateConfigMap(ctx context.Context, clientset kubernetes.Interface, cm *v1.ConfigMap) (*v1.ConfigMap, error) {
	name := cm.GetName()
	namespace := cm.GetNamespace()
	existingCm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			cm, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create %q configmap", name)
			}
			return cm, nil
		}

		return nil, errors.Wrapf(err, "failed to retrieve %q configmap.", name)
	}

	existingCm.Data = cm.Data
	existingCm.OwnerReferences = cm.OwnerReferences
	if existingCm, err := clientset.CoreV1().ConfigMaps(namespace).Update(ctx, existingCm, metav1.UpdateOptions{}); err != nil {
		return nil, errors.Wrapf(err, "failed to update existing %q configmap", existingCm.Name)
	}

	return existingCm, nil
}

// GetOperatorSetting gets the operator setting from Env Var merged with ConfigMap
// returns defaultValue if setting is not found
func GetOperatorSetting(settingName, defaultValue string) string {
	if settingValue, ok := os.LookupEnv(settingName); ok {
		return settingValue
	}
	return defaultValue
}

func GetValue(data map[string]string, settingName, defaultValue string) string {
	settingValue := defaultValue

	if val, ok := os.LookupEnv(settingName); ok {
		settingValue = val
	}

	return settingValue
}
