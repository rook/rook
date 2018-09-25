/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

type ConfigMapKVStore struct {
	namespace string
	clientset kubernetes.Interface
	ownerRef  metav1.OwnerReference
}

func NewConfigMapKVStore(namespace string, clientset kubernetes.Interface, ownerRef metav1.OwnerReference) *ConfigMapKVStore {
	return &ConfigMapKVStore{
		namespace: namespace,
		clientset: clientset,
		ownerRef:  ownerRef,
	}
}

func (kv *ConfigMapKVStore) GetValue(storeName, key string) (string, error) {
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(storeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	val, ok := cm.Data[key]
	if !ok {
		return "", errors.NewNotFound(schema.GroupResource{}, key)
	}

	return val, nil
}

func (kv *ConfigMapKVStore) SetValue(storeName, key, value string) error {
	return kv.SetValueWithLabels(storeName, key, value, nil)
}

func (kv *ConfigMapKVStore) SetValueWithLabels(storeName, key, value string, labels map[string]string) error {
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(storeName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// the given config map doesn't exist yet, create it now with the given key/val
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storeName,
				Namespace: kv.namespace,
			},
			Data: map[string]string{key: value},
		}
		if labels != nil {
			cm.Labels = labels
		}
		SetOwnerRef(kv.clientset, kv.namespace, &cm.ObjectMeta, &kv.ownerRef)

		_, err = kv.clientset.CoreV1().ConfigMaps(kv.namespace).Create(cm)
		return err
	}

	// config map already exists, so update it with the given key/val
	cm.Data[key] = value

	_, err = kv.clientset.CoreV1().ConfigMaps(kv.namespace).Update(cm)
	if err != nil {
		return err
	}

	return nil
}

func (kv *ConfigMapKVStore) GetStore(storeName string) (map[string]string, error) {
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(storeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return cm.Data, nil
}

func (kv *ConfigMapKVStore) ClearStore(storeName string) error {
	err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Delete(storeName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		// a real error, return it (we're OK with clearing a store that doesn't exist)
		return err
	}

	return nil
}
