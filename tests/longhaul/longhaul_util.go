/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package longhaul

import (
	"math/rand"

	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// utility to install and prometheus and grafana to monitor rook cluster
var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "longhaul")
)

type ChaosHelper struct {
	namespace string
	kh        *utils.K8sHelper
}

func NewChaosHelper(namespace string, kh *utils.K8sHelper) *ChaosHelper {
	return &ChaosHelper{namespace: namespace, kh: kh}

}

func (h *ChaosHelper) Monkey() {
	time.Sleep(5 * time.Minute)
	for {
		logger.Info("Attempt to kill a Random Pod in namespace %s", h.namespace)
		listOpts := metav1.ListOptions{}
		pods, err := h.kh.Clientset.CoreV1().Pods(h.namespace).List(listOpts)
		if err != nil {
			logger.Infof("Error getting list of pods ,err ->%v", err)
			return
		}
		kp := pods.Items[rand.Intn(len(pods.Items)-1)]
		logger.Info("Killing pod with Name= %s", kp.Name)
		propagation := metav1.DeletePropagationForeground
		delOptions := &metav1.DeleteOptions{PropagationPolicy: &propagation}
		err = h.kh.Clientset.CoreV1().Pods(h.namespace).Delete(kp.Name, delOptions)
		if err != nil {
			logger.Infof("Cannot delete pod %s, err->%v", kp.Name, err)
			continue
		}
		logger.Infof("pod %s deleted", kp.Name)
		time.Sleep(5 * time.Minute)
	}
}
