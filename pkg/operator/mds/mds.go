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
package mds

import (
	"fmt"

	cephmds "github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	appName            = "mds"
	dataPoolSuffix     = "-data"
	metadataPoolSuffix = "-metadata"
	keyringName        = "keyring"
)

type Cluster struct {
	Name      string
	Namespace string
	Version   string
	Replicas  int32
	context   *clusterd.Context
	dataDir   string
	placement k8sutil.Placement
}

func New(context *clusterd.Context, name, namespace, version string, placement k8sutil.Placement) *Cluster {
	return &Cluster{
		context:   context,
		placement: placement,
		Name:      name,
		Namespace: namespace,
		Version:   version,
		Replicas:  1,
		dataDir:   k8sutil.DataDir,
	}
}

func (c *Cluster) Start(clientset kubernetes.Interface, cluster *mon.ClusterInfo) error {
	logger.Infof("start running mds")

	if cluster == nil || len(cluster.Monitors) == 0 {
		return fmt.Errorf("missing mons to start mds")
	}

	id := "mds1"
	err := c.createKeyring(clientset, cluster, id)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	// start the deployment
	deployment := c.makeDeployment(id)
	_, err = clientset.ExtensionsV1beta1().Deployments(c.Namespace).Create(deployment)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mds deployment. %+v", err)
		}
		logger.Infof("mds deployment already exists")
	} else {
		logger.Infof("mds deployment started")
	}

	return nil
}

func (c *Cluster) createKeyring(clientset kubernetes.Interface, cluster *mon.ClusterInfo, id string) error {
	_, err := clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the mds keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get mds secrets. %+v", err)
	}

	// get-or-create-key for the user account
	keyring, err := cephmds.CreateKeyring(c.context, c.Name, id)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	// Store the keyring in a secret
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: c.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mds secrets. %+v", err)
	}

	return nil
}

func (c *Cluster) makeDeployment(id string) *extensions.Deployment {
	deployment := &extensions.Deployment{}
	deployment.Name = appName
	deployment.Namespace = c.Namespace

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{c.mdsContainer(id)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			k8sutil.ConfigOverrideVolume(),
		},
	}
	c.placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        appName,
			Labels:      c.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podTemplateSpec, Replicas: &c.Replicas}

	return deployment
}

func (c *Cluster) mdsContainer(id string) v1.Container {

	command := fmt.Sprintf("/usr/local/bin/rookd mds --config-dir=%s --mds-id=%s ",
		k8sutil.DataDir, id)
	return v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", command)},
		Name:    appName,
		Image:   k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOKD_MDS_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: appName}, Key: keyringName}}},
			opmon.ClusterNameEnvVar(c.Name),
			opmon.MonEndpointEnvVar(),
			opmon.MonSecretEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
	}
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.Namespace,
	}
}
