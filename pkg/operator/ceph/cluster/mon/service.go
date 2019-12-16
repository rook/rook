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
	"strconv"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Cluster) createService(mon *monConfig) (string, error) {
	labels := c.getLabels(mon.DaemonName)
	svcDef := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mon.ResourceName,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "msgr1",
					Port: mon.Port,
					// --public-bind-addr=IP with no IP:port has the mon listen on port 6789
					// regardless of what port the mon advertises (--public-addr) to the outside.
					TargetPort: intstr.FromInt(int(DefaultMsgr1Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRef(&svcDef.ObjectMeta, &c.ownerRef)

	// If deploying Nautilus or newer we need a new port for the monitor service
	if c.ClusterInfo.CephVersion.IsAtLeastNautilus() {
		addServicePort(svcDef, "msgr2", DefaultMsgr2Port)
	}

	s, err := k8sutil.CreateOrUpdateService(c.context.Clientset, c.Namespace, svcDef)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create service for mon %s", mon.DaemonName)
	}

	if s == nil {
		logger.Errorf("service ip not found for mon %q. if this is not a unit test, this is an error", mon.ResourceName)
		return "", nil
	}

	// mon endpoint are not actually like, they remain with the mgrs1 format
	// however it's interesting to show that monitors can be addressed via 2 different ports
	// in the end the service has msgr1 and msgr2 ports configured so it's not entirely wrong
	if c.ClusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Infof("mon %q endpoint are [v2:%s:%s,v1:%s:%d]", mon.DaemonName, s.Spec.ClusterIP, strconv.Itoa(int(DefaultMsgr2Port)), s.Spec.ClusterIP, mon.Port)
	} else {
		logger.Infof("mon %q endpoint is %s:%d", mon.DaemonName, s.Spec.ClusterIP, mon.Port)
	}
	return s.Spec.ClusterIP, nil
}
