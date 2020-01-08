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

package yugabytedb

import (
	yugabytedbv1alpha1 "github.com/rook/rook/pkg/apis/yugabytedb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *ClusterController) OnAdd(obj interface{}) {
	clusterObj := obj.(*yugabytedbv1alpha1.YBCluster).DeepCopy()
	logger.Infof("new cluster %s added to namespace %s", clusterObj.Name, clusterObj.Namespace)

	cluster := NewCluster(clusterObj, c.context)

	if err := validateClusterSpec(cluster.spec); err != nil {
		logger.Errorf("invalid cluster spec: %+v", err)
		return
	}

	if err := c.createMasterHeadlessService(cluster); err != nil {
		logger.Errorf("failed to create master headless service: %+v", err)
		return
	}

	if err := c.createTServerHeadlessService(cluster); err != nil {
		logger.Errorf("failed to create TServer headless service: %+v", err)
		return
	}

	if err := c.createMasterUIService(cluster); err != nil {
		logger.Errorf("failed to create Master UI service: %+v", err)
		return
	}

	if err := c.createTServerUIService(cluster); err != nil {
		logger.Errorf("failed to create replica service: %+v", err)
		return
	}

	if err := c.createMasterStatefulset(cluster); err != nil {
		logger.Errorf("failed to create master stateful set: %+v", err)
		return
	}

	if err := c.createTServerStatefulset(cluster); err != nil {
		logger.Errorf("failed to create tserver stateful set: %+v", err)
		return
	}

	logger.Infof("succeeded creating and initializing cluster in namespace %s", cluster.namespace)
}

func (c *ClusterController) createMasterUIService(cluster *cluster) error {
	return c.createUIService(cluster, false)
}

// Create UI service for TServer, if user has specified a UI port for it. Do not create it implicitly, with default port.
func (c *ClusterController) createTServerUIService(cluster *cluster) error {
	return c.createUIService(cluster, true)
}

func (c *ClusterController) createUIService(cluster *cluster, isTServerService bool) error {
	ports, err := getPortsFromSpec(cluster.spec.Master.Network)
	if err != nil {
		return err
	}

	serviceName := masterUIServiceName
	label := masterName

	if isTServerService {
		ports, err = getPortsFromSpec(cluster.spec.TServer.Network)
		if err != nil {
			return err
		}
		// If user hasn't specified TServer UI port, do not create a UI service for it.
		if ports.tserverPorts.ui <= 0 {
			return nil
		}

		serviceName = tserverUIServiceName
		label = tserverName
	}

	// Append CR name suffix to make the service name & label unique in current namespace.
	serviceName = cluster.addCRNameSuffix(serviceName)
	label = cluster.addCRNameSuffix(label)

	// This service is meant to be used by clients of the database. It exposes a ClusterIP that will
	// automatically load balance connections to the different database pods.
	uiService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: cluster.namespace,
			Labels:    createAppLabels(label),
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(label),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    createUIServicePorts(ports, isTServerService),
		},
	}
	k8sutil.SetOwnerRef(&uiService.ObjectMeta, &cluster.ownerRef)

	if _, err := c.context.Clientset.CoreV1().Services(cluster.namespace).Create(uiService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("client service %s already exists in namespace %s", uiService.Name, uiService.Namespace)
	} else {
		logger.Infof("client service %s started in namespace %s", uiService.Name, uiService.Namespace)
	}

	return nil
}

func (c *ClusterController) createMasterHeadlessService(cluster *cluster) error {
	return c.createHeadlessService(cluster, false)
}

func (c *ClusterController) createTServerHeadlessService(cluster *cluster) error {
	return c.createHeadlessService(cluster, true)
}

func (c *ClusterController) createHeadlessService(cluster *cluster, isTServerService bool) error {
	serviceName := masterNamePlural
	label := masterName

	if isTServerService {
		serviceName = tserverNamePlural
		label = tserverName
	}

	// Append CR name suffix to make the service name & label unique in current namespace.
	serviceName = cluster.addCRNameSuffix(serviceName)
	label = cluster.addCRNameSuffix(label)

	// This service only exists to create DNS entries for each pod in the stateful
	// set such that they can resolve each other's IP addresses. It does not
	// create a load-balanced ClusterIP and should not be used directly by clients
	// in most circumstances.
	headlessService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: cluster.namespace,
			Labels:    createAppLabels(label),
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(label),
			// We want all pods in the StatefulSet to have their addresses published for
			// the sake of the other YugabyteDB pods even before they're ready, since they
			// have to be able to talk to each other in order to become ready.
			ClusterIP: "None",
			Ports:     createServicePorts(cluster, isTServerService),
		},
	}

	k8sutil.SetOwnerRef(&headlessService.ObjectMeta, &cluster.ownerRef)

	if _, err := c.context.Clientset.CoreV1().Services(cluster.namespace).Create(headlessService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("headless service %s already exists in namespace %s", headlessService.Name, headlessService.Namespace)
	} else {
		logger.Infof("headless service %s started in namespace %s", headlessService.Name, headlessService.Namespace)
	}

	return nil
}

