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

// Package nfs for the Edgefs manager.
package nfs

import (
	"fmt"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName = "rook-edgefs-nfs"

	/* Volumes definitions */
	serviceAccountName = "rook-edgefs-cluster"
	dataVolumeName     = "edgefs-datadir"
	stateVolumeFolder  = ".state"
	etcVolumeFolder    = ".etc"
)

// Start the rgw manager
func (c *NFSController) CreateService(s edgefsv1.NFS, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *NFSController) UpdateService(s edgefsv1.NFS, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the nfs instance
func (c *NFSController) CreateOrUpdate(s edgefsv1.NFS, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Infof("starting update=%v service=%s", update, s.Name)

	logger.Infof("NFS Base image is %s", c.rookImage)
	// validate NFS service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid NFS service %s arguments. %+v", s.Name, err)
	}

	// check if NFS service already exists
	exists, err := serviceExists(c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("NFS service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("NFS service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	// start the deployment
	deployment := c.makeDeployment(s.Name, s.Namespace, c.rookImage, s.Spec)
	if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Create(deployment); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}

		logger.Infof("%s deployment already exists", appName)
		if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Update(deployment); err != nil {
			return fmt.Errorf("failed to update %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment updated", appName)
	} else {
		logger.Infof("%s deployment started", appName)
	}

	// create the nfs service
	service := c.makeNFSService(instanceName(s.Name), s.Name, s.Namespace)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create nfs service. %+v", err)
		}
		logger.Infof("nfs service %s already exists", service)
	} else {
		logger.Infof("nfs service %s started", service)
	}

	return nil
}

func (c *NFSController) makeNFSService(name, svcname, namespace string) *v1.Service {
	labels := getLabels(name, svcname, namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{Name: "grpc", Port: 49000, Protocol: v1.ProtocolTCP},
				{Name: "nfs-tcp", Port: 2049, Protocol: v1.ProtocolTCP},
				{Name: "nfs-udp", Port: 2049, Protocol: v1.ProtocolUDP},
				{Name: "nlockmgr-tcp", Port: 32803, Protocol: v1.ProtocolTCP},
				{Name: "nlockmgr-udp", Port: 32803, Protocol: v1.ProtocolUDP},
				{Name: "mountd-tcp", Port: 20048, Protocol: v1.ProtocolTCP},
				{Name: "mountd-udp", Port: 20048, Protocol: v1.ProtocolUDP},
				{Name: "portmapper-tcp", Port: 111, Protocol: v1.ProtocolTCP},
				{Name: "portmapper-udp", Port: 111, Protocol: v1.ProtocolUDP},
				{Name: "statd-tcp", Port: 662, Protocol: v1.ProtocolTCP},
				{Name: "statd-udp", Port: 662, Protocol: v1.ProtocolUDP},
				{Name: "rquotad-tcp", Port: 875, Protocol: v1.ProtocolTCP},
				{Name: "rquotad-udp", Port: 875, Protocol: v1.ProtocolUDP},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *NFSController) makeDeployment(svcname, namespace, rookImage string, nfsSpec edgefsv1.NFSSpec) *apps.Deployment {
	name := instanceName(svcname)
	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
	}

	if c.dataVolumeSize.Value() > 0 {
		// dataVolume case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataVolumeName,
				},
			},
		})
	} else {
		// dataDir case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: c.dataDirHostPath,
				},
			},
		})
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: getLabels(name, svcname, namespace),
		},
		Spec: v1.PodSpec{
			Containers:         []v1.Container{c.nfsContainer(svcname, name, rookImage, nfsSpec)},
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            volumes,
			HostIPC:            true,
			HostNetwork:        c.NetworkSpec.IsHost(),
			NodeSelector:       map[string]string{namespace: "cluster"},
			ServiceAccountName: serviceAccountName,
		},
	}

	if c.NetworkSpec.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.NetworkSpec.IsMultus() {
		k8sutil.ApplyMultus(c.NetworkSpec, &podSpec.ObjectMeta)
	}
	nfsSpec.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)

	// apply current NFS CRD options to pod's specification
	nfsSpec.Placement.ApplyToPodSpec(&podSpec.Spec)

	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &nfsSpec.Instances,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	nfsSpec.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	return d
}

func (c *NFSController) nfsContainer(svcname, name, containerImage string, nfsSpec edgefsv1.NFSSpec) v1.Container {
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE", "IPC_LOCK"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc.target",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"nfs"},
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name:  "CCOW_SVCNAME",
				Value: svcname,
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name: "K8S_NAMESPACE",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       nfsSpec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
			{Name: "nfs-tcp", ContainerPort: 2049, Protocol: v1.ProtocolTCP},
			{Name: "nfs-udp", ContainerPort: 2049, Protocol: v1.ProtocolUDP},
			{Name: "nlockmgr-tcp", ContainerPort: 32803, Protocol: v1.ProtocolTCP},
			{Name: "nlockmgr-udp", ContainerPort: 32803, Protocol: v1.ProtocolUDP},
			{Name: "mountd-tcp", ContainerPort: 20048, Protocol: v1.ProtocolTCP},
			{Name: "mountd-udp", ContainerPort: 20048, Protocol: v1.ProtocolUDP},
			{Name: "portmapper-tcp", ContainerPort: 111, Protocol: v1.ProtocolTCP},
			{Name: "portmapper-udp", ContainerPort: 111, Protocol: v1.ProtocolUDP},
			{Name: "statd-tcp", ContainerPort: 662, Protocol: v1.ProtocolTCP},
			{Name: "statd-udp", ContainerPort: 662, Protocol: v1.ProtocolUDP},
			{Name: "rquotad-tcp", ContainerPort: 875, Protocol: v1.ProtocolTCP},
			{Name: "rquotad-udp", ContainerPort: 875, Protocol: v1.ProtocolUDP},
		},
		VolumeMounts: volumeMounts,
	}

	if nfsSpec.RelaxedDirUpdates {
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSNFS_RELAXED_DIR_UPDATES",
			Value: "1",
		})
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("nfs",
		c.resourceProfile == "embedded" || nfsSpec.ResourceProfile == "embedded",
		nfsSpec.ChunkCacheSize, nfsSpec.Resources)...)

	return cont
}

// Delete NFS service and possibly some artifacts.
func (c *NFSController) DeleteService(s edgefsv1.NFS) error {
	// check if service  exists
	exists, err := serviceExists(c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a NFS service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("NFS service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting NFS service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the nfs service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(instanceName(s.Name), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete NFS service. %+v", err)
	}

	// Make a best effort to delete the NFS pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting NFS service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "nfs",
	}
}

// Validate the NFS arguments
func validateService(context *clusterd.Context, s edgefsv1.NFS) error {
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	return nil
}

func instanceName(svcname string) string {
	return fmt.Sprintf("%s-%s", appName, svcname)
}

// Check if the NFS service exists
func serviceExists(context *clusterd.Context, s edgefsv1.NFS) (bool, error) {
	_, err := context.Clientset.AppsV1().Deployments(s.Namespace).Get(instanceName(s.Name), metav1.GetOptions{})
	if err == nil {
		// the deployment was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	// not found
	return false, nil
}
