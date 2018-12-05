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

package controller

import (
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/controller/endpoint"
	"strings"
)

// SyncClusterHeadlessService checks if a Headless Service exists
// for the given Cluster, in order for the StatefulSets to utilize it.
// If it doesn't exists, then create it.
func (cc *ClusterController) syncClusterHeadlessService(c *cassandrav1alpha1.Cluster) error {
	clusterHeadlessService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            util.HeadlessServiceNameForCluster(c),
			Namespace:       c.Namespace,
			Labels:          util.ClusterLabels(c),
			OwnerReferences: []metav1.OwnerReference{util.NewControllerRef(c)},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Type:      corev1.ServiceTypeClusterIP,
			Selector:  util.ClusterLabels(c),
			// Necessary to specify a Port to work correctly
			// https://github.com/kubernetes/kubernetes/issues/32796
			// TODO: find in what version this was fixed
			Ports: []corev1.ServicePort{
				{
					Name: "prometheus",
					Port: 9180,
				},
			},
		},
	}

	logger.Infof("Syncing ClusterHeadlessService `%s` for Cluster `%s`", clusterHeadlessService.Name, c.Name)

	return cc.syncService(clusterHeadlessService, c)
}

// SyncMemberServices checks, for every Pod of the Cluster that
// has been created, if a corresponding ClusterIP Service exists,
// which will serve as a static ip.
// If it doesn't exist, it creates it.
// It also assigns the first two members of each rack as seeds.
func (cc *ClusterController) syncMemberServices(c *cassandrav1alpha1.Cluster) error {

	pods, err := util.GetPodsForCluster(c, cc.podLister)
	if err != nil {
		return err
	}

	// For every Pod of the cluster that exists, check that a
	// a corresponding ClusterIP Service exists, and if it doesn't,
	// create it.
	logger.Infof("Syncing MemberServices for Cluster `%s`", c.Name)
	for _, pod := range pods {
		if err := cc.syncService(memberServiceForPod(pod, c), c); err != nil {
			logger.Errorf("Error syncing member service for '%s'", pod.Name)
			return err
		}
	}
	return nil
}

// syncService checks if the given Service exists and creates it if it doesn't
// it creates it
func (cc *ClusterController) syncService(s *corev1.Service, c *cassandrav1alpha1.Cluster) error {

	existingService, err := cc.serviceLister.Services(s.Namespace).Get(s.Name)
	// If we get an error but without the IsNotFound error raised
	// then something is wrong with the network, so requeue.
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	// If the service already exists, check that it's
	// controlled by the given Cluster
	if err == nil {
		return util.VerifyOwner(existingService, c)
	}

	// At this point, the Service doesn't exist, so we are free to create it
	_, err = cc.kubeClient.CoreV1().Services(s.Namespace).Create(s)
	return err

}

func memberServiceForPod(pod *corev1.Pod, cluster *cassandrav1alpha1.Cluster) *corev1.Service {

	labels := util.ClusterLabels(cluster)
	labels[constants.DatacenterNameLabel] = pod.Labels[constants.DatacenterNameLabel]
	labels[constants.RackNameLabel] = pod.Labels[constants.RackNameLabel]
	// If Member is seed, add the appropriate label
	if strings.HasSuffix(pod.Name, "-0") || strings.HasSuffix(pod.Name, "-1") {
		labels[constants.SeedLabel] = ""
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			OwnerReferences: []metav1.OwnerReference{util.NewControllerRef(cluster)},
			Labels:          labels,
			Annotations:     map[string]string{endpoint.TolerateUnreadyEndpointsAnnotation: "true"},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: util.StatefulSetPodLabel(pod.Name),
			Ports: []corev1.ServicePort{
				{
					Name: "inter-node-communication",
					Port: 7000,
				},
				{
					Name: "ssl-inter-node-communication",
					Port: 7001,
				},
				{
					Name: "jmx-monitoring",
					Port: 7199,
				},
				{
					Name: "cql",
					Port: 9042,
				},
				{
					Name: "thrift",
					Port: 9160,
				},
				{
					Name: "cql-ssl",
					Port: 9142,
				},
			},
			PublishNotReadyAddresses: true,
		},
	}
}
