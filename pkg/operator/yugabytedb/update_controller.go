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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *ClusterController) OnUpdate(oldObj, newObj interface{}) {
	// TODO Create the cluster if previous attempt to create has failed.
	_ = oldObj.(*yugabytedbv1alpha1.YBCluster).DeepCopy()
	newObjCluster := newObj.(*yugabytedbv1alpha1.YBCluster).DeepCopy()
	newYBCluster := NewCluster(newObjCluster, c.context)

	// Validate new spec
	if err := validateClusterSpec(newYBCluster.spec); err != nil {
		logger.Errorf("invalid cluster spec: %+v", err)
		return
	}

	// Update headless service ports
	if err := c.updateMasterHeadlessService(newYBCluster); err != nil {
		logger.Errorf("failed to update Master headless service: %+v", err)
		return
	}

	if err := c.updateTServerHeadlessService(newYBCluster); err != nil {
		logger.Errorf("failed to update TServer headless service: %+v", err)
		return
	}

	// Create/update/delete UI services (create/delete would apply for TServer UI services)
	if err := c.updateMasterUIService(newYBCluster); err != nil {
		logger.Errorf("failed to update Master UI service: %+v", err)
		return
	}

	if err := c.updateTServerUIService(newYBCluster); err != nil {
		logger.Errorf("failed to update TServer UI service: %+v", err)
		return
	}

	// Update StatefulSets replica count, command, ports & PVCs.
	if err := c.updateMasterStatefulset(newYBCluster); err != nil {
		logger.Errorf("failed to update Master statefulsets: %+v", err)
		return
	}

	if err := c.updateTServerStatefulset(newYBCluster); err != nil {
		logger.Errorf("failed to update TServer statefulsets: %+v", err)
		return
	}

	logger.Infof("cluster %s updated in namespace %s", newObjCluster.Name, newObjCluster.Namespace)
}

func (c *ClusterController) updateMasterHeadlessService(newCluster *cluster) error {
	return c.updateHeadlessService(newCluster, false)
}

func (c *ClusterController) updateTServerHeadlessService(newCluster *cluster) error {
	return c.updateHeadlessService(newCluster, true)
}

func (c *ClusterController) updateHeadlessService(newCluster *cluster, isTServerService bool) error {
	serviceName := masterNamePlural

	if isTServerService {
		serviceName = tserverNamePlural
	}

	serviceName = newCluster.addCRNameSuffix(serviceName)

	service, err := c.context.Clientset.CoreV1().Services(newCluster.namespace).Get(serviceName, metav1.GetOptions{})

	if err != nil {
		return err
	}

	service.Spec.Ports = createServicePorts(newCluster, isTServerService)

	if _, err := c.context.Clientset.CoreV1().Services(newCluster.namespace).Update(service); err != nil {
		return err
	}

	logger.Infof("headless service %s updated in namespace %s", service.Name, service.Namespace)

	return nil
}

func (c *ClusterController) updateMasterUIService(newCluster *cluster) error {
	return c.updateUIService(newCluster, false)
}

// Update/Delete UI service for TServer, if user has specified/removed a UI port for it.
func (c *ClusterController) updateTServerUIService(newCluster *cluster) error {
	return c.updateUIService(newCluster, true)
}

