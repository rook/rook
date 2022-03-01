/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"

	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	configName = "csi-ceph-conf-override"
)

// checkCsiCephConfigMapExists checks if the csi-ceph-conf-override configmap
// exists in the given namespace.
func checkCsiCephConfigMapExists(ctx context.Context, clientset kubernetes.Interface, namespace string) (bool, error) {
	_, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return false, errors.Wrapf(err, "failed to get csi ceph.conf configmap %q (in %q)", configName, namespace)
		}
		return false, nil
	}

	logger.Infof("csi ceph.conf configmap %q exists", configName)
	return true, nil
}
