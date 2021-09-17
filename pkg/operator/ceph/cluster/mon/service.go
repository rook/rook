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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Cluster) createService(mon *monConfig) (string, error) {
	svcDef := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mon.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(mon, false, true),
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "tcp-msgr1",
					Port: mon.Port,
					// --public-bind-addr=IP with no IP:port has the mon listen on port 6789
					// regardless of what port the mon advertises (--public-addr) to the outside.
					TargetPort: intstr.FromInt(int(DefaultMsgr1Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: c.getLabels(mon, false, false),
		},
	}
	err := c.ownerInfo.SetOwnerReference(svcDef)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to mon service %q", svcDef.Name)
	}

	// If deploying Nautilus or newer we need a new port for the monitor service
	addServicePort(svcDef, "tcp-msgr2", DefaultMsgr2Port)

	// Set the ClusterIP if the service does not exist and we expect a certain cluster IP
	// For example, in disaster recovery the service might have been deleted accidentally, but we have the
	// expected endpoint from the mon configmap.
	if mon.PublicIP != "" {
		_, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(c.ClusterInfo.Context, svcDef.Name, metav1.GetOptions{})
		if err != nil && kerrors.IsNotFound(err) {
			logger.Infof("ensuring the clusterIP for mon %q is %q", mon.DaemonName, mon.PublicIP)
			svcDef.Spec.ClusterIP = mon.PublicIP
		}
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
	logger.Infof("mon %q endpoint is [v2:%s:%s,v1:%s:%d]", mon.DaemonName, s.Spec.ClusterIP, strconv.Itoa(int(DefaultMsgr2Port)), s.Spec.ClusterIP, mon.Port)

	return s.Spec.ClusterIP, nil
}
