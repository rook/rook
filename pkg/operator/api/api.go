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
package api

import (
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
)

const (
	deploymentName = "rook-api"
)

type Cluster struct {
	clientset kubernetes.Interface
	Namespace string
	Version   string
	Replicas  int32
}

func New(clientset kubernetes.Interface, namespace, version string) *Cluster {
	return &Cluster{
		clientset: clientset,
		Namespace: namespace,
		Version:   version,
		Replicas:  1,
	}
}

func (c *Cluster) Start() error {
	logger.Infof("starting the Rook api")

	// start the service
	err := c.startService()
	if err != nil {
		return fmt.Errorf("failed to start api service. %+v", err)
	}

	// start the deployment
	deployment := c.makeDeployment()
	_, err = c.clientset.ExtensionsV1beta1().Deployments(c.Namespace).Create(deployment)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create api deployment. %+v", err)
		}
		logger.Infof("api deployment already exists")
	} else {
		logger.Infof("api deployment started")
	}

	return nil
}

func (c *Cluster) makeDeployment() *extensions.Deployment {
	deployment := &extensions.Deployment{}
	deployment.Name = deploymentName
	deployment.Namespace = c.Namespace

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: v1.ObjectMeta{
			Name:        deploymentName,
			Labels:      c.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{c.apiContainer()},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes: []v1.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			},
		},
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podSpec, Replicas: &c.Replicas}

	return deployment
}

func (c *Cluster) apiContainer() v1.Container {
	envVars := []v1.EnvVar{
		k8sutil.NamespaceEnvVar(),
		opmon.MonSecretEnvVar(),
		opmon.AdminSecretEnvVar(),
		opmon.MonEndpointEnvVar(),
		opmon.ClusterNameEnvVar(),
	}
	// replace the ROOKD prefix with ROOK_OPERATOR
	for i := range envVars {
		envVars[i].Name = strings.Replace(envVars[i].Name, "ROOKD_", "ROOK_OPERATOR_", 1)
	}

	command := fmt.Sprintf("/usr/bin/rook-operator api --data-dir=%s --api-port=%d --container-version=%s",
		k8sutil.DataDir, model.Port, c.Version)
	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    deploymentName,
		Image:   k8sutil.MakeRookOperatorImage(c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		},
		Env: envVars,
	}
}

func (c *Cluster) startService() error {
	labels := c.getLabels()
	s := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      deploymentName,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       deploymentName,
					Port:       model.Port,
					TargetPort: intstr.FromInt(int(model.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	s, err := c.clientset.CoreV1().Services(c.Namespace).Create(s)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create api service. %+v", err)
		}
		logger.Infof("api service already running")
		return nil
	}

	logger.Infof("API service running at %s:%d", s.Spec.ClusterIP, model.Port)
	return nil
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     deploymentName,
		k8sutil.ClusterAttr: c.Namespace,
	}
}
