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

	apps "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// create a apps.statefulset
func CreateStatefulSet(clientset kubernetes.Interface, name, namespace string, ss *apps.StatefulSet) error {
	ctx := context.TODO()
	_, err := clientset.AppsV1().StatefulSets(namespace).Create(ctx, ss, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			_, err = clientset.AppsV1().StatefulSets(namespace).Update(ctx, ss, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("failed to start %s statefulset: %+v\n%+v", name, err, ss)
		}
	}
	return nil
}

// DeleteStatefulset makes a best effort at deleting a statefulset and its pods, then waits for them to be deleted
func DeleteStatefulset(clientset kubernetes.Interface, namespace, name string) error {
	ctx := context.TODO()
	logger.Debugf("removing %s statefulset if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.AppsV1().StatefulSets(namespace).Delete(ctx, name, *options)
	}
	getAction := func() error {
		_, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "statefulset", deleteAction, getAction)
}

// AddRookVersionLabelToStatefulSet adds or updates a label reporting the Rook version which last
// modified a apps.statefulset.
func AddRookVersionLabelToStatefulSet(ss *apps.StatefulSet) {
	if ss == nil {
		return
	}
	if ss.Labels == nil {
		ss.Labels = map[string]string{}
	}
	addRookVersionLabel(ss.Labels)
}
