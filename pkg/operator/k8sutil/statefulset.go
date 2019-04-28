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
	"fmt"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// make a headless svc for statefulset
func makeHeadlessSvc(name, namespace string, label map[string]string, clientset kubernetes.Interface) (*corev1.Service, error) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    label,
		},
		Spec: corev1.ServiceSpec{
			Selector: label,
			Ports:    []corev1.ServicePort{{Name: "dummy", Port: 1234}},
		},
	}

	_, err := clientset.CoreV1().Services(namespace).Create(svc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create %s headless service. %+v", name, err)
	}
	return svc, nil
}

// create a apps.statefulset and a headless svc
func CreateStatefulSet(name, namespace, appName string, clientset kubernetes.Interface, ss *apps.StatefulSet) (*corev1.Service, error) {
	label := ss.GetLabels()
	svc, err := makeHeadlessSvc(appName, namespace, label, clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to start %s service: %v\n%v", name, err, ss)
	}

	_, err = clientset.AppsV1().StatefulSets(namespace).Create(ss)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			_, err = clientset.AppsV1().StatefulSets(namespace).Update(ss)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to start %s statefulset: %v\n%v", name, err, ss)
		}
	}
	return svc, err
}
