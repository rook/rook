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
	k8smon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	appName = "osd"
)

type Cluster struct {
	Namespace       string
	Keyring         string
	Version         string
	dataDirHostPath string
	deviceFilter    string
	useAllDevices   bool
}

func New(namespace, version, deviceFilter, dataDirHostPath string, useAllDevices bool) *Cluster {
	return &Cluster{
		Namespace:       namespace,
		Version:         version,
		deviceFilter:    deviceFilter,
		dataDirHostPath: dataDirHostPath,
		useAllDevices:   useAllDevices,
	}
}

func (c *Cluster) Start(clientset kubernetes.Interface, cluster *mon.ClusterInfo) error {
	logger.Infof("start running osds")

	if cluster == nil || len(cluster.Monitors) == 0 {
		return fmt.Errorf("missing mons to start osds")
	}

	ds, err := c.makeDaemonSet(cluster)
	_, err = clientset.Extensions().DaemonSets(c.Namespace).Create(ds)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
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
	ds.Name = appName
	ds.Namespace = c.Namespace

	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: v1.ObjectMeta{
			Name: appName,
			Labels: map[string]string{
				k8sutil.AppAttr:     appName,
				k8sutil.ClusterAttr: cluster.Name,
			},
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{c.osdContainer(cluster)},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes: []v1.Volume{
				{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
				{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}},
			},
		},
	}

	ds.Spec = extensions.DaemonSetSpec{Template: podSpec}

	return ds, nil
}

func (c *Cluster) osdContainer(cluster *mon.ClusterInfo) v1.Container {

	command := fmt.Sprintf("/usr/bin/rookd osd --data-dir=%s --mon-endpoints=%s --cluster-name=%s ",
		k8sutil.DataDir, mon.FlattenMonEndpoints(cluster.Monitors), cluster.Name)
	if c.deviceFilter != "" {
		command += fmt.Sprintf("--data-devices=%s ", c.deviceFilter)
	} else if c.useAllDevices {
		command += fmt.Sprintf("--data-devices=all ")
	}

	privileged := true
	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    appName,
		Image:   k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			{Name: "devices", MountPath: "/dev"},
		},
		Env: []v1.EnvVar{
			k8smon.MonSecretEnvVar(),
			k8smon.AdminSecretEnvVar(),
		},
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
	}
}
