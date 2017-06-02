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

	"strings"

	"strconv"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	appName    = "osd"
	appNameFmt = "osd-%s"
)

type Cluster struct {
	context         *clusterd.Context
	placement       k8sutil.Placement
	Name            string
	Namespace       string
	Keyring         string
	Version         string
	Storage         StorageSpec
	dataDirHostPath string
}

func New(context *clusterd.Context, name, namespace, version string, storageSpec StorageSpec, dataDirHostPath string, placement k8sutil.Placement) *Cluster {
	return &Cluster{
		context:         context,
		placement:       placement,
		Name:            name,
		Namespace:       namespace,
		Version:         version,
		Storage:         storageSpec,
		dataDirHostPath: dataDirHostPath,
	}
}

func (c *Cluster) Start() error {
	logger.Infof("start running osds in namespace %s", c.Namespace)

	if c.Storage.UseAllNodes {
		// make a daemonset for all nodes in the cluster
		ds := c.makeDaemonSet(c.Storage.Selection, c.Storage.Config)
		_, err := c.context.Clientset.Extensions().DaemonSets(c.Namespace).Create(ds)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create osd daemon set. %+v", err)
			}
			logger.Infof("osd daemon set already exists")
		} else {
			logger.Infof("osd daemon set started")
		}
	} else {
		for i := range c.Storage.Nodes {
			// fully resolve the storage config for this node
			n := c.Storage.resolveNode(c.Storage.Nodes[i].Name)

			// create the replicaSet that will run the OSDs for this node
			rs := c.makeReplicaSet(n.Name, n.Devices, n.Directories, n.Selection, n.Config)
			_, err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Create(rs)
			if err != nil {
				if !errors.IsAlreadyExists(err) {
					return fmt.Errorf("failed to create osd replica set for node %s. %+v", n.Name, err)
				}
				logger.Infof("osd replica set already exists for node %s", n.Name)
			} else {
				logger.Infof("osd replica set started for node %s", n.Name)
			}
		}
	}

	return nil
}

func (c *Cluster) makeDaemonSet(selection Selection, config Config) *extensions.DaemonSet {
	ds := &extensions.DaemonSet{}
	ds.Name = appName
	ds.Namespace = c.Namespace

	podSpec := c.podTemplateSpec(nil, nil, selection, config)

	ds.Spec = extensions.DaemonSetSpec{Template: podSpec}
	return ds
}

func (c *Cluster) makeReplicaSet(nodeName string, devices []Device, directories []Directory,
	selection Selection, config Config) *extensions.ReplicaSet {

	rs := &extensions.ReplicaSet{}
	rs.Name = fmt.Sprintf(appNameFmt, nodeName)
	rs.Namespace = c.Namespace

	podSpec := c.podTemplateSpec(devices, directories, selection, config)
	podSpec.Spec.NodeSelector = map[string]string{metav1.LabelHostname: nodeName}

	replicaCount := int32(1)

	rs.Spec = extensions.ReplicaSetSpec{
		Template: podSpec,
		Replicas: &replicaCount,
	}

	return rs
}

func (c *Cluster) podTemplateSpec(devices []Device, directories []Directory, selection Selection, config Config) v1.PodTemplateSpec {
	// by default, the data/config dir will be an empty volume
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		// the user has specified a host path to use for the data dir, use that instead
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	// create volume config for the data dir and /dev so the pod can access devices on the host
	volumes := []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}},
		k8sutil.ConfigOverrideVolume(),
	}

	// add each OSD directory as another host path volume source
	for _, d := range directories {
		dirVolume := v1.Volume{
			Name:         k8sutil.PathToVolumeName(d.Path),
			VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: d.Path}},
		}
		volumes = append(volumes, dirVolume)
	}

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{c.osdContainer(devices, directories, selection, config)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes:       volumes,
	}
	c.placement.ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
			Labels: map[string]string{
				k8sutil.AppAttr:     appName,
				k8sutil.ClusterAttr: c.Namespace,
			},
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}
}

