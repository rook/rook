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
package client

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

type ConfigMapKVStore struct {
	namespace string
	clientset kubernetes.Interface
	ownerInfo *OwnerInfo
}

// NewConfigMapKVStore returns a new KV store
func NewConfigMapKVStore(namespace string, clientset kubernetes.Interface, ownerInfo *OwnerInfo) *ConfigMapKVStore {
	return &ConfigMapKVStore{
		namespace: namespace,
		clientset: clientset,
		ownerInfo: ownerInfo,
	}
}

// GetValue get a value from a KV store
func (kv *ConfigMapKVStore) GetValue(storeName, key string) (string, error) {
	ctx := context.TODO()
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(ctx, storeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	val, ok := cm.Data[key]
	if !ok {
		return "", k8serrors.NewNotFound(schema.GroupResource{}, key)
	}

	return val, nil
}

// SetValue set a value to a KV store
func (kv *ConfigMapKVStore) SetValue(storeName, key, value string) error {
	return kv.SetValueWithLabels(storeName, key, value, nil)
}

// SetValueWithLabels set a value with label to a KV store
func (kv *ConfigMapKVStore) SetValueWithLabels(storeName, key, value string, labels map[string]string) error {
	ctx := context.TODO()
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(ctx, storeName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
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
		err = kv.ownerInfo.SetOwnerReference(cm)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference of confimap %q", cm.Name)
		}

		_, err = kv.clientset.CoreV1().ConfigMaps(kv.namespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	// config map already exists, so update it with the given key/val
	cm.Data[key] = value

	_, err = kv.clientset.CoreV1().ConfigMaps(kv.namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

// GetStore get the store of a KV store
func (kv *ConfigMapKVStore) GetStore(storeName string) (map[string]string, error) {
	ctx := context.TODO()
	cm, err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Get(ctx, storeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return cm.Data, nil
}

// ClearStore clear the store of a KV store
func (kv *ConfigMapKVStore) ClearStore(storeName string) error {
	ctx := context.TODO()
	err := kv.clientset.CoreV1().ConfigMaps(kv.namespace).Delete(ctx, storeName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		// a real error, return it (we're OK with clearing a store that doesn't exist)
		return err
	}

	return nil
}
