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
package file

import (
	"fmt"
	"strconv"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephmds "github.com/rook/rook/pkg/daemon/ceph/mds"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	AppName = "rook-ceph-mds"
)

// Create the file system
func CreateFilesystem(context *clusterd.Context, fs cephv1beta1.Filesystem, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {
	if err := validateFilesystem(context, fs); err != nil {
		return err
	}

	var dataPools []*model.Pool
	for _, p := range fs.Spec.DataPools {
		dataPools = append(dataPools, p.ToModel(""))
	}
	f := cephmds.NewFS(fs.Name, fs.Spec.MetadataPool.ToModel(""), dataPools, fs.Spec.MetadataServer.ActiveCount)
	if err := f.CreateFilesystem(context, fs.Namespace); err != nil {
		return fmt.Errorf("failed to create file system %s: %+v", fs.Name, err)
	}

	filesystem, err := client.GetFilesystem(context, fs.Namespace, fs.Name)
	if err != nil {
		return fmt.Errorf("failed to get file system %s. %+v", fs.Name, err)
	}

	logger.Infof("start running mds for file system %s", fs.Name)

	// start the deployment
	deployment := makeDeployment(context.Clientset, fs, strconv.Itoa(filesystem.ID), version, hostNetwork, ownerRefs)
	_, err = context.Clientset.ExtensionsV1beta1().Deployments(fs.Namespace).Create(deployment)
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
func DeleteFilesystem(context *clusterd.Context, fs cephv1beta1.Filesystem) error {
	// Delete the mds deployment
	k8sutil.DeleteDeployment(context.Clientset, fs.Namespace, instanceName(fs))

	// Delete the keyring
	// Delete the rgw keyring
	err := context.Clientset.CoreV1().Secrets(fs.Namespace).Delete(instanceName(fs), &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete mds secret. %+v", err)
	}

	// Delete the ceph file system and pools
	if err := cephmds.DeleteFilesystem(context, fs.Namespace, fs.Name); err != nil {
		return fmt.Errorf("failed to delete file system %s: %+v", fs.Name, err)
	}

	return nil
}

func instanceName(fs cephv1beta1.Filesystem) string {
	return fmt.Sprintf("%s-%s", AppName, fs.Name)
}

func makeDeployment(clientset kubernetes.Interface, fs cephv1beta1.Filesystem, filesystemID, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) *extensions.Deployment {
	deployment := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(fs),
			Namespace: fs.Namespace,
		},
	}
	k8sutil.SetOwnerRefs(clientset, fs.Namespace, &deployment.ObjectMeta, ownerRefs)

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{mdsContainer(fs, filesystemID, version)},
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
	fs.Spec.MetadataServer.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instanceName(fs),
			Labels:      getLabels(fs),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	// double the number of MDS instances for failover
	mdsCount := fs.Spec.MetadataServer.ActiveCount * 2

	deployment.Spec = extensions.DeploymentSpec{Template: podTemplateSpec, Replicas: &mdsCount}

	return deployment
}

func mdsContainer(fs cephv1beta1.Filesystem, filesystemID, version string) v1.Container {

	return v1.Container{
		Args: []string{
			"ceph",
			"mds",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
		},
		Name:  instanceName(fs),
		Image: k8sutil.MakeRookImage(version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_POD_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
			{Name: "ROOK_FILESYSTEM_ID", Value: filesystemID},
			{Name: "ROOK_ACTIVE_STANDBY", Value: strconv.FormatBool(fs.Spec.MetadataServer.ActiveStandby)},
			opmon.ClusterNameEnvVar(fs.Namespace),
			opmon.EndpointEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: fs.Spec.MetadataServer.Resources,
	}
}

func getLabels(fs cephv1beta1.Filesystem) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: fs.Namespace,
		"rook_file_system":  fs.Name,
	}
}

func validateFilesystem(context *clusterd.Context, f cephv1beta1.Filesystem) error {
	if f.Name == "" {
		return fmt.Errorf("missing name")
	}
	if f.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if len(f.Spec.DataPools) == 0 {
		return fmt.Errorf("at least one data pool required")
	}
	if err := pool.ValidatePoolSpec(context, f.Namespace, &f.Spec.MetadataPool); err != nil {
		return fmt.Errorf("invalid metadata pool. %+v", err)
	}
	for _, p := range f.Spec.DataPools {
		if err := pool.ValidatePoolSpec(context, f.Namespace, &p); err != nil {
			return fmt.Errorf("Invalid data pool. %+v", err)
		}
	}
	if f.Spec.MetadataServer.ActiveCount < 1 {
		return fmt.Errorf("MetadataServer.ActiveCount must be at least 1")
	}

	return nil
}
