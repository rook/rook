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

package object

import (
	"fmt"

	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *clusterConfig) startDeployment(rgwConfig *rgwConfig) (*apps.Deployment, error) {
	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    c.getLabels(),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getLabels(),
			},
			Template: c.makeRGWPodSpec(rgwConfig),
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)
	k8sutil.SetOwnerRefs(&d.ObjectMeta, c.ownerRefs)

	logger.Debugf("starting rgw deployment: %+v", d)
	deployment, err := c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Get(d.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to see if rgw deployment %s already exists. %+v", d.Name, err)
	} else if err == nil {
		// deployment exists
		var uErr error
		deployment, uErr = c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Update(d)
		if uErr != nil {
			// may fail to update when labels have changed on the deployment and thus the label selector
			// in this case we can try to delete the deployment and recreate
			dErr := c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Delete(d.Name, &metav1.DeleteOptions{})
			if dErr != nil {
				return nil, fmt.Errorf("failed to delete existing rgw deployment %s as part of update attempt. %+v", d.Name, dErr)
			}
		} else {
			return deployment, uErr
		}
	}
	// err != nil && isNotFound  or  err == nil && update failed, causing earlier dep to be deleted
	deployment, err = c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Create(d)
	if err != nil {
		return nil, fmt.Errorf("failed to create rgw deployment %s: %+v", c.instanceName(), err)
	}
	return deployment, err
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{},
		Containers: []v1.Container{
			c.makeDaemonContainer(rgwConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			opspec.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolume(),
		),
		HostNetwork: c.hostNetwork,
	}
	if c.hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// Set the ssl cert if specified
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Keep the SSL secret as secure as possible in the container. Give only user read perms.
		userReadOnly := int32(0400)
		certVol := v1.Volume{
			Name: certVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: c.store.Spec.Gateway.SSLCertificateRef,
					Items: []v1.KeyToPath{
						{Key: certKeyName, Path: certFilename, Mode: &userReadOnly},
					}}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}
	c.store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: c.getLabels(),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	return podTemplateSpec
}

func (c *clusterConfig) makeDaemonContainer(rgwConfig *rgwConfig) v1.Container {

	// start the rgw daemon in the foreground
	container := v1.Container{
		Name:  "rgw",
		Image: c.cephVersion.Image,
		Command: []string{
			"radosgw",
		},
		Args: append(
			append(
				opspec.DaemonFlags(c.clusterInfo, c.store.Name),
				"--foreground",
				cephconfig.NewFlag("name", generateCephXUser(rgwConfig.ResourceName)),
				cephconfig.NewFlag("host", opspec.ContainerEnvVarReference("POD_NAME")),
				cephconfig.NewFlag("rgw-mime-types-file", mimeTypesMountPath()),
			), c.defaultSettings().GlobalFlags()..., // use default settings as flags until mon kv store supported
		),
		VolumeMounts: append(
			opspec.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolumeMount(),
		),
		Env:       opspec.DaemonEnvVars(c.cephVersion.Image),
		Resources: c.store.Spec.Gateway.Resources,
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

func (c *clusterConfig) startService() (string, error) {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.store.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRefs(&svc.ObjectMeta, c.ownerRefs)
	if c.hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", c.store.Spec.Gateway.Port)
	addPort(svc, "https", c.store.Spec.Gateway.SecurePort)

	svc, err := c.context.Clientset.CoreV1().Services(c.store.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create rgw service. %+v", err)
		}
		svc, err = c.context.Clientset.CoreV1().Services(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get existing service IP. %+v", err)
		}
		return svc.Spec.ClusterIP, nil
	}

	logger.Infof("Gateway service running at %s:%d", svc.Spec.ClusterIP, c.store.Spec.Gateway.Port)
	return svc.Spec.ClusterIP, nil
}

func addPort(service *v1.Service, name string, port int32) {
	if port == 0 {
		return
	}
	service.Spec.Ports = append(service.Spec.Ports, v1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(port)),
		Protocol:   v1.ProtocolTCP,
	})
}

func (c *clusterConfig) getLabels() map[string]string {
	labels := opspec.PodLabels(AppName, c.store.Namespace, "rgw", c.store.Name)
	labels["rook_object_store"] = c.store.Name
	return labels
}
