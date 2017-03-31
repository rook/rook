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

	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	appName = "osd"
)

type Cluster struct {
	clientset       kubernetes.Interface
	Namespace       string
	Keyring         string
	Version         string
	useAllDevices   bool
	dataDirHostPath string
	deviceFilter    string
}

func New(clientset kubernetes.Interface, namespace, version, deviceFilter, dataDirHostPath string, useAllDevices bool) *Cluster {
	return &Cluster{
		clientset:       clientset,
		Namespace:       namespace,
		Version:         version,
		deviceFilter:    deviceFilter,
		dataDirHostPath: dataDirHostPath,
		useAllDevices:   useAllDevices,
	}
}

func (c *Cluster) Start() error {
	logger.Infof("start running osds")

	ds, err := c.makeDaemonSet()
	_, err = c.clientset.Extensions().DaemonSets(c.Namespace).Create(ds)
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

func (c *Cluster) makeDaemonSet() (*extensions.DaemonSet, error) {
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
				k8sutil.ClusterAttr: c.Namespace,
			},
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{c.osdContainer()},
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

func (c *Cluster) osdContainer() v1.Container {

	command := fmt.Sprintf("/usr/bin/rookd osd --data-dir=%s ", k8sutil.DataDir)
	var devices string
	if c.deviceFilter != "" {
		devices = c.deviceFilter
	} else if c.useAllDevices {
		devices = "all"
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
			v1.EnvVar{Name: "ROOKD_DATA_DEVICES", Value: devices},
			opmon.ClusterNameEnvVar(),
			opmon.MonEndpointEnvVar(),
			opmon.MonSecretEnvVar(),
			opmon.AdminSecretEnvVar(),
		},
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
	}
}
