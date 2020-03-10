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
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) *apps.Deployment {
	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    getLabels(c.store.Name, c.store.Namespace),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(c.store.Name, c.store.Namespace),
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
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)

	return d
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(rgwConfig),
		},
		Containers: []v1.Container{
			c.makeDaemonContainer(rgwConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			controller.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolume(),
		),
		HostNetwork:       c.clusterSpec.Network.IsHost(),
		PriorityClassName: c.store.Spec.Gateway.PriorityClassName,
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec)

	if c.clusterSpec.Network.IsHost() {
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
	preferredDuringScheduling := false
	k8sutil.SetNodeAntiAffinityForPod(&podSpec, c.store.Spec.Gateway.Placement, c.clusterSpec.Network.IsHost(), preferredDuringScheduling, getLabels(c.store.Name, c.store.Namespace),
		nil)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: getLabels(c.store.Name, c.store.Namespace),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	return podTemplateSpec
}

func (c *clusterConfig) makeChownInitContainer(rgwConfig *rgwConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*c.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
		c.store.Spec.Gateway.Resources,
		mon.PodSecurityContext(),
	)
}

func (c *clusterConfig) makeDaemonContainer(rgwConfig *rgwConfig) v1.Container {
	// start the rgw daemon in the foreground
	container := v1.Container{
		Name:  "rgw",
		Image: c.clusterSpec.CephVersion.Image,
		Command: []string{
			"radosgw",
		},
		Args: append(
			append(
				controller.DaemonFlags(c.clusterInfo, strings.TrimPrefix(generateCephXUser(rgwConfig.ResourceName), "client.")),
				"--foreground",
				cephconfig.NewFlag("rgw frontends", fmt.Sprintf("%s %s", rgwFrontendName, c.portString())),
				cephconfig.NewFlag("host", controller.ContainerEnvVarReference("POD_NAME")),
				cephconfig.NewFlag("rgw-mime-types-file", mimeTypesMountPath()),
			),
		),
		VolumeMounts: append(
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolumeMount(),
		),
		Env:       controller.DaemonEnvVars(c.clusterSpec.CephVersion.Image),
		Resources: c.store.Spec.Gateway.Resources,
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/swift/healthcheck",
					Port: intstr.FromInt(int(c.store.Spec.Gateway.Port)),
				},
			},
			InitialDelaySeconds: 10,
		},
		SecurityContext: mon.PodSecurityContext(),
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

func (r *ReconcileCephObjectStore) generateService(cephObjectStore *cephv1.CephObjectStore) *v1.Service {
	labels := getLabels(cephObjectStore.Name, cephObjectStore.Namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}

	if r.cephClusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", cephObjectStore.Spec.Gateway.Port)
	addPort(svc, "https", cephObjectStore.Spec.Gateway.SecurePort)

	return svc
}

func (r *ReconcileCephObjectStore) reconcileService(cephObjectStore *cephv1.CephObjectStore) (string, error) {
	service := r.generateService(cephObjectStore)
	// Set owner ref to the parent object
	err := controllerutil.SetControllerReference(cephObjectStore, service, r.scheme)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to ceph object store")
	}

	svc, err := r.context.Clientset.CoreV1().Services(cephObjectStore.Namespace).Create(service)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return "", errors.Wrapf(err, "failed to create ceph object store service")
		}
		svc, err = r.context.Clientset.CoreV1().Services(cephObjectStore.Namespace).Get(instanceName(cephObjectStore.Name), metav1.GetOptions{})
		if err != nil {
			return "", errors.Wrapf(err, "failed to get existing service IP")
		}
		return svc.Spec.ClusterIP, nil
	}

	logger.Infof("ceph object store gateway service running at %s:%d", svc.Spec.ClusterIP, cephObjectStore.Spec.Gateway.Port)
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

func getLabels(name, namespace string) map[string]string {
	labels := controller.PodLabels(AppName, namespace, "rgw", name)
	labels["rook_object_store"] = name
	return labels
}
