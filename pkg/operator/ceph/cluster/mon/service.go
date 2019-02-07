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

package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Cluster) createService(mon *monConfig) (string, error) {
	labels := c.getLabels(mon.DaemonName)
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mon.ResourceName,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       mon.ResourceName,
					Port:       mon.Port,
					TargetPort: intstr.FromInt(int(mon.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &s.ObjectMeta, &c.ownerRef)
	if c.HostNetwork {
		s.Spec.ClusterIP = v1.ClusterIPNone
	}

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(s)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create mon service. %+v", err)
		}
		s, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get(mon.ResourceName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get mon %s service ip. %+v", mon.ResourceName, err)
		}
	}

	if s == nil {
		logger.Warningf("service ip not found for mon %s. this better be a test", mon.ResourceName)
		return "", nil
	}

	logger.Infof("mon %s endpoint is %s:%d", mon.DaemonName, s.Spec.ClusterIP, mon.Port)
	return s.Spec.ClusterIP, nil
}