func (c *ClusterController) updateUIService(newCluster *cluster, isTServerService bool) error {
	ports, err := getPortsFromSpec(newCluster.spec.Master.Network)
	if err != nil {
		return err
	}

	serviceName := masterUIServiceName

	if isTServerService {
		ports, err = getPortsFromSpec(newCluster.spec.TServer.Network)
		if err != nil {
			return err
		}

		serviceName = tserverUIServiceName
	}

	serviceName = newCluster.addCRNameSuffix(serviceName)

	service, err := c.context.Clientset.CoreV1().Services(newCluster.namespace).Get(serviceName, metav1.GetOptions{})

	if err != nil {
		// Create TServer UI Service, if it wasn't present & new spec needs one.
		if errors.IsNotFound(err) && isTServerService {
			// The below condition is not clubbed with other two above, so as to
			// report the error if any of above conditions is false; irrespective of TServer UI port value in the new spec.
			if ports.tserverPorts.ui > 0 {
				return c.createTServerUIService(newCluster)
			}

			// Return if TServer UI service did not exist previously & new spec also doesn't need one to be created.
			return nil
		}

		return err
	}

	// Delete the TServer UI service if existed, but new spec doesn't need one.
	if isTServerService && service != nil && ports.tserverPorts.ui <= 0 {
		if err := c.context.Clientset.CoreV1().Services(newCluster.namespace).Delete(serviceName, &metav1.DeleteOptions{}); err != nil {
			return err
		}

		logger.Infof("UI service %s deleted in namespace %s", service.Name, service.Namespace)

		return nil
	}

	// Update the UI service for Master or TServer, otherwise.
	service.Spec.Ports = createUIServicePorts(ports, isTServerService)

	if _, err := c.context.Clientset.CoreV1().Services(newCluster.namespace).Update(service); err != nil {
		return err
	}

	logger.Infof("client service %s updated in namespace %s", service.Name, service.Namespace)

	return nil
}

func (c *ClusterController) updateMasterStatefulset(newCluster *cluster) error {
	return c.updateStatefulSet(newCluster, false)
}

func (c *ClusterController) updateTServerStatefulset(newCluster *cluster) error {
	return c.updateStatefulSet(newCluster, true)
}

func (c *ClusterController) updateStatefulSet(newCluster *cluster, isTServerStatefulset bool) error {
	ports, err := getPortsFromSpec(newCluster.spec.Master.Network)

	if err != nil {
		return err
	}

	replicas := int32(newCluster.spec.Master.Replicas)
	sfsName := newCluster.addCRNameSuffix(masterName)
	masterServiceName := newCluster.addCRNameSuffix(masterNamePlural)
	masterCompleteName := newCluster.addCRNameSuffix(masterName)
	vct := *newCluster.spec.Master.VolumeClaimTemplate.DeepCopy()
	vct.Name = newCluster.addCRNameSuffix(vct.Name)
	volumeClaimTemplates := []v1.PersistentVolumeClaim{vct}
	command := createMasterContainerCommand(newCluster.namespace, masterServiceName, masterCompleteName, ports.masterPorts.rpc, newCluster.spec.Master.Replicas)
	containerPorts := createMasterContainerPortsList(ports)

	if isTServerStatefulset {
		masterRPCPort := ports.masterPorts.rpc
		ports, err := getPortsFromSpec(newCluster.spec.TServer.Network)

		if err != nil {
			return err
		}

		replicas = int32(newCluster.spec.TServer.Replicas)
		sfsName = newCluster.addCRNameSuffix(tserverName)
		masterServiceName = newCluster.addCRNameSuffix(masterNamePlural)
		masterCompleteName := newCluster.addCRNameSuffix(masterName)
		tserverServiceName := newCluster.addCRNameSuffix(tserverNamePlural)
		vct = *newCluster.spec.TServer.VolumeClaimTemplate.DeepCopy()
		vct.Name = newCluster.addCRNameSuffix(vct.Name)
		volumeClaimTemplates = []v1.PersistentVolumeClaim{vct}
		command = createTServerContainerCommand(newCluster.namespace, tserverServiceName, masterServiceName, masterCompleteName,
			masterRPCPort, ports.tserverPorts.rpc, ports.tserverPorts.postgres, newCluster.spec.TServer.Replicas)
		containerPorts = createTServerContainerPortsList(ports)
	}

	sfs, err := c.context.Clientset.AppsV1().StatefulSets(newCluster.namespace).Get(sfsName, metav1.GetOptions{})

	if err != nil {
		return err
	}

	sfs.Spec.Replicas = &replicas
	sfs.Spec.Template.Spec.Containers[0].Command = command
	sfs.Spec.Template.Spec.Containers[0].Ports = containerPorts
	sfs.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name = vct.Name
	sfs.Spec.VolumeClaimTemplates = volumeClaimTemplates

	if _, err := c.context.Clientset.AppsV1().StatefulSets(newCluster.namespace).Update(sfs); err != nil {
		return err
	}

	logger.Infof("stateful set %s updated in namespace %s", sfs.Name, sfs.Namespace)

	return nil
}
