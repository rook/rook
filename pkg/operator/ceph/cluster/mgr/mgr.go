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

// Package mgr for the Ceph manager.
package mgr

import (
	"fmt"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mgr")

const (
	appName     = "rook-ceph-mgr"
	keyringName = "keyring"

	prometheusModuleName = "prometheus"
	metricsPort          = 9283
)

// Cluster is the ceph mgr manager
type Cluster struct {
	Namespace   string
	Version     string
	Replicas    int
	placement   rookalpha.Placement
	context     *clusterd.Context
	dataDir     string
	HostNetwork bool
	resources   v1.ResourceRequirements
	ownerRef    metav1.OwnerReference
}

// New creates an instance of the mgr
func New(context *clusterd.Context, namespace, version string, placement rookalpha.Placement, hostNetwork bool,
	resources v1.ResourceRequirements, ownerRef metav1.OwnerReference) *Cluster {
	return &Cluster{
		context:     context,
		Namespace:   namespace,
		placement:   placement,
		Version:     version,
		Replicas:    1,
		dataDir:     k8sutil.DataDir,
		HostNetwork: hostNetwork,
		resources:   resources,
		ownerRef:    ownerRef,
	}
}

// Start the mgr instance
func (c *Cluster) Start() error {
	logger.Infof("start running mgr")

	for i := 0; i < c.Replicas; i++ {
		name := fmt.Sprintf("%s%d", appName, i)
		if err := c.createKeyring(c.Namespace, name); err != nil {
			return fmt.Errorf("failed to create mgr keyring. %+v", err)
		}

		if err := c.enablePrometheusModule(c.Namespace); err != nil {
			return fmt.Errorf("failed to enable mgr prometheus module. %+v", err)
		}

		// create the service
		service := c.makeService(appName)
		if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(service); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mgr service. %+v", err)
			}
			logger.Infof("%s service already exists", name)
		} else {
			logger.Infof("%s service started", name)
		}

		// start the deployment
		deployment := c.makeDeployment(name)
		if _, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Create(deployment); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mgr deployment. %+v", err)
			}
			logger.Infof("%s deployment already exists", name)
		} else {
			logger.Infof("%s deployment started", name)
		}
	}

	return nil
}

func (c *Cluster) makeService(name string) *v1.Service {
	labels := c.getLabels()
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       c.Namespace,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
			Labels:          labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "http-metrics",
					Port:     int32(metricsPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}
}

func (c *Cluster) makeDeployment(name string) *extensions.Deployment {

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: c.getLabels(),
			Annotations: map[string]string{"prometheus.io/scrape": "true",
				"prometheus.io/port": strconv.Itoa(metricsPort)},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{c.mgrContainer(name)},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes: []v1.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
				k8sutil.ConfigOverrideVolume(),
			},
			HostNetwork: c.HostNetwork,
		},
	}
	if c.HostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	return &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       c.Namespace,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		Spec: extensions.DeploymentSpec{Template: podSpec, Replicas: &replicas},
	}
}

func (c *Cluster) mgrContainer(name string) v1.Container {

	return v1.Container{
		Args: []string{
			"ceph",
			"mgr",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
		},
		Name:  name,
		Image: k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_MGR_NAME", Value: name},
			{Name: "ROOK_MGR_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: name}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(c.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: c.resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "mgr",
				ContainerPort: int32(6800),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "http-metrics",
				ContainerPort: int32(metricsPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
	}
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.Namespace,
	}
}

func (c *Cluster) createKeyring(clusterName, name string) error {
	_, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(name, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the mgr keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get mgr secrets. %+v", err)
	}

	// get-or-create-key for the user account
	keyring, err := createKeyring(c.context, clusterName, name)
	if err != nil {
		return fmt.Errorf("failed to create mgr keyring. %+v", err)
	}

	// Store the keyring in a secret
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       c.Namespace,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mgr secrets. %+v", err)
	}

	return nil
}

// Ceph docs about the prometheus module: http://docs.ceph.com/docs/master/mgr/prometheus/
func (c *Cluster) enablePrometheusModule(clusterName string) error {
	if err := client.MgrEnableModule(c.context, clusterName, prometheusModuleName, true); err != nil {
		return fmt.Errorf("failed to enable mgr prometheus module. %+v", err)
	}
	return nil
}

func getKeyringProperties(name string) (string, []string) {
	username := fmt.Sprintf("mgr.%s", name)
	access := []string{"mon", "allow *"}
	return username, access
}

// create a keyring for the mds client with a limited set of privileges
func createKeyring(context *clusterd.Context, clusterName, name string) (string, error) {
	// get-or-create-key for the user account
	username, access := getKeyringProperties(name)
	keyring, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return keyring, nil
}
