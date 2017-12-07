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

// Package osd for the Ceph OSDs.
package osd

import (
	"fmt"

	"strings"

	"strconv"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	opmon "github.com/rook/rook/pkg/operator/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")

const (
	appName    = "rook-ceph-osd"
	appNameFmt = "rook-ceph-osd-%s"
)

var clusterAccessRules = []v1beta1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"configmaps"},
		Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
	},
}

// Cluster keeps track of the OSDs
type Cluster struct {
	context         *clusterd.Context
	Namespace       string
	placement       rookalpha.Placement
	Keyring         string
	Version         string
	Storage         rookalpha.StorageSpec
	dataDirHostPath string
	HostNetwork     bool
	resources       v1.ResourceRequirements
}

// New creates an instance of the OSD manager
func New(context *clusterd.Context, namespace, version string, storageSpec rookalpha.StorageSpec, dataDirHostPath string, placement rookalpha.Placement, hostNetwork bool, resources v1.ResourceRequirements) *Cluster {
	return &Cluster{
		context:         context,
		Namespace:       namespace,
		placement:       placement,
		Version:         version,
		Storage:         storageSpec,
		dataDirHostPath: dataDirHostPath,
		HostNetwork:     hostNetwork,
		resources:       resources,
	}
}

// Start the osd management
func (c *Cluster) Start() error {
	logger.Infof("start running osds in namespace %s", c.Namespace)

	// create the artifacts for the api service to work with RBAC enabled
	err := k8sutil.MakeRole(c.context.Clientset, c.Namespace, appName, clusterAccessRules)
	if err != nil {
		logger.Warningf("failed to init RBAC for OSDs. %+v", err)
	}

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
			n := c.Storage.ResolveNode(c.Storage.Nodes[i].Name)

			resources := k8sutil.MergeResourceRequirements(c.Storage.Nodes[i].Resources, c.resources)

			// create the replicaSet that will run the OSDs for this node
			rs := c.makeReplicaSet(n.Name, n.Devices, n.Selection, resources, n.Config)
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

func (c *Cluster) makeDaemonSet(selection rookalpha.Selection, config rookalpha.Config) *extensions.DaemonSet {
	ds := &extensions.DaemonSet{}
	ds.Name = appName
	ds.Namespace = c.Namespace

	podSpec := c.podTemplateSpec(nil, selection, c.resources, config)

	ds.Spec = extensions.DaemonSetSpec{Template: podSpec}
	return ds
}

func (c *Cluster) makeReplicaSet(nodeName string, devices []rookalpha.Device,
	selection rookalpha.Selection, resources v1.ResourceRequirements, config rookalpha.Config) *extensions.ReplicaSet {

	rs := &extensions.ReplicaSet{}
	rs.Name = fmt.Sprintf(appNameFmt, nodeName)
	rs.Namespace = c.Namespace

	podSpec := c.podTemplateSpec(devices, selection, resources, config)
	podSpec.Spec.NodeSelector = map[string]string{apis.LabelHostname: nodeName}

	replicaCount := int32(1)

	rs.Spec = extensions.ReplicaSetSpec{
		Template: podSpec,
		Replicas: &replicaCount,
	}

	return rs
}

func (c *Cluster) podTemplateSpec(devices []rookalpha.Device, selection rookalpha.Selection,
	resources v1.ResourceRequirements, config rookalpha.Config) v1.PodTemplateSpec {
	// by default, the data/config dir will be an empty volume
	dataDirSource := v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}
	if c.dataDirHostPath != "" {
		// the user has specified a host path to use for the data dir, use that instead
		dataDirSource = v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.dataDirHostPath}}
	}

	volumes := []v1.Volume{
		{Name: k8sutil.DataDirVolume, VolumeSource: dataDirSource},
		k8sutil.ConfigOverrideVolume(),
	}

	// by default, don't define any volume config unless it is required
	if len(devices) > 0 || selection.DeviceFilter != "" || selection.GetUseAllDevices() || selection.MetadataDevice != "" {
		// create volume config for the data dir and /dev so the pod can access devices on the host
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
	}

	// add each OSD directory as another host path volume source
	for _, d := range selection.Directories {
		dirVolume := v1.Volume{
			Name:         k8sutil.PathToVolumeName(d.Path),
			VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: d.Path}},
		}
		volumes = append(volumes, dirVolume)
	}

	podSpec := v1.PodSpec{
		ServiceAccountName: appName,
		Containers:         []v1.Container{c.osdContainer(devices, selection, resources, config)},
		RestartPolicy:      v1.RestartPolicyAlways,
		Volumes:            volumes,
		HostNetwork:        c.HostNetwork,
	}
	if c.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
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

