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
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetValueStoreNotExist(t *testing.T) {
	kv, storeName := newKVStore()

	// try to get a value from a store that does not exist
	_, err := kv.GetValue(storeName, "key1")
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestGetValueKeyNotExist(t *testing.T) {
	// create a configmap (store) that has no keys in it
	cm := &v1.ConfigMap{Data: map[string]string{}}
	kv, storeName := newKVStore(cm)

	// try to get a value from a store that does exist but from a key that does not exist
	_, err := kv.GetValue(storeName, "key1")
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestGetValue(t *testing.T) {
	key := "key1"
	value := "value1"

	// create a configmap (store) that has a key value pair in it
	cm := &v1.ConfigMap{Data: map[string]string{key: value}}
	kv, storeName := newKVStore(cm)

	actualValue, err := kv.GetValue(storeName, key)
	assert.Nil(t, err)
	assert.Equal(t, value, actualValue)
}

func TestSetValueStoreNotExist(t *testing.T) {
	key := "key1"
	value := "value1"

	// start with no stores created at all
	kv, storeName := newKVStore()

	// try to set a value on a store that doesn't exist.  The store should be created automatically
	// and there should be no error.
	err := kv.SetValue(storeName, key, value)
	assert.Nil(t, err)

	// try to get the value that was set, it should be as expected
	actualValue, err := kv.GetValue(storeName, key)
	assert.Nil(t, err)
	assert.Equal(t, value, actualValue)

	// get a value that doesn't exist
	_, err = kv.GetValue(storeName, "key2")
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

}

func TestSetValueUpdate(t *testing.T) {
	key := "key1"
	value := "value1"

	// create a configmap (store) that has a key value pair in it
	cm := &v1.ConfigMap{Data: map[string]string{key: value}}
	kv, storeName := newKVStore(cm)

	// try to set the already existing key to a new value, which should update it
	newValue := "value2"
	err := kv.SetValue(storeName, key, newValue)
	assert.Nil(t, err)

	// try to get the key, this should return the updated value
	actualValue, err := kv.GetValue(storeName, key)
	assert.Nil(t, err)
	assert.Equal(t, newValue, actualValue)
}

func TestGetStoreNotExist(t *testing.T) {
	kv, storeName := newKVStore()

	_, err := kv.GetStore(storeName)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestGetStore(t *testing.T) {
	key := "key1"
	value := "value1"

	// create a configmap (store) that has a key value pair in it
	cm := &v1.ConfigMap{Data: map[string]string{key: value}}
	kv, storeName := newKVStore(cm)

	actualStore, err := kv.GetStore(storeName)
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{key: value}, actualStore)
}

func TestClearStoreNotExist(t *testing.T) {
	kv, storeName := newKVStore()

	// clearing a store that does not exist is OK, should be no error
	err := kv.ClearStore(storeName)
	assert.Nil(t, err)
}

func TestClearStore(t *testing.T) {
	key := "key1"
	value := "value1"

	// create a configmap (store) that has a key value pair in it
	cm := &v1.ConfigMap{Data: map[string]string{key: value}}
	kv, storeName := newKVStore(cm)

	// verify the store/key/value exist
	actualValue, err := kv.GetValue(storeName, key)
	assert.Nil(t, err)
	assert.Equal(t, value, actualValue)

	// now clear the store
	err = kv.ClearStore(storeName)
	assert.Nil(t, err)

	// getting the store should return an error for not exist
	_, err = kv.GetStore(storeName)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func newKVStore(stores ...*v1.ConfigMap) (*ConfigMapKVStore, string) {
	namespace := "kvstore_test"
	storeName := "store1"
	var objects []runtime.Object
	for _, cm := range stores {
		cm.Name = storeName
		cm.Namespace = namespace
		objects = append(objects, cm)
	}

	clientset := fake.NewSimpleClientset(objects...)
	ownerInfo := NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, "")
	return NewConfigMapKVStore(namespace, clientset, ownerInfo), storeName
}
