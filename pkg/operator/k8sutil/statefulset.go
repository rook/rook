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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
)

// create a apps.statefulset
func CreateStatefulSet(name, namespace string, clientset kubernetes.Interface, ss *apps.StatefulSet) error {
	_, err := clientset.AppsV1().StatefulSets(namespace).Create(ss)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			_, err = clientset.AppsV1().StatefulSets(namespace).Update(ss)
		}
		if err != nil {
			return fmt.Errorf("failed to start %s statefulset: %+v\n%+v", name, err, ss)
		}
	}
	return nil
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
