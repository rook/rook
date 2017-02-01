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
package osd

import (
	"fmt"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
	api "k8s.io/client-go/1.5/pkg/api/v1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

const (
	osdApp        = "cephosd"
	daemonSetName = "osd"
)

type Cluster struct {
	Namespace string
	Keyring   string
	Version   string
}

func New(namespace, version string) *Cluster {
	return &Cluster{
		Namespace: namespace,
		Version:   version,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset, cluster *mon.ClusterInfo) error {
	logger.Infof("start running osds")

	if cluster == nil || len(cluster.Monitors) == 0 {
		return fmt.Errorf("missing mons to start osds")
	}

	ds, err := c.makeDaemonSet(cluster)
	_, err = clientset.DaemonSets(c.Namespace).Create(ds)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create osd daemon set. %+v", err)
		}
		logger.Infof("osd daemon set already exists")
	} else {
		logger.Infof("osd daemon set started")
	}

	return nil
}

func (c *Cluster) makeDaemonSet(cluster *mon.ClusterInfo) (*extensions.DaemonSet, error) {
	ds := &extensions.DaemonSet{}
	ds.Name = daemonSetName
	ds.Namespace = c.Namespace

	podSpec := api.PodTemplateSpec{
		ObjectMeta: api.ObjectMeta{
			Name: "rookosd",
			Labels: map[string]string{
				k8sutil.AppAttr:     osdApp,
				k8sutil.ClusterAttr: cluster.Name,
			},
			Annotations: map[string]string{},
		},
		Spec: api.PodSpec{
			Containers:    []api.Container{c.osdContainer(cluster)},
			RestartPolicy: api.RestartPolicyAlways,
			Volumes: []api.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
				{Name: "devices", VolumeSource: api.VolumeSource{HostPath: &api.HostPathVolumeSource{Path: "/dev"}}},
			},
		},
	}

	ds.Spec = extensions.DaemonSetSpec{Template: podSpec}

	return ds, nil
}

func (c *Cluster) osdContainer(cluster *mon.ClusterInfo) api.Container {

	command := fmt.Sprintf("/usr/bin/rookd osd --data-dir=%s --mon-endpoints=%s --cluster-name=%s --mon-secret=%s --admin-secret=%s ",
		k8sutil.DataDir, mon.FlattenMonEndpoints(cluster.Monitors), cluster.Name, cluster.MonitorSecret, cluster.AdminSecret)
	privileged := true
	return api.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    "cephosd",
		Image:   k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []api.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			{Name: "devices", MountPath: "/dev"},
		},
		SecurityContext: &api.SecurityContext{Privileged: &privileged},
	}
}
