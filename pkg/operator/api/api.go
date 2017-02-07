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

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	k8smon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

const (
	deploymentName = "rook-api"
)

type Cluster struct {
	Namespace string
	Version   string
	Replicas  int32
}

func New(namespace, version string) *Cluster {
	return &Cluster{
		Namespace: namespace,
		Version:   version,
		Replicas:  1,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset, cluster *mon.ClusterInfo) error {
	logger.Infof("starting the Rook api")

	if cluster == nil || len(cluster.Monitors) == 0 {
		return fmt.Errorf("missing mons to start")
	}

	// start the service
	err := c.startService(clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start api service. %+v", err)
	}

	// start the deployment
	deployment, err := c.makeDeployment(cluster)
	_, err = clientset.Deployments(c.Namespace).Create(deployment)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create api deployment. %+v", err)
		}
		logger.Infof("api deployment already exists")
	} else {
		logger.Infof("api deployment started")
	}

	return nil
}

func (c *Cluster) makeDeployment(cluster *mon.ClusterInfo) (*extensions.Deployment, error) {
	deployment := &extensions.Deployment{}
	deployment.Name = deploymentName
	deployment.Namespace = c.Namespace

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: v1.ObjectMeta{
			Name:        deploymentName,
			Labels:      getLabels(cluster.Name),
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{c.apiContainer(cluster)},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes: []v1.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			},
		},
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podSpec, Replicas: &c.Replicas}

	return deployment, nil
}

func (c *Cluster) apiContainer(cluster *mon.ClusterInfo) v1.Container {
	// need a different prefix on the env vars
	monSecretVar := k8smon.MonSecretEnvVar()
	adminSecretVar := k8smon.AdminSecretEnvVar()
	monSecretVar.Name = "ROOK_OPERATOR_MON_SECRET"
	adminSecretVar.Name = "ROOK_OPERATOR_ADMIN_SECRET"

	command := fmt.Sprintf("/usr/bin/rook-operator api --data-dir=%s --mon-endpoints=%s --cluster-name=%s --api-port=%d",
		k8sutil.DataDir, mon.FlattenMonEndpoints(cluster.Monitors), cluster.Name, model.Port)
	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    deploymentName,
		Image:   fmt.Sprintf("quay.io/rook/rook-operator:%v", c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		},
		Env: []v1.EnvVar{
			monSecretVar,
			adminSecretVar,
		},
	}
}

func (c *Cluster) startService(clientset *kubernetes.Clientset, clusterInfo *mon.ClusterInfo) error {
	labels := getLabels(clusterInfo.Name)
	s := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: labels,
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

	s, err := clientset.Services(k8sutil.Namespace).Create(s)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create api service. %+v", err)
		}
	}

	logger.Infof("API service running at %s:%d", s.Spec.ClusterIP, model.Port)
	return nil
}

func getLabels(clusterName string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     deploymentName,
		k8sutil.ClusterAttr: clusterName,
	}
}
