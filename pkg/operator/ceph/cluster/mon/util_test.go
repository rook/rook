/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package mon

import (
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	monValueFrom = "monitors"
)

func createFakeSecret(name, ns string, clientset *fake.Clientset) error {
	data := make(map[string][]byte, 1)
	data[monValueFrom] = []byte("mon1:6790,mon2:6790")
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: data,
	}
	_, err := clientset.CoreV1().Secrets(ns).Create(s)
	return err
}

func getFakeSecretData(name, ns, key string, clientset *fake.Clientset) (string, error) {
	s, err := clientset.CoreV1().Secrets(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	data, ok := s.Data[key]
	if !ok {
		return "", nil
	}

	return string(data), nil
}

func createFakeStorageClass(name, clusterName, secretName, secretNS string, clientset *fake.Clientset) error {
	labels := map[string]string{
		csiSCLabelSelector: clusterName,
	}
	param := map[string]string{
		csiSCSecretParamName:   secretName,
		csiSCSecretNSParamName: secretNS,
		csiSCMonParamName:      monValueFrom,
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Provisioner: "csi-rbdplugin",
		Parameters:  param,
	}
	_, err := clientset.Storage().StorageClasses().Create(sc)
	return err
}

func TestUpdateSC(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	err := createFakeSecret("fakeSecret", "fakeNS", clientset)
	assert.Nil(t, err)
	err = createFakeStorageClass("fakeSC", "fakeCluster", "fakeSecret", "fakeNS", clientset)
	assert.Nil(t, err)
	context := &clusterd.Context{Clientset: clientset}
	mon1 := &monConfig{
		PublicIP: "1.2.3.4",
		Port:     6790,
	}
	mon2 := &monConfig{
		PublicIP: "5.6.7.8",
		Port:     6790,
	}

	newMons := []*monConfig{mon1, mon2}
	monAddr := monConfigString(newMons)
	err = updateMonValuesForSC(context, "fakeCluster", monAddr)
	assert.Nil(t, err)
	val, err := getFakeSecretData("fakeSecret", "fakeNS", monValueFrom, clientset)
	assert.Nil(t, err)
	assert.True(t, strings.Contains(val, "1.2.3.4:6790"))
	assert.True(t, strings.Contains(val, "5.6.7.8:6790"))
}
