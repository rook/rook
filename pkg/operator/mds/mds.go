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

// Package mds for file systems.
package mds

import (
	"fmt"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/client"
	cephmds "github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/pool"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mds")

const (
	appName            = "rook-ceph-mds"
	dataPoolSuffix     = "-data"
	metadataPoolSuffix = "-metadata"
	keyringName        = "keyring"
)

func CreateFileSystem(context *clusterd.Context, clusterName string, f *model.FilesystemRequest, version string, hostNetwork bool) error {
	fs := &Filesystem{
		ObjectMeta: metav1.ObjectMeta{Name: f.Name, Namespace: clusterName},
		Spec: FilesystemSpec{
			MetadataPool: pool.ModelToSpec(f.MetadataPool),
			MetadataServer: MetadataServerSpec{
				ActiveCount: f.MetadataServer.ActiveCount,
			},
		},
	}
	for _, p := range f.DataPools {
		fs.Spec.DataPools = append(fs.Spec.DataPools, pool.ModelToSpec(p))
	}

	return fs.Create(context, version, hostNetwork)
}

// Create the file system
func (f *Filesystem) Create(context *clusterd.Context, version string, hostNetwork bool) error {
	if err := f.validate(context); err != nil {
		return err
	}

	var dataPools []*model.Pool
	for _, pool := range f.Spec.DataPools {
		dataPools = append(dataPools, pool.ToModel(""))
	}
	fs := cephmds.NewFS(f.Name, f.Spec.MetadataPool.ToModel(""), dataPools, f.Spec.MetadataServer.ActiveCount)
	if err := fs.CreateFilesystem(context, f.Namespace); err != nil {
		return fmt.Errorf("failed to create file system %s: %+v", f.Name, err)
	}

	filesystem, err := client.GetFilesystem(context, f.Namespace, f.Name)
	if err != nil {
		return fmt.Errorf("failed to get file system %s. %+v", f.Name, err)
	}

	logger.Infof("start running mds for file system %s", f.Name)

	// start the deployment
	deployment := f.makeDeployment(strconv.Itoa(filesystem.ID), version, hostNetwork)
	_, err = context.Clientset.ExtensionsV1beta1().Deployments(f.Namespace).Create(deployment)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mds deployment. %+v", err)
		}
		logger.Infof("mds deployment %s already exists", deployment.Name)
	} else {
		logger.Infof("mds deployment %s started", deployment.Name)
	}

	return nil
}

// Delete the file system
func (f *Filesystem) Delete(context *clusterd.Context) error {
	// Delete the mds deployment
	k8sutil.DeleteDeployment(context.Clientset, f.Namespace, f.instanceName())

	// Delete the keyring
	// Delete the rgw keyring
	err := context.Clientset.CoreV1().Secrets(f.Namespace).Delete(f.instanceName(), &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete mds secret. %+v", err)
	}

	// Delete the ceph file system and pools
	if err := cephmds.DeleteFilesystem(context, f.Namespace, f.Name); err != nil {
		return fmt.Errorf("failed to delete file system %s: %+v", f.Name, err)
	}

	return nil
}

func (f *Filesystem) instanceName() string {
	return fmt.Sprintf("%s-%s", appName, f.Name)
}

func (f *Filesystem) makeDeployment(filesystemID, version string, hostNetwork bool) *extensions.Deployment {
	deployment := &extensions.Deployment{}
	deployment.Name = f.instanceName()
	deployment.Namespace = f.Namespace

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{f.mdsContainer(filesystemID, version)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			k8sutil.ConfigOverrideVolume(),
		},
		HostNetwork: hostNetwork,
	}
	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	f.Spec.MetadataServer.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        f.instanceName(),
			Labels:      f.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	// double the number of MDS instances for failover
	mdsCount := f.Spec.MetadataServer.ActiveCount * 2

	deployment.Spec = extensions.DeploymentSpec{Template: podTemplateSpec, Replicas: &mdsCount}

	return deployment
}

func (f *Filesystem) mdsContainer(filesystemID, version string) v1.Container {

	return v1.Container{
		Args: []string{
			"mds",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
		},
		Name:  f.instanceName(),
		Image: k8sutil.MakeRookImage(version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_POD_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
			{Name: "ROOK_FILESYSTEM_ID", Value: filesystemID},
			{Name: "ROOK_ACTIVE_STANDBY", Value: strconv.FormatBool(f.Spec.MetadataServer.ActiveStandby)},
			opmon.ClusterNameEnvVar(f.Namespace),
			opmon.EndpointEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			k8sutil.ConfigOverrideEnvVar(),
		},
	}
}

func (f *Filesystem) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: f.Namespace,
		"rook_file_system":  f.Name,
	}
}

func (f *Filesystem) validate(context *clusterd.Context) error {
	if f.Name == "" {
		return fmt.Errorf("missing name")
	}
	if f.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if len(f.Spec.DataPools) == 0 {
		return fmt.Errorf("at least one data pool required")
	}
	if err := f.Spec.MetadataPool.Validate(context, f.Namespace); err != nil {
		return fmt.Errorf("invalid metadata pool. %+v", err)
	}
	for _, pool := range f.Spec.DataPools {
		if err := pool.Validate(context, f.Namespace); err != nil {
			return fmt.Errorf("Invalid data pool. %+v", err)
		}
	}
	if f.Spec.MetadataServer.ActiveCount < 1 {
		return fmt.Errorf("MetadataServer.ActiveCount must be at least 1")
	}

	return nil
}
