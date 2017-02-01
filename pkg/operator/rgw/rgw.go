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
package rgw

import (
	"fmt"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	cephrgw "github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	api "k8s.io/client-go/1.5/pkg/api/v1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

const (
	rgwApp         = "cephrgw"
	deploymentName = "rgw"
)

type Cluster struct {
	Namespace string
	Version   string
	Replicas  int32
	factory   client.ConnectionFactory
}

func New(namespace, version string, factory client.ConnectionFactory) *Cluster {
	return &Cluster{
		Namespace: namespace,
		Version:   version,
		Replicas:  2,
		factory:   factory,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset, cluster *mon.ClusterInfo) error {
	logger.Infof("start running rgw")

	if cluster == nil || len(cluster.Monitors) == 0 {
		return fmt.Errorf("missing mons to start rgw")
	}

	keyring, err := c.createKeyring(cluster)
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	err = c.startService(clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	// start the deployment
	deployment, err := c.makeDeployment(cluster, keyring)
	_, err = clientset.Deployments(c.Namespace).Create(deployment)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create rgw deployment. %+v", err)
		}
		logger.Infof("rgw deployment already exists")
	} else {
		logger.Infof("rgw deployment started")
	}

	return nil
}

func (c *Cluster) createKeyring(cluster *mon.ClusterInfo) (string, error) {
	context := &clusterd.Context{ConfigDir: "/var/lib/rook"}

	// connect to the ceph cluster
	conn, err := mon.ConnectToClusterAsAdmin(context, c.factory, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to connect to cluster. %+v", err)
	}
	defer conn.Shutdown()

	// create the keyring
	keyring, err := cephrgw.CreateKeyring(conn)
	if err != nil {
		return "", fmt.Errorf("failed to create keyring. %+v", err)
	}

	return keyring, nil
}

func (c *Cluster) makeDeployment(cluster *mon.ClusterInfo, keyring string) (*extensions.Deployment, error) {
	deployment := &extensions.Deployment{}
	deployment.Name = deploymentName
	deployment.Namespace = c.Namespace

	podSpec := api.PodTemplateSpec{
		ObjectMeta: api.ObjectMeta{
			Name:        "rook-rgw",
			Labels:      getLabels(cluster.Name),
			Annotations: map[string]string{},
		},
		Spec: api.PodSpec{
			Containers:    []api.Container{c.rgwContainer(cluster, keyring)},
			RestartPolicy: api.RestartPolicyAlways,
			Volumes: []api.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podSpec, Replicas: &c.Replicas}

	return deployment, nil
}

func (c *Cluster) rgwContainer(cluster *mon.ClusterInfo, keyring string) api.Container {

	command := fmt.Sprintf("/usr/bin/rookd rgw --data-dir=%s --mon-endpoints=%s --cluster-name=%s --mon-secret=%s --admin-secret=%s --rgw-port=%d --rgw-host=%s --rgw-keyring=%s",
		k8sutil.DataDir, mon.FlattenMonEndpoints(cluster.Monitors), cluster.Name, cluster.MonitorSecret, cluster.AdminSecret, cephrgw.RGWPort, cephrgw.DNSName, keyring)
	return api.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    rgwApp,
		Image:   k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []api.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		},
	}
}

func (c *Cluster) startService(clientset *kubernetes.Clientset, clusterInfo *mon.ClusterInfo) error {
	labels := getLabels(clusterInfo.Name)
	s := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "ceph-rgw",
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "ceph-rgw",
					Port:       cephrgw.RGWPort,
					TargetPort: intstr.FromInt(int(cephrgw.RGWPort)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	s, err := clientset.Services(k8sutil.Namespace).Create(s)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create mon service. %+v", err)
		}
	}

	logger.Infof("RGW service running at %s:%d", s.Spec.ClusterIP, cephrgw.RGWPort)
	return nil
}

func getLabels(clusterName string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     rgwApp,
		k8sutil.ClusterAttr: clusterName,
	}
}