func (c *ClusterController) createMasterStatefulset(cluster *cluster) error {
	return c.createStatefulSet(cluster, false)
}

func (c *ClusterController) createTServerStatefulset(cluster *cluster) error {
	return c.createStatefulSet(cluster, true)
}

func (c *ClusterController) createStatefulSet(cluster *cluster, isTServerStatefulset bool) error {
	replicas := int32(cluster.spec.Master.Replicas)
	name := masterName
	label := masterName
	serviceName := masterNamePlural
	volumeClaimTemplates := []v1.PersistentVolumeClaim{
		cluster.spec.Master.VolumeClaimTemplate,
	}

	if isTServerStatefulset {
		replicas = int32(cluster.spec.TServer.Replicas)
		name = tserverName
		label = tserverName
		serviceName = tserverNamePlural
		volumeClaimTemplates = []v1.PersistentVolumeClaim{
			cluster.spec.TServer.VolumeClaimTemplate,
		}
	}

	// Append CR name suffix to make the service name & label unique in current namespace.
	name = cluster.addCRNameSuffix(name)
	label = cluster.addCRNameSuffix(label)
	serviceName = cluster.addCRNameSuffix(serviceName)
	volumeClaimTemplates[0].Name = cluster.addCRNameSuffix(volumeClaimTemplates[0].Name)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.namespace,
			Labels:    createAppLabels(label),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:         serviceName,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Replicas:            &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: createAppLabels(label),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.namespace,
					Labels:    createAppLabels(label),
				},
				Spec: createPodSpec(cluster, c.containerImage, isTServerStatefulset, name, serviceName),
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
	cluster.annotations.ApplyToObjectMeta(&statefulSet.Spec.Template.ObjectMeta)
	cluster.annotations.ApplyToObjectMeta(&statefulSet.ObjectMeta)
	k8sutil.SetOwnerRef(&statefulSet.ObjectMeta, &cluster.ownerRef)

	if _, err := c.context.Clientset.AppsV1().StatefulSets(cluster.namespace).Create(statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}

	return nil
}

func createPodSpec(cluster *cluster, containerImage string, isTServerStatefulset bool, name, serviceName string) v1.PodSpec {
	return v1.PodSpec{
		Affinity: &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
					{
						Weight: int32(100),
						PodAffinityTerm: v1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      k8sutil.AppAttr,
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{name},
									},
								},
							},
							TopologyKey: v1.LabelHostname,
						},
					},
				},
			},
		},
		Containers: []v1.Container{createContainer(cluster, containerImage, isTServerStatefulset, name, serviceName)},
	}
}

func createContainer(cluster *cluster, containerImage string, isTServerStatefulset bool, name, serviceName string) v1.Container {
	ports, _ := getPortsFromSpec(cluster.spec.Master.Network)
	masterCompleteName := cluster.addCRNameSuffix(masterName)
	command := createMasterContainerCommand(cluster.namespace, serviceName, masterCompleteName, ports.masterPorts.rpc, cluster.spec.Master.Replicas)
	containerPorts := createMasterContainerPortsList(ports)
	volumeMountName := cluster.addCRNameSuffix(cluster.spec.Master.VolumeClaimTemplate.Name)

	if isTServerStatefulset {
		masterServiceName := cluster.addCRNameSuffix(masterNamePlural)
		masterCompleteName := cluster.addCRNameSuffix(masterName)
		masterRPCPort := ports.masterPorts.rpc
		ports, _ = getPortsFromSpec(cluster.spec.TServer.Network)
		command = createTServerContainerCommand(cluster.namespace, serviceName, masterServiceName, masterCompleteName, masterRPCPort, ports.tserverPorts.rpc, ports.tserverPorts.postgres, cluster.spec.TServer.Replicas)
		containerPorts = createTServerContainerPortsList(ports)
		volumeMountName = cluster.addCRNameSuffix(cluster.spec.TServer.VolumeClaimTemplate.Name)
	}

	return v1.Container{
		Name:            name,
		Image:           yugabyteDBImageName,
		ImagePullPolicy: v1.PullAlways,
		Env: []v1.EnvVar{
			{
				Name:  envGetHostsFrom,
				Value: envGetHostsFromVal,
			},
			{
				Name: envPodIP,
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: envPodIPVal,
					},
				},
			},
			{
				Name: envPodName,
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: envPodNameVal,
					},
				},
			},
		},
		Command: command,
		Ports:   containerPorts,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      volumeMountName,
				MountPath: volumeMountPath,
			},
		},
	}
}