func (c *Cluster) osdContainer(devices []rookalpha.Device, selection rookalpha.Selection,
	resources v1.ResourceRequirements, config rookalpha.Config) v1.Container {

	envVars := []v1.EnvVar{
		nodeNameEnvVar(),
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		opmon.ClusterNameEnvVar(c.Namespace),
		opmon.EndpointEnvVar(),
		opmon.SecretEnvVar(),
		opmon.AdminSecretEnvVar(),
		k8sutil.ConfigDirEnvVar(),
		k8sutil.ConfigOverrideEnvVar(),
	}

	devMountNeeded := false

	// only 1 of device list, device filter and use all devices can be specified.  We prioritize in that order.
	if len(devices) > 0 {
		deviceNames := make([]string, len(devices))
		for i := range devices {
			deviceNames[i] = devices[i].Name
		}
		envVars = append(envVars, dataDevicesEnvVar(strings.Join(deviceNames, ",")))
		devMountNeeded = true
	} else if selection.DeviceFilter != "" {
		envVars = append(envVars, deviceFilterEnvVar(selection.DeviceFilter))
		devMountNeeded = true
	} else if selection.GetUseAllDevices() {
		envVars = append(envVars, deviceFilterEnvVar("all"))
		devMountNeeded = true
	}

	if selection.MetadataDevice != "" {
		envVars = append(envVars, metadataDeviceEnvVar(selection.MetadataDevice))
		devMountNeeded = true
	}

	volumeMounts := []v1.VolumeMount{
		{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
		k8sutil.ConfigOverrideMount(),
	}
	if devMountNeeded {
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, devMount)
	}

	if len(selection.Directories) > 0 {
		// for each directory the user has specified, create a volume mount and pass it to the pod via cmd line arg
		dirPaths := make([]string, len(selection.Directories))
		for i := range selection.Directories {
			dpath := selection.Directories[i].Path
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

	privileged := false
	// elevate to be privileged if it is going to mount devices
	if devMountNeeded {
		privileged = true
	}
	runAsUser := int64(0)
	// don't set runAsNonRoot explicitly when it is false, Kubernetes version < 1.6.4 has
	// an issue with this fixed in https://github.com/kubernetes/kubernetes/pull/47009
	// runAsNonRoot := false
	readOnlyRootFilesystem := false
	return v1.Container{
		// Set the hostname so we have the pod's host in the crush map rather than the pod container name
		Args:         []string{"osd"},
		Name:         appName,
		Image:        k8sutil.MakeRookImage(c.Version),
		VolumeMounts: volumeMounts,
		Env:          envVars,
		SecurityContext: &v1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &runAsUser,
			// don't set runAsNonRoot explicitly when it is false, Kubernetes version < 1.6.4 has
			// an issue with this fixed in https://github.com/kubernetes/kubernetes/pull/47009
			// RunAsNonRoot:           &runAsNonRoot,
			ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		},
		Resources: resources,
	}
}

func nodeNameEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}}
}

func dataDevicesEnvVar(dataDevices string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICES", Value: dataDevices}
}

func deviceFilterEnvVar(filter string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DEVICE_FILTER", Value: filter}
}

func metadataDeviceEnvVar(metadataDevice string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_METADATA_DEVICE", Value: metadataDevice}
}

func dataDirectoriesEnvVar(dataDirectories string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_DATA_DIRECTORIES", Value: dataDirectories}
}

func osdStoreEnvVar(osdStore string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_OSD_STORE", Value: osdStore}
}

func osdDatabaseSizeEnvVar(databaseSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_OSD_DATABASE_SIZE", Value: strconv.Itoa(databaseSize)}
}

func osdWalSizeEnvVar(walSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_OSD_WAL_SIZE", Value: strconv.Itoa(walSize)}
}

func osdJournalSizeEnvVar(journalSize int) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_OSD_JOURNAL_SIZE", Value: strconv.Itoa(journalSize)}
}

func locationEnvVar(location string) v1.EnvVar {
	return v1.EnvVar{Name: "ROOK_LOCATION", Value: location}
}
