/*
Copyright 2021 The Rook Authors. All rights reserved.

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
package multus

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	ifBase = "mlink"
)

func DetermineNewLinkName(interfaces []net.Interface) (string, error) {
	var newLinkName string

	linkNumber := -1
	for _, iface := range interfaces {
		if idStrs := strings.Split(iface.Name, ifBase); len(idStrs) > 1 {
			id, err := strconv.Atoi(idStrs[1])
			if err != nil {
				return newLinkName, errors.Wrap(err, "failed to convert string to integer")
			}
			if id > linkNumber {
				linkNumber = id
			}
		}
	}
	linkNumber += 1

	newLinkName = fmt.Sprintf("%s%d", ifBase, linkNumber)
	fmt.Printf("new multus link name determined: %q\n", newLinkName)

	return newLinkName, nil
}

func AnnotateController(k8sClient *kubernetes.Clientset, controllerName, controllerNamespace, migratedLinkName string) error {
	pod, err := k8sClient.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get controller pod")
	}

	pod.ObjectMeta.Annotations["multus-migration"] = migratedLinkName

	_, err = k8sClient.CoreV1().Pods(controllerNamespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update controller pod")
	}

	return nil
}
