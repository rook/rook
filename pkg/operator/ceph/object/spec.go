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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	rgwdaemon "github.com/rook/rook/pkg/daemon/ceph/rgw"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func startDeployment(context *clusterd.Context, store cephv1beta1.ObjectStore, version string, replicas int32, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {

	deployment := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(store),
			Namespace: store.Namespace,
		},
		Spec: extensions.DeploymentSpec{Template: makeRGWPodSpec(store, version, hostNetwork), Replicas: &replicas},
	}
	k8sutil.SetOwnerRefs(context.Clientset, store.Namespace, &deployment.ObjectMeta, ownerRefs)
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(store.Namespace).Create(deployment)
	return err
}

func startDaemonset(context *clusterd.Context, store cephv1beta1.ObjectStore, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {

	daemonset := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(store),
			Namespace: store.Namespace,
		},
		Spec: extensions.DaemonSetSpec{
			UpdateStrategy: extensions.DaemonSetUpdateStrategy{
				Type: extensions.RollingUpdateDaemonSetStrategyType,
			},
			Template: makeRGWPodSpec(store, version, hostNetwork),
		},
	}
	k8sutil.SetOwnerRefs(context.Clientset, store.Namespace, &daemonset.ObjectMeta, ownerRefs)

	_, err := context.Clientset.ExtensionsV1beta1().DaemonSets(store.Namespace).Create(daemonset)
	return err
}

func makeRGWPodSpec(store cephv1beta1.ObjectStore, version string, hostNetwork bool) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			makeConfigInitContainer(store, version),
		},
		Containers: []v1.Container{
			makeDaemonContainer(store, version),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes:       opspec.PodVolumes(""),
		HostNetwork:   hostNetwork,
	}
	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// Set the ssl cert if specified
	if store.Spec.Gateway.SSLCertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: store.Spec.Gateway.SSLCertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instanceName(store),
			Labels:      getLabels(store),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}
}

func makeConfigInitContainer(store cephv1beta1.ObjectStore, version string) v1.Container {
	container := v1.Container{
		Name:  opspec.ConfigInitContainerName,
		Image: k8sutil.MakeRookImage(version),
		Args: []string{
			"ceph",
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", store.Name),
			fmt.Sprintf("--rgw-port=%d", store.Spec.Gateway.Port),
			fmt.Sprintf("--rgw-secure-port=%d", store.Spec.Gateway.SecurePort),
		},
		VolumeMounts: opspec.RookVolumeMounts(),
		Env: []v1.EnvVar{
			{Name: "ROOK_RGW_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: instanceName(store)}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(store.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: store.Spec.Gateway.Resources,
	}

	if store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		// Pass the flag for using the ssl cert
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func makeDaemonContainer(store cephv1beta1.ObjectStore, version string) v1.Container {

	// start the rgw daemon in the foreground
	container := v1.Container{
		Name:  "rgw",
		Image: k8sutil.MakeRookImage(version),
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
		Resources:    store.Spec.Gateway.Resources,
	}

	return container
}

func startService(context *clusterd.Context, store cephv1beta1.ObjectStore, hostNetwork bool, ownerRefs []metav1.OwnerReference) (string, error) {
	labels := getLabels(store)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(store),
			Namespace: store.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRefs(context.Clientset, store.Namespace, &svc.ObjectMeta, ownerRefs)
	if hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", store.Spec.Gateway.Port)
	addPort(svc, "https", store.Spec.Gateway.SecurePort)

	svc, err := context.Clientset.CoreV1().Services(store.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create rgw service. %+v", err)
		}
		svc, err = context.Clientset.CoreV1().Services(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get existing service IP. %+v", err)
		}
		return svc.Spec.ClusterIP, nil
	}

	logger.Infof("Gateway service running at %s:%d", svc.Spec.ClusterIP, store.Spec.Gateway.Port)
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

func getLabels(store cephv1beta1.ObjectStore) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: store.Namespace,
		"rook_object_store": store.Name,
	}
}