func (c *Cluster) osdContainer(devices []Device, directories []Directory, selection Selection, config Config) v1.Container {

	envVars := []v1.EnvVar{
		hostnameEnvVar(),
		opmon.ClusterNameEnvVar(c.Name),
		opmon.MonEndpointEnvVar(),
		opmon.MonSecretEnvVar(),
		opmon.AdminSecretEnvVar(),
		k8sutil.ConfigDirEnvVar(),
		k8sutil.ConfigOverrideEnvVar(),
	}

	// only 1 of device list, device filter and use all devices can be specified.  We prioritize in that order.
	if len(devices) > 0 {
		deviceNames := make([]string, len(devices))
		for i := range devices {
			deviceNames[i] = devices[i].Name
		}
		envVars = append(envVars, dataDevicesEnvVar(strings.Join(deviceNames, ",")))
	} else if selection.DeviceFilter != "" {
		envVars = append(envVars, deviceFilterEnvVar(selection.DeviceFilter))
	} else if selection.getUseAllDevices() {
		envVars = append(envVars, deviceFilterEnvVar("all"))
	}

	if selection.MetadataDevice != "" {
		envVars = append(envVars, metadataDeviceEnvVar(selection.MetadataDevice))
	}

	volumeMounts := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		{Name: "devices", MountPath: "/dev"},
		k8sutil.ConfigOverrideMount(),
	}

	if len(directories) > 0 {
		// for each directory the user has specified, create a volume mount and pass it to the pod via cmd line arg
		dirPaths := make([]string, len(directories))
		for i := range directories {
			dpath := directories[i].Path
			dirPaths[i] = dpath
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: k8sutil.PathToVolumeName(dpath), MountPath: dpath})
		}

		envVars = append(envVars, dataDirectoriesEnvVar(strings.Join(dirPaths, ",")))
	}

	if config.StoreConfig.StoreType != "" {
		envVars = append(envVars, osdStoreEnvVar(config.StoreConfig.StoreType))
	}

	if config.StoreConfig.DatabaseSizeMB != 0 {
		envVars = append(envVars, osdDatabaseSizeEnvVar(config.StoreConfig.DatabaseSizeMB))
	}

	if config.StoreConfig.WalSizeMB != 0 {
		envVars = append(envVars, osdWalSizeEnvVar(config.StoreConfig.WalSizeMB))
	}

	if config.StoreConfig.JournalSizeMB != 0 {
		envVars = append(envVars, osdJournalSizeEnvVar(config.StoreConfig.JournalSizeMB))
	}

	if config.Location != "" {
		envVars = append(envVars, locationEnvVar(config.Location))
	}

	// set the hostname to the host's name from the downstream api.
	// the crush map doesn't like hostnames with periods, so we replace them with underscores.
	hostnameUpdate := `echo $(HOSTNAME) | sed "s/\./_/g" > /etc/hostname; hostname -F /etc/hostname`
	command := "/usr/local/bin/rookd osd"

	privileged := true
	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		// Set the hostname so we have the pod's host in the crush map rather than the pod container name
		Command:         []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s; %s", hostnameUpdate, command)},
		Name:            appName,
		Image:           k8sutil.MakeRookImage(c.Version),
		VolumeMounts:    volumeMounts,
		Env:             envVars,
		SecurityContext: &v1.SecurityContext{Privileged: &privileged},
	}
}

func hostnameEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "HOSTNAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}}
}

func dataDevicesEnvVar(dataDevices string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_DATA_DEVICES", Value: dataDevices}
}

func deviceFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_DATA_DEVICE_FILTER", Value: filter}
}

func metadataDeviceEnvVar(metadataDevice string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_METADATA_DEVICE", Value: metadataDevice}
}

func dataDirectoriesEnvVar(dataDirectories string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_DATA_DIRECTORIES", Value: dataDirectories}
}

func osdStoreEnvVar(osdStore string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_OSD_STORE", Value: osdStore}
}

func osdDatabaseSizeEnvVar(databaseSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_OSD_DATABASE_SIZE", Value: strconv.Itoa(databaseSize)}
}

func osdWalSizeEnvVar(walSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_OSD_WAL_SIZE", Value: strconv.Itoa(walSize)}
}

func osdJournalSizeEnvVar(journalSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_OSD_JOURNAL_SIZE", Value: strconv.Itoa(journalSize)}
}

func locationEnvVar(location string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_LOCATION", Value: location}
}
