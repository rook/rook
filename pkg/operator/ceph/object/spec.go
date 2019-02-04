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

// Package object for the Ceph object store.
package object

import (
	"fmt"
	"path"

	rgwdaemon "github.com/rook/rook/pkg/daemon/ceph/rgw"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *config) startDeployment() error {
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.store.Namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getLabels(),
			},
			Template: c.makeRGWPodSpec(),
			Replicas: &c.store.Spec.Gateway.Instances,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.SetOwnerRefs(c.context.Clientset, c.store.Namespace, &d.ObjectMeta, c.ownerRefs)

	logger.Debugf("starting mds deployment: %+v", d)
	_, err := c.context.Clientset.Apps().Deployments(c.store.Namespace).Create(d)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw deployment %s: %+v", c.instanceName(), err)
		}
		logger.Infof("deployment for rgw %s already exists. updating if needed", c.instanceName())
		// There may be a *lot* of rgws, and they are stateless, so don't bother waiting until the
		// entire deployment is updated to move on.
		_, err := c.context.Clientset.Apps().Deployments(c.store.Namespace).Update(d)
		if err != nil {
			return fmt.Errorf("failed to update rgw deployment %s. %+v", c.instanceName(), err)
		}
	}

	return nil
}

func (c *config) startDaemonset() error {
	d := &apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.store.Namespace,
		},
		Spec: apps.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getLabels(),
			},
			UpdateStrategy: apps.DaemonSetUpdateStrategy{
				Type: apps.RollingUpdateDaemonSetStrategyType,
			},
			Template: c.makeRGWPodSpec(),
		},
	}
	k8sutil.SetOwnerRefs(c.context.Clientset, c.store.Namespace, &d.ObjectMeta, c.ownerRefs)

	logger.Debugf("starting rgw daemonset: %+v", d)
	_, err := c.context.Clientset.Apps().DaemonSets(c.store.Namespace).Create(d)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw daemonset %s: %+v", c.instanceName(), err)
		}
		logger.Infof("daemonset for rgw %s already exists. updating if needed", c.instanceName())
		// There may be a *lot* of rgws, and they are stateless, so don't bother waiting until the
		// entire daemonset is updated to move on.
		// TODO: is the above statement safe to assume?
		// TODO: Are there any steps for RGW that need to happen before the daemons upgrade?
		_, err := c.context.Clientset.Apps().DaemonSets(c.store.Namespace).Update(d)
		if err != nil {
			return fmt.Errorf("failed to update rgw daemonset %s. %+v", c.instanceName(), err)
		}
	}

	return nil
}

func (c *config) makeRGWPodSpec() v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeConfigInitContainer(),
		},
		Containers: []v1.Container{
			c.makeDaemonContainer(),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes:       opspec.PodVolumes(""),
		HostNetwork:   c.hostNetwork,
	}
	if c.hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// Set the ssl cert if specified
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: c.store.Spec.Gateway.SSLCertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	c.store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        c.instanceName(),
			Labels:      c.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}
}

func (c *config) makeConfigInitContainer() v1.Container {
	container := v1.Container{
		Name:  opspec.ConfigInitContainerName,
		Image: k8sutil.MakeRookImage(c.rookVersion),
		Args: []string{
			"ceph",
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", c.store.Name),
			fmt.Sprintf("--rgw-port=%d", c.store.Spec.Gateway.Port),
			fmt.Sprintf("--rgw-secure-port=%d", c.store.Spec.Gateway.SecurePort),
		},
		VolumeMounts: opspec.RookVolumeMounts(),
		Env: []v1.EnvVar{
			{Name: "ROOK_RGW_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: c.instanceName()}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(c.store.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: c.store.Spec.Gateway.Resources,
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func (c *config) makeDaemonContainer() v1.Container {

	// start the rgw daemon in the foreground
	container := v1.Container{
		Name:  "rgw",
		Image: c.cephVersion.Image,
		Command: []string{
			"radosgw",
		},
		Args: []string{
			"--foreground",
			"--name=client.radosgw.gateway",
			fmt.Sprintf("--rgw-mime-types-file=%s", rgwdaemon.GetMimeTypesPath(k8sutil.DataDir)),
		},
		VolumeMounts: opspec.CephVolumeMounts(),
		Env:          k8sutil.ClusterDaemonEnvVars(),
		Resources:    c.store.Spec.Gateway.Resources,
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

func (c *config) startService() (string, error) {
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
	k8sutil.SetOwnerRefs(c.context.Clientset, c.store.Namespace, &svc.ObjectMeta, c.ownerRefs)
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

func (c *config) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.store.Namespace,
		"rook_object_store": c.store.Name,
	}
}
